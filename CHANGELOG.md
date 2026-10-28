# Changelog

## 0.1.0 - 2026-10-28

### Features

- **pkg:** The error model as a nested module, mountain/pkg
- **pkg:** pkg/telemetry, the game's traces moved unchanged
- **pkg:** NewLogger, JSON lines with trace ids; errs.Write logs at ERROR
- Carry errs and telemetry from mountain pkg/ at a53488e576e5c8845183197d9ce4331ce5db4a38
- **telemetry:** Export failures log at WARN
- **errs:** connectkeys, Connect/gRPC error keys with an outermost fallback interceptor (#2)
- **proto:** ukubi.errors.v1.ErrorKey with buf lint, breaking and drift checks (#3)
- **ukubicheck:** go/analysis analyzer for env reads, error keys, package state and log levels (#4)

### Fixes

- **errs:** Nil causes, typed-nil errors, shared Wrap and big bodies
- **telemetry:** Reject an endpoint that is not an http(s) URL

### Other

- Bootstrap ukubi-go with README, MIT license and .gitignore
- golangci-lint, go test -race and a trailer check
- Module path github.com/MohammadBnei/ukubi-go
- go vet, go test -race and gofmt on GitHub-hosted runners
- List the errs and telemetry packages with their import paths
- **telemetry:** Say what a second SetErrorHandler does
- **telemetry:** Drop the private standard reference from the error handler doc
