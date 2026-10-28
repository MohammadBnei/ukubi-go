// Command ukubicheck holds ukubi apps to the Code Standard's source rules, as a go vet tool:
//
//	go build -o ukubicheck github.com/MohammadBnei/ukubi-go/cmd/ukubicheck
//	go vet -vettool=./ukubicheck -envban.allow=cmd/app/config.go -errkeys.registry=internal/errs/keys.go ./...
//
// Its analyzers are envban (only the config file reads the environment), errkeys (a key is one of the registry's
// constants), pkgstate (no mutable package-level state) and loglevel (no log package outside the wrappers). Each
// type-checks the code, skips _test.go files, and with its flags empty bans everywhere. -vettool replaces go vet's own
// analyzers, so run plain go vet too. See the README for the flags and the ceilings.
package main

import "golang.org/x/tools/go/analysis/unitchecker"

func main() { unitchecker.Main(envBan(), errKeys(), pkgState(), logLevel()) }
