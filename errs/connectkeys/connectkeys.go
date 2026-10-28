// Package connectkeys carries an error key (standard §6) beside a Connect or gRPC code as a proto detail, and keys
// every error a handler returns without one as KeyInternal, keeping its code and words.
//
// The detail is the app's own message, passed to New as a template: any message with `string key = 1` and, if it has
// one, `string source = 2` (agentfleet.v1.ErrorKey, bridge.v1.ErrorKey and ukubi.errors.v1.ErrorKey all are). With
// several templates every error carries one detail per template, so a server can send an old and a new detail side by
// side while clients move. Keys are errs.Key: an app keeps one closed registry for its JSON and Connect keys.
//
// Errors Connect or gRPC raise before a handler runs (an unreadable body, an unknown procedure) carry no key.
// Ported from agent-fleet core/internal/errkeys.
package connectkeys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/MohammadBnei/ukubi-go/errs"
)

// KeyInternal is the fallback's key for an error built without one: its code says what it was. It is the Connect
// wire key; errs.KeyInternal ("internal") is the JSON body's.
const KeyInternal errs.Key = "server.internal"

// Keys builds and reads keyed errors for one app's detail templates. It holds only the templates' types, so it never
// changes after New: an app keeps it in a package var.
type Keys struct {
	details []detail
}

type detail struct {
	typ         protoreflect.MessageType
	name        string
	key, source protoreflect.FieldDescriptor // source is nil when the template has none
}

// New is the keys for the given detail templates. It panics unless there is at least one and each has a singular
// `string key = 1` and, if it has a field named source, a singular `string source = 2`, either outside any oneof
// (proto3 `optional` is allowed). Other fields stay unset.
func New(templates ...proto.Message) *Keys {
	if len(templates) == 0 {
		panic("connectkeys: New needs a detail template")
	}
	k := &Keys{}
	for _, t := range templates {
		md := t.ProtoReflect().Descriptor()
		d := detail{typ: t.ProtoReflect().Type(), name: string(md.FullName())}
		d.key = field(md, "key", 1)
		if md.Fields().ByName("source") != nil {
			d.source = field(md, "source", 2)
		}
		k.details = append(k.details, d)
	}
	return k
}

func field(md protoreflect.MessageDescriptor, name protoreflect.Name, num protoreflect.FieldNumber) protoreflect.FieldDescriptor {
	f := md.Fields().ByName(name)
	if f == nil || f.Number() != num || f.Kind() != protoreflect.StringKind || f.Cardinality() == protoreflect.Repeated || (f.ContainingOneof() != nil && !f.ContainingOneof().IsSynthetic()) {
		panic(fmt.Sprintf("connectkeys: %s needs a singular `string %s = %d`", md.FullName(), name, num))
	}
	return f
}

func (k *Keys) msgs(key errs.Key, source string) []proto.Message {
	out := make([]proto.Message, 0, len(k.details))
	for _, d := range k.details {
		m := d.typ.New()
		m.Set(d.key, protoreflect.ValueOfString(string(key)))
		if source != "" {
			if d.source == nil {
				panic(fmt.Sprintf("connectkeys: %s has no source field for %q", d.name, source))
			}
			m.Set(d.source, protoreflect.ValueOfString(source))
		}
		out = append(out, m.Interface())
	}
	return out
}

func (k *Keys) addConnect(e *connect.Error, key errs.Key, source string) {
	for _, m := range k.msgs(key, source) {
		d, err := connect.NewErrorDetail(m)
		if err != nil {
			panic(err) // a two-string message always marshals
		}
		e.AddDetail(d)
	}
}

// Connect is Connect's error for code with err's words, keyed.
func (k *Keys) Connect(code connect.Code, key errs.Key, err error) *connect.Error {
	return k.Sourced(code, key, "", err)
}

// Sourced is Connect, with the source whose words err holds. It panics on a non-empty source when a template has no
// source field, rather than drop it.
func (k *Keys) Sourced(code connect.Code, key errs.Key, source string, err error) *connect.Error {
	e := connect.NewError(code, err)
	k.addConnect(e, key, source)
	return e
}

// Status is gRPC's error for code with msg, keyed. It panics on codes.OK, which carries no error.
func (k *Keys) Status(code codes.Code, key errs.Key, msg string) error {
	if code == codes.OK {
		panic("connectkeys: Status with codes.OK")
	}
	return k.withStatus(status.New(code, msg), key)
}

