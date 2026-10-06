// Package assets meng-embed template & file statis ke dalam binary.
package assets

import "embed"

//go:embed templates static
var FS embed.FS
