# ukubi-go

Shared Go packages for ukubi apps: the error model, telemetry (traces and JSON logs), and the checks that keep apps on the ukubi Code Standard.

Module: `github.com/MohammadBnei/ukubi-go`

Packages are added by pull request; each one is listed here with its import path when it lands.

## Packages

| Package | Import path | What it is |
| --- | --- | --- |
| errs | `github.com/MohammadBnei/ukubi-go/errs` | The error model: a Connect code, a stable key, and the JSON error body with its HTTP status. |
| telemetry | `github.com/MohammadBnei/ukubi-go/telemetry` | Traces over OTLP/HTTP when an endpoint is set, and the JSON logger with trace ids. |

With an endpoint, `telemetry.New` sets OTel's error handler, so errors reported through `otel.Handle` (export failures included) log as slog lines at WARN. OTel's own config and limit warnings still print as plain text on stderr.

They came from `mountain/pkg`, with their history.

## License

MIT, see [LICENSE](LICENSE).
