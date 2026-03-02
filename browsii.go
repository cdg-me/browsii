// Package browsii exposes embedded documentation for use by the CLI.
package browsii

import _ "embed"

// Quickstart is the content of QUICKSTART.md, embedded at build time.
//
//go:embed QUICKSTART.md
var Quickstart string
