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
| ukubicheck | `github.com/MohammadBnei/ukubi-go/cmd/ukubicheck` | A `go vet -vettool` binary: only the config file reads the environment, keys are the registry's constants, no mutable package-level state, no `log` package outside the wrappers. See [ukubicheck](#ukubicheck). |

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

## ukubicheck

Four `go/analysis` analyzers in one vet tool. Each type-checks the code (an import alias, a dot import, a method value and a call all name the same object), skips `_test.go` files, and with its flags empty bans everywhere.

| Analyzer | Refuses | Flags |
| --- | --- | --- |
| envban | Standard §5's readers in any file but the allowed ones: `os` `Getenv`, `LookupEnv`, `Environ`, `ExpandEnv`, `Setenv`, `Unsetenv`, `Clearenv`, `UserHomeDir`, `UserCacheDir`, `UserConfigDir`, `TempDir`, `MkdirTemp`, `CreateTemp`, `Getwd`, `StartProcess`; `io/ioutil` `TempDir`, `TempFile`; `syscall` `Getenv`, `Environ`, `Setenv`, `Unsetenv`, `Clearenv`, `ForkExec`, `Exec`, `StartProcess`, `Getwd`; `golang.org/x/sys/unix` `Getenv`, `Environ`, `Setenv`, `Unsetenv`, `Clearenv`, `Exec`, `Getwd`; `net/http.ProxyFromEnvironment`. Also `os/exec`, `os/user` and `golang.org/x/sys/execabs` whole, a dot import of a reader's package, cgo and `//go:linkname`. An allowed file that reads nothing, or is missing from its package, fails (the positive control). | `-envban.allow=cmd/app/config.go[,…]`: module-relative files, one config file per binary. |
| errkeys | Making a key outside the registry file: a literal, an untyped constant, a conversion (constant or run time) or an operator (`A+B`) of the key type. A key constant, a variable, a parameter, a field or a call's result passes. A registry with no key constant, or missing, fails. | `-errkeys.type` (default `github.com/MohammadBnei/ukubi-go/errs.Key`), `-errkeys.registry=internal/errs/keys.go`. |
| pkgstate | A package-level `var` that is not the blank `_`, an embed into a `string` or `embed.FS`, a value of `regexp.MustCompile`, `errors.New` or `connectkeys.New`, or a table; `func init()`; any write into a package-level variable of the module (assignment, range, `++`/`--`, `&`, `delete`, `clear`, `copy`, through fields, indexes, slices and pointers); a table used other than read (indexed, ranged over, compared, `len`, `cap`, `slices.Contains`, `slices.Index`, `slices.Clone`, `maps.Clone`), also when what indexing it yields (`v := t[k]` or `v, ok := t[k]`) is a slice, map or pointer. Generated files are skipped. A table is marked `//ukubicheck:table <reason>` on the `var` (or its group), and the mark crosses packages as a fact. | none |
| loglevel | Any use of the `log` package but its `Logger` type outside the wrapper packages. A wrapper that uses no `log` fails. | `-loglevel.wrappers=internal/logs[,…]`: module-relative package directories. |

Run it beside plain `go vet`, since `-vettool` replaces vet's own analyzers, and with a fresh `GOCACHE`: vet's cache key leaves out the files the build excludes, which envban reads, so a warm cache replays a stale result for them.

```sh
go build -o "$RUNNER_TEMP/ukubicheck" github.com/MohammadBnei/ukubi-go/cmd/ukubicheck
GOCACHE="$RUNNER_TEMP/vetcache" go vet -vettool="$RUNNER_TEMP/ukubicheck" \
  -envban.allow=cmd/app/config.go -errkeys.registry=internal/errs/keys.go -loglevel.wrappers=internal/logs ./...
```

An analyzer is turned off with `-<name>=false`; this repo's own CI runs it with `-errkeys=false`, since ukubi-go's library keys have no registry. ukubi-ci's `go-checks` passes no analyzer flags today, so an app adopting the tool through it waits for a `vettool-flags` input there.

Ceilings, each a reason to keep an app's own guard until it is covered:

- A package the build excludes whole (`//go:build integration`, another GOOS, cgo with `CGO_ENABLED=0`) is never analyzed: run the tool once per tag set the app ships (`-tags=integration`), and keep a syntactic "package was not walked" test until then. The known case is the fleet's `internal/dbtest`.
- Files the build excludes in an analyzed package get envban's syntactic check (by import name) only, not keys, state or logs.
- An allowed env file is allowed whole: no per-name exception (the game's `forge.go` `os.Environ`).
- errkeys passes zero values, keys from reflection or unmarshalling, generic type-parameter conversions, and keys a dependency makes at run time (`errs.From`'s `source.code`, connectkeys' `Of`). It does not check the registry for duplicate values. A wrong `-errkeys.type` flags nothing, and its control fires only through the registry's package.
- pkgstate does not follow a write through a range's value variable (`for _, v := range tbl` with `v` a slice), through a copied struct element that holds a slice, map or pointer, or through a mutating method on a made value (`regexp.Longest`). It does not check writes from `_test.go` files, so a test that swaps a global passes.
- A positive control fires only while its package is analyzed: a mistyped wrapper directory or allowed file in a package vet never visits passes.
- The tests' `testdata` modules use `replace` directives to local stubs (an exception to standard §4's no-`replace` rule, for testdata only). Directories with a `go.mod` are left out of the module zip, so `cmd/ukubicheck`'s tests run only from a checkout, not from the module cache.

## License

MIT, see [LICENSE](LICENSE).
