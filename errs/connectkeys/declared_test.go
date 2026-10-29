package connectkeys

import (
	"testing"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/MohammadBnei/ukubi-go/errs"
	errorsv1 "github.com/MohammadBnei/ukubi-go/proto/gen/go/ukubi/errors/v1"
)

// declaredProto is a test-only proto, as the descriptor buf would build from:
//
//	enum TestErrorKey {
//	  TEST_ERROR_KEY_UNSPECIFIED = 0;
//	  TEST_ERROR_KEY_FULL = 1 [(ukubi.errors.v1.key) = "test.full"];
//	  TEST_ERROR_KEY_GONE = 2 [(ukubi.errors.v1.key) = "test.gone"];
//	}
//	service TestService {
//	  rpc Open(Req) returns (Req) { option (ukubi.errors.v1.keys) = "test.full"; option (ukubi.errors.v1.keys) = "server.internal"; }
//	  rpc Close(Req) returns (Req) { option (ukubi.errors.v1.keys) = "test.gone"; }
//	}
//	service OtherService { rpc Ping(Req) returns (Req) { option (ukubi.errors.v1.keys) = "test.full"; } }
const declaredProto = `
name: "test/v1/test.proto"
package: "test.v1"
dependency: "ukubi/errors/v1/options.proto"
syntax: "proto3"
message_type { name: "Req" }
enum_type {
  name: "TestErrorKey"
  value { name: "TEST_ERROR_KEY_UNSPECIFIED" number: 0 }
  value { name: "TEST_ERROR_KEY_FULL" number: 1 options { [ukubi.errors.v1.key]: "test.full" } }
  value { name: "TEST_ERROR_KEY_GONE" number: 2 options { [ukubi.errors.v1.key]: "test.gone" } }
}
service {
  name: "TestService"
  method { name: "Open" input_type: ".test.v1.Req" output_type: ".test.v1.Req" options { [ukubi.errors.v1.keys]: ["test.full", "server.internal"] } }
  method { name: "Close" input_type: ".test.v1.Req" output_type: ".test.v1.Req" options { [ukubi.errors.v1.keys]: "test.gone" } }
}
service {
  name: "OtherService"
  method { name: "Ping" input_type: ".test.v1.Req" output_type: ".test.v1.Req" options { [ukubi.errors.v1.keys]: "test.full" } }
}
`

// TestCheckDeclared: the good proto passes; each mutation trips exactly the check it names, and nothing else.
func TestCheckDeclared(t *testing.T) {
	good := []errs.Key{"test.full", "test.gone", KeyInternal}
	cases := []struct {
		name     string
		mutate   func(*descriptorpb.FileDescriptorProto)
		registry []errs.Key
		want     string // the one finding, without the "connectkeys: " prefix; "" passes
	}{
		{name: "good", registry: good},
		{name: "good, KeyInternal unregistered", registry: []errs.Key{"test.full", "test.gone"}},
		{
			name: "an RPC lists an undeclared key",
			mutate: func(fd *descriptorpb.FileDescriptorProto) {
				proto.SetExtension(fd.Service[0].Method[1].Options, errorsv1.E_Keys, []string{"test.gone", "test.nope"})
			},
			registry: good,
			want:     `test.v1.TestService.Close lists "test.nope", which no key enum in test/v1/test.proto declares`,
		},
		{
			name: "an enum value with no wire string",
			mutate: func(fd *descriptorpb.FileDescriptorProto) {
				fd.EnumType[0].Value = append(fd.EnumType[0].Value, &descriptorpb.EnumValueDescriptorProto{Name: proto.String("TEST_ERROR_KEY_BARE"), Number: proto.Int32(3)})
			},
			registry: good,
			want:     "test.v1.TEST_ERROR_KEY_BARE declares no key: give it (ukubi.errors.v1.key)",
		},
		{
			name: "a wire string declared twice",
			mutate: func(fd *descriptorpb.FileDescriptorProto) {
				opts := &descriptorpb.EnumValueOptions{}
				proto.SetExtension(opts, errorsv1.E_Key, "test.full")
				fd.EnumType[0].Value = append(fd.EnumType[0].Value, &descriptorpb.EnumValueDescriptorProto{Name: proto.String("TEST_ERROR_KEY_FULL_AGAIN"), Number: proto.Int32(3), Options: opts})
			},
			registry: good,
			want:     `test.v1.TEST_ERROR_KEY_FULL and test.v1.TEST_ERROR_KEY_FULL_AGAIN both declare "test.full"`,
		},
		{
			name:     "a declared key the registry lacks",
			registry: []errs.Key{"test.full", KeyInternal},
			want:     `test.v1.TEST_ERROR_KEY_GONE declares "test.gone", which the registry lacks`,
		},
		{
			name:     "a registry key nothing declares",
			registry: append([]errs.Key{"test.extra"}, good...),
			want:     `registry key "test.extra" is declared by no key enum`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fd := &descriptorpb.FileDescriptorProto{}
			if err := prototext.Unmarshal([]byte(declaredProto), fd); err != nil {
				t.Fatal(err)
			}
			if c.mutate != nil {
				c.mutate(fd)
			}
			f, err := protodesc.NewFile(fd, protoregistry.GlobalFiles)
			if err != nil {
				t.Fatal(err)
			}
			var svcs []protoreflect.ServiceDescriptor
			for i := range f.Services().Len() {
				svcs = append(svcs, f.Services().Get(i))
			}
			err = CheckDeclared(c.registry, svcs...)
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("want nil, got:\n%v", err)
			case c.want != "" && (err == nil || err.Error() != "connectkeys: "+c.want):
				t.Fatalf("want %q, got:\n%v", c.want, err)
			}
		})
	}
}
