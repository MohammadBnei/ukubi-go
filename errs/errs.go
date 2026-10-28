// Package errs is ukubi's error model (mountain plan/15 §4): one of Connect's 16 codes, a stable key from the app's
// own closed registry, the words a person reads (Say), the source whose words Detail holds, and the cause. On the
// wire it is {code, key, message, source, detail} with the status Connect's table gives the code. Pages translate
// keys and never parse messages. It carries no Connect dependency: a client of a Connect service maps its errors.
//
// Each app keeps its registry: its Key constants and the sources whose own codes it passes on (From, Down build
// "<source>.<code>"), with a test that every key it uses is one of them. Only KeyInternal is shared: Write's own.
package errs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// Code is one of Connect's codes, by its wire name.
type Code string

const (
	Canceled           Code = "canceled"
	Unknown            Code = "unknown"
	InvalidArgument    Code = "invalid_argument"
	DeadlineExceeded   Code = "deadline_exceeded"
	NotFound           Code = "not_found"
	AlreadyExists      Code = "already_exists"
	PermissionDenied   Code = "permission_denied"
	ResourceExhausted  Code = "resource_exhausted"
	FailedPrecondition Code = "failed_precondition"
	Aborted            Code = "aborted"
	OutOfRange         Code = "out_of_range"
	Unimplemented      Code = "unimplemented"
	Internal           Code = "internal"
	Unavailable        Code = "unavailable"
	DataLoss           Code = "data_loss"
	Unauthenticated    Code = "unauthenticated"
)

// Status is Connect's code -> HTTP status table (connect-go's connectCodeToHTTP); an unknown code is a 500.
func (c Code) Status() int {
	switch c {
	case Canceled:
		return 499
	case InvalidArgument, FailedPrecondition, OutOfRange:
		return 400
	case Unauthenticated:
		return 401
	case PermissionDenied:
		return 403
	case NotFound:
		return 404
	case AlreadyExists, Aborted:
		return 409
	case ResourceExhausted:
		return 429
	case Unimplemented:
		return 501
	case Unavailable:
		return 503
	case DeadlineExceeded:
		return 504
	}
	return 500 // unknown, internal, data_loss
}

// Valid: c is one of the 16.
func (c Code) Valid() bool {
	switch c {
	case Canceled, Unknown, InvalidArgument, DeadlineExceeded, NotFound, AlreadyExists, PermissionDenied, ResourceExhausted,
		FailedPrecondition, Aborted, OutOfRange, Unimplemented, Internal, Unavailable, DataLoss, Unauthenticated:
		return true
	}
	return false
}

// Error is one error of the model.
type Error struct {
	Code   Code
	Key    Key    // stable, from the app's registry
	Say    string // what a person reads
	Source string // whose words Detail holds ("" = ours)
	Detail string // the source's own message, kept as it was
	cause  error
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return e.Say + ": " + e.Detail
	}
	return e.Say
}

func (e *Error) Unwrap() error { return e.cause }

func of(code Code, key Key, say string) *Error { return &Error{Code: code, Key: key, Say: say} }

// Invalid: the request is wrong (invalid_argument, 400).
func Invalid(key Key, say string) *Error { return of(InvalidArgument, key, say) }

// Precondition: the request is fine but what it acts on is not in a state to take it (failed_precondition, plan/15's
// Off; 400).
func Precondition(key Key, say string) *Error { return of(FailedPrecondition, key, say) }

// Missing: what it names does not exist (not_found, 404).
func Missing(key Key, say string) *Error { return of(NotFound, key, say) }

// NotHere: this deployment does not do that (unimplemented, 501).
func NotHere(key Key, say string) *Error { return of(Unimplemented, key, say) }

// Conflict: it clashes with the current state; try again on fresh state (aborted, 409).
func Conflict(key Key, say string) *Error { return of(Aborted, key, say) }

// Wrap keeps cause under e (errors.Is and As see it) and returns e.
func (e *Error) Wrap(cause error) *Error {
	e.cause = cause
	return e
}

// From is an error a source answered with its own code and words: key "<source>.<code>" (a registered source).
func From(source string, code Code, say, detail string, cause error) *Error {
	if !code.Valid() {
		code = Unknown
	}
	return &Error{Code: code, Key: Key(source + "." + string(code)), Say: say, Source: source, Detail: detail, cause: cause}
}

// Down is a source that did not answer: unavailable (503), its words in Detail, key "<source>.unavailable".
func Down(source string, cause error) *Error {
	return From(source, Unavailable, source+" is not answering", cause.Error(), cause)
}

// Key is a stable error key, from the app's registry. A page translates a key; its message is for a person.
type Key string

// KeyInternal is Write's key for an error outside the model: not ours to explain, the server's log says what.
const KeyInternal Key = "internal"

// Body is the wire shape.
type Body struct {
	Code    Code   `json:"code"`
	Key     string `json:"key"`
	Message string `json:"message"`
	Source  string `json:"source"`
	Detail  string `json:"detail"`
}

// Write answers err as the model's JSON with its code's status. An error outside the model is internal: its words
// are logged, not sent (they can hold anything).
func Write(w http.ResponseWriter, err error) {
	var e *Error
	if !errors.As(err, &e) {
		slog.Error(fmt.Sprintf("internal error: %v", err)) // ERROR, through whatever handler the app installed
		e = of(Internal, KeyInternal, "something went wrong on the server (its log says what)")
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(e.Code.Status())
	_ = json.NewEncoder(w).Encode(Body{Code: e.Code, Key: string(e.Key), Message: e.Say, Source: e.Source, Detail: e.Detail})
}

// IsBody: resp's body is an error of the model (IsModel). The body is read and put back.
func IsBody(resp *http.Response) bool {
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(b))
	return err == nil && IsModel(b)
}

// IsModel: b is an error of the model (from a service that speaks it), with one of the 16 codes and a key.
func IsModel(b []byte) bool {
	var x Body
	return json.Unmarshal(b, &x) == nil && x.Code.Valid() && x.Key != ""
}
