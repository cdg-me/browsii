package sdk

import "embed"

// FS contains the reference guest SDK files (e.g. sdk/go).
// Used by the CLI to dynamically write the standard environments necessary to compile WASM.
//
//go:embed go/*
var FS embed.FS
