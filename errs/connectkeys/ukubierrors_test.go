package connectkeys

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	errorsv1 "github.com/MohammadBnei/ukubi-go/proto/gen/go/ukubi/errors/v1"
)

// TestUkubiErrorKey: the generated ukubi.errors.v1.ErrorKey as a template. Generated messages marshal in field order,
// so these are the exact bytes on the wire (TestBytes can only compare dynamicpb's records in any order).
func TestUkubiErrorKey(t *testing.T) {
	k := New(&errorsv1.ErrorKey{})
	d := k.Sourced(connect.CodeUnavailable, "store.unavailable", "store", nil).Details()[0]
	if want := []byte("\x0a\x11store.unavailable\x12\x05store"); d.Type() != "ukubi.errors.v1.ErrorKey" || !bytes.Equal(d.Bytes(), want) {
		t.Errorf("sourced: %s %x, want %x", d.Type(), d.Bytes(), want)
	}
	if got := k.Connect(connect.CodeUnknown, KeyInternal, nil).Details()[0].Bytes(); !bytes.Equal(got, []byte("\x0a\x0fserver.internal")) {
		t.Errorf("connect: %x", got)
	}
	s, _ := status.FromError(k.Status(codes.Unknown, KeyInternal, "x"))
	if a := s.Proto().GetDetails(); len(a) != 1 || a[0].GetTypeUrl() != "type.googleapis.com/ukubi.errors.v1.ErrorKey" ||
		!bytes.Equal(a[0].GetValue(), []byte("\x0a\x0fserver.internal")) {
		t.Errorf("grpc: %v", a)
	}
}

// TestUkubiErrorKeyWire: ukubi.errors.v1.ErrorKey has agentfleet.v1.ErrorKey's (and bridge.v1.ErrorKey's) fields,
// field for field, so each reads the other's bytes.
func TestUkubiErrorKeyWire(t *testing.T) {
	fleet := fleetLike(t, "agentfleet.v1")
	shape := func(m proto.Message) (out []string) {
		fs := m.ProtoReflect().Descriptor().Fields()
		for i := range fs.Len() {
			f := fs.Get(i)
			out = append(out, fmt.Sprintf("%s %d %v %v presence=%v oneof=%v", f.Name(), f.Number(), f.Kind(), f.Cardinality(), f.HasPresence(), f.ContainingOneof() != nil))
		}
		return out
	}
	if a, b := shape(&errorsv1.ErrorKey{}), shape(fleet); len(a) != 2 || !slices.Equal(a, b) {
		t.Errorf("fields %q, fleet's %q", a, b)
	}
	b, err := proto.Marshal(&errorsv1.ErrorKey{Key: "store.unavailable", Source: "store"})
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(b, fleet); err != nil {
		t.Fatal(err)
	}
	get := func(m proto.Message, name protoreflect.Name) string {
		return m.ProtoReflect().Get(m.ProtoReflect().Descriptor().Fields().ByName(name)).String()
	}
	if get(fleet, "key") != "store.unavailable" || get(fleet, "source") != "store" {
		t.Errorf("fleet read %x as %v", b, fleet)
	}
	back, err := proto.Marshal(fleet)
	if err != nil {
		t.Fatal(err)
	}
	var uk errorsv1.ErrorKey
	if err := proto.Unmarshal(back, &uk); err != nil || uk.GetKey() != "store.unavailable" || uk.GetSource() != "store" {
		t.Errorf("read fleet's %x as %v (%v)", back, &uk, err)
	}
}

// TestUkubiErrorKeyAdditive: with the old detail first and ukubi.errors.v1.ErrorKey second, every error carries both
// with the same fields, and a client knowing either name alone reads the key.
func TestUkubiErrorKeyAdditive(t *testing.T) {
	fleet, uk := fleetLike(t, "agentfleet.v1"), &errorsv1.ErrorKey{}
	both := New(fleet, uk)
	ds := both.Sourced(connect.CodeAborted, keyChanged, "store", nil).Details()
	if len(ds) != 2 || ds[0].Type() != "agentfleet.v1.ErrorKey" || ds[1].Type() != "ukubi.errors.v1.ErrorKey" || !sameFields(ds[0].Bytes(), ds[1].Bytes()) {
		var names []string
		for _, d := range ds {
			names = append(names, d.Type())
		}
		t.Fatalf("details %v", names)
	}
	err := both.Sourced(connect.CodeAborted, keyChanged, "store", nil)
	if New(fleet).Of(err) != keyChanged || New(uk).Of(err) != keyChanged {
		t.Errorf("fleet %q ukubi %q", New(fleet).Of(err), New(uk).Of(err))
	}
	for name, err := range map[string]error{
		"fleet only": New(fleet).Sourced(connect.CodeAborted, keyChanged, "store", nil),
		"ukubi only": New(uk).Status(codes.Aborted, keyChanged, "x"),
	} {
		if both.Of(err) != keyChanged {
			t.Errorf("%s: %q", name, both.Of(err))
		}
	}
}
