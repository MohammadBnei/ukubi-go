# ukubi-go

Shared Go packages for ukubi apps: the error model, telemetry (traces and JSON logs), and the checks that keep apps on the ukubi Code Standard.

Module: `github.com/MohammadBnei/ukubi-go`

Packages are added by pull request; each one is listed here with its import path when it lands.

## Packages

| Package | Import path | What it is |
| --- | --- | --- |
| errs | `github.com/MohammadBnei/ukubi-go/errs` | The error model: a Connect code, a stable key, and the JSON error body with its HTTP status. |
| connectkeys | `github.com/MohammadBnei/ukubi-go/errs/connectkeys` | The key as a Connect or gRPC error detail (the app's own `ErrorKey {key = 1, source = 2}` message, given to `New`), and the fallback interceptors that key anything left unkeyed as `server.internal`, installed outermost. |
| connectkeys/wiring | `github.com/MohammadBnei/ukubi-go/errs/connectkeys/wiring` | A syntactic check an app calls from a test: every Connect handler and gRPC server takes the fallback through the app's wrapper. It bans `connect.WithRequestGate`, whose errors skip the fallback. It finds generated handlers only in `/proto/gen/…connect` packages (hand-built `connect.New…Handler` calls are checked too), so the caller asserts the handler and server counts are above zero. |
| proto/gen/go/ukubi/errors/v1 | `github.com/MohammadBnei/ukubi-go/proto/gen/go/ukubi/errors/v1` | `ukubi.errors.v1.ErrorKey {string key = 1; string source = 2;}`, the shared error detail, generated from `proto/ukubi/errors/v1/errors.proto`. Its fields are `agentfleet.v1.ErrorKey`'s and `bridge.v1.ErrorKey`'s, number for number. |
| telemetry | `github.com/MohammadBnei/ukubi-go/telemetry` | Traces over OTLP/HTTP when an endpoint is set, and the JSON logger with trace ids. |

With an endpoint, `telemetry.New` sets OTel's error handler, so errors reported through `otel.Handle` (export failures included) log as slog lines at WARN. OTel's own config and limit warnings still print as plain text on stderr.

They came from `mountain/pkg`, with their history.

## ukubi.errors.v1.ErrorKey

Adoption is additive. An app that sends its own detail today (`agentfleet.v1.ErrorKey`, `bridge.v1.ErrorKey`) passes both templates to `connectkeys.New`, the old one first by convention, so every error carries both details with the same bytes. The old detail goes only once every client reads the new name, because today's readers match one name each and skip a detail they do not know:

- agent-fleet `islands/src/errors.ts` reads the detail whose type name is `agentfleet.v1.ErrorKey`, by hand.
- agent-fleet `core/internal/errkeys` `Of` reads only `*agentfleetv1.ErrorKey`, by Go type.
- No client reads `bridge.v1.ErrorKey` today. The bridge's own fallback (`internal/bus/errkeys.go` `hasKey`) checks only `bridge.v1.ErrorKey`, so the bridge's switch moves it to connectkeys or teaches it the new name.

Neither reader depends on the order of the details.

Go apps import the generated package and never generate their own Go copy of `errors.proto`: protobuf-go panics at start-up when two packages register the same file. A TypeScript client generates TS only from a copy, or reads the detail by hand as islands does.

Contributors: `proto/` is one buf module (`buf.yaml`: STANDARD lint, PACKAGE breaking). After editing a `.proto`, run `(cd proto && buf lint && buf generate --clean)` and commit `proto/gen`; `buf generate` runs protoc-gen-go through `go run`, so the generator is the protobuf version `go.mod` pins. CI fails on lint, on a break against the base (the PR's base branch, or main's previous commit on a push), unless a commit touching `proto/` says `BREAKING CHANGE:`, and on generated code that differs from what is committed.

## License

MIT, see [LICENSE](LICENSE).
