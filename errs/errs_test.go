package errs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const keyTest Key = "test.invalid" // a key of this test's own (the registries are the apps')

// TestStatus: Connect's table (connect-go's connectCodeToHTTP), all 16 codes.
func TestStatus(t *testing.T) {
	for c, want := range map[Code]int{Canceled: 499, Unknown: 500, InvalidArgument: 400, DeadlineExceeded: 504, NotFound: 404,
		AlreadyExists: 409, PermissionDenied: 403, ResourceExhausted: 429, FailedPrecondition: 400, Aborted: 409, OutOfRange: 400,
		Unimplemented: 501, Internal: 500, Unavailable: 503, DataLoss: 500, Unauthenticated: 401} {
		if !c.Valid() || c.Status() != want {
			t.Errorf("%s: %d, want %d", c, c.Status(), want)
		}
	}
	if Code("teapot").Valid() || Code("teapot").Status() != 500 {
		t.Error("an unknown code")
	}
}

func body(t *testing.T, err error) (int, Body) {
	t.Helper()
	w := httptest.NewRecorder()
	Write(w, err)
	var b Body
	if json.NewDecoder(w.Body).Decode(&b) != nil || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("not the model's body: %q", w.Body)
	}
	return w.Code, b
}

// TestWrite: the wire shape and status; the cause stays reachable; an error outside the model is internal and its
// words are not sent.
func TestWrite(t *testing.T) {
	cause := errors.New("cause")
	e := Invalid(keyTest, "no such building").Wrap(cause)
	if !errors.Is(e, cause) {
		t.Error("the cause is lost")
	}
	if code, b := body(t, e); code != 400 || b != (Body{Code: InvalidArgument, Key: string(keyTest), Message: "no such building"}) {
		t.Errorf("invalid: %d %+v", code, b)
	}
	if code, b := body(t, Down("sim", errors.New("dial tcp: refused"))); code != 503 || b.Key != "sim.unavailable" || b.Source != "sim" || b.Detail != "dial tcp: refused" {
		t.Errorf("down: %d %+v", code, b)
	}
	if code, b := body(t, From("fleet", "nonsense", "x", "y", nil)); code != 500 || b.Code != Unknown || b.Key != "fleet.unknown" {
		t.Errorf("a code outside the 16: %d %+v", code, b)
	}
	if code, b := body(t, errors.New("db password is hunter2")); code != 500 || b.Code != Internal || strings.Contains(b.Message+b.Detail, "hunter2") {
		t.Errorf("internal: %d %+v", code, b)
	}
}

// TestIsBody: a model's error body is recognised and left readable; anything else is not.
func TestIsBody(t *testing.T) {
	for raw, want := range map[string]bool{`{"code":"not_found","key":"scenario.missing","message":"no"}`: true,
		`{"code":"teapot","key":"x"}`: false, `{"code":"not_found"}`: false, "plain text": false} {
		resp := &http.Response{Body: io.NopCloser(strings.NewReader(raw))}
		got := IsBody(resp)
		rest, _ := io.ReadAll(resp.Body)
		if got != want || string(rest) != raw {
			t.Errorf("%q: %v (body %q)", raw, got, rest)
		}
	}
}

// TestWriteLogs: an error outside the model is logged at ERROR, with today's words, through whatever handler slog's
// default is (an app's JSON one); one of the model is not logged.
func TestWriteLogs(t *testing.T) {
	var buf bytes.Buffer
	old, w, flags := slog.Default(), log.Writer(), log.Flags()
	t.Cleanup(func() { slog.SetDefault(old); log.SetOutput(w); log.SetFlags(flags) }) // SetDefault(old) leaves log redirected
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	Write(httptest.NewRecorder(), errors.New("pq: connection refused"))
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil || m["level"] != "ERROR" || m["msg"] != "internal error: pq: connection refused" {
		t.Errorf("%s: %v", buf.Bytes(), err)
	}
	buf.Reset()
	Write(httptest.NewRecorder(), Invalid(keyTest, "x"))
	if buf.Len() != 0 {
		t.Errorf("a model error is logged: %s", buf.Bytes()) // the positive control: the check sees a log line
	}
}

// TestEdges: a nil cause, a typed-nil *Error, a shared error wrapped twice and a body past the MiB IsBody reads.
func TestEdges(t *testing.T) {
	if e := Down("sim", nil); e.Key != "sim.unavailable" || e.Detail != "" {
		t.Errorf("down, no cause: %+v", e)
	}
	var typed *Error
	if code, b := body(t, error(typed)); code != 500 || b.Code != Internal {
		t.Errorf("typed nil: %d %+v", code, b)
	}
	shared, a, b := Missing(keyTest, "x"), errors.New("a"), errors.New("b")
	if wa, wb := shared.Wrap(a), shared.Wrap(b); !errors.Is(wa, a) || errors.Is(wa, b) || !errors.Is(wb, b) || shared.Unwrap() != nil {
		t.Error("Wrap changed the shared error")
	}
	raw := strings.Repeat("x", 3<<20)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(raw))}
	if IsBody(resp) {
		t.Error("a big plain body is a model error")
	}
	if rest, _ := io.ReadAll(resp.Body); len(rest) != len(raw) {
		t.Errorf("the body was cut to %d of %d bytes", len(rest), len(raw))
	}
}
