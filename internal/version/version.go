// Package version holds the build-time version identity for Muninn and
// muninnctl. Set via -ldflags -X at build time; left as "dev" for local
// `go build`/`go run` invocation that don't pass it.
package version

var Version = "dev"