func (k *Keys) withStatus(s *status.Status, key errs.Key) error {
	p := s.Proto()
	for _, m := range k.msgs(key, "") {
		a, err := anypb.New(m)
		if err != nil {
			panic(err)
		}
		p.Details = append(p.Details, a)
	}
	return status.FromProto(p).Err()
}

// Of is the key an error carries, Connect's or gRPC's, or "" without one. It matches the templates by full name and
// needs no entry in the global proto registry.
func (k *Keys) Of(err error) errs.Key {
	var ce *connect.Error
	if errors.As(err, &ce) {
		for _, d := range ce.Details() {
			if key, ok := k.read(d.Type(), d.Bytes()); ok {
				return key
			}
		}
		return ""
	}
	if s, ok := status.FromError(err); ok && err != nil {
		for _, a := range s.Proto().GetDetails() {
			u := a.GetTypeUrl()
			if key, ok := k.read(u[strings.LastIndex(u, "/")+1:], a.GetValue()); ok {
				return key
			}
		}
	}
	return ""
}

func (k *Keys) read(name string, b []byte) (errs.Key, bool) {
	for _, d := range k.details {
		if d.name != name {
			continue
		}
		m := d.typ.New()
		if proto.Unmarshal(b, m.Interface()) != nil {
			return "", false
		}
		return errs.Key(m.Get(d.key).String()), true
	}
	return "", false
}

// WithInterceptors is a Connect handler's interceptors, ics, behind the fallback: outermost, it keys an error the
// handler or any of ics returns without one as KeyInternal, keeping its code and words. Every handler takes its
// interceptors through this, never connect.WithInterceptors (connectkeys/wiring).
func (k *Keys) WithInterceptors(ics ...connect.Interceptor) connect.HandlerOption {
	return connect.WithInterceptors(append([]connect.Interceptor{keyAll{k}}, ics...)...)
}

type keyAll struct{ k *Keys }

func (a keyAll) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		res, err := next(ctx, req)
		return res, a.k.withConnectKey(err)
	}
}

func (keyAll) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (a keyAll) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return a.k.withConnectKey(next(ctx, conn))
	}
}

func (k *Keys) withConnectKey(err error) error {
	if err == nil || k.Of(err) != "" {
		return err
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		return k.Connect(plainCode(err), KeyInternal, err)
	}
	// A copy, never ce itself: a handler may return a shared sentinel, which adding to would race across requests and
	// stay keyed for in-process callers. Meta() makes the map on its first call, a write: so read a struct copy of ce,
	// whose Meta() makes it on the copy only (the bridge's form; fleet reads ce.Meta() and races on a bare sentinel).
	src := *ce
	e := connect.NewError(src.Code(), src.Unwrap())
	for _, d := range src.Details() {
		e.AddDetail(d)
	}
	for key, v := range src.Meta() {
		e.Meta()[key] = append([]string(nil), v...)
	}
	k.addConnect(e, KeyInternal, "")
	return e
}

// plainCode is the code Connect sends for a plain error (its toWire, wrapIfContextError): a context error's own, else
// Unknown.
func plainCode(err error) connect.Code {
	switch {
	case errors.Is(err, context.Canceled):
		return connect.CodeCanceled
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded):
		return connect.CodeDeadlineExceeded
	}
	return connect.CodeUnknown
}

// UnaryServerInterceptor and StreamServerInterceptor key an error a gRPC handler returns without one as KeyInternal,
// keeping its code and message. Chain them first.
func (k *Keys) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		res, err := next(ctx, req)
		return res, k.withStatusKey(err)
	}
}

func (k *Keys) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, next grpc.StreamHandler) error {
		return k.withStatusKey(next(srv, ss))
	}
}

func (k *Keys) withStatusKey(err error) error {
	if err == nil || k.Of(err) != "" {
		return err
	}
	s, ok := status.FromError(err)
	if !ok {
		s = status.FromContextError(err) // a plain error: a context error's code, else Unknown, as gRPC would answer
	}
	if s.Code() == codes.OK {
		return err // an OK status carries no detail; never reached for a non-nil error
	}
	return k.withStatus(s, KeyInternal)
}
