// Package dist contains embedded copies of files distributed with Roxy.
package dist

import _ "embed"

//go:embed roxy.mime.json
var defaultMimeJSON []byte

// DefaultMimeJSON returns the contents of "dist/roxy.mime.json".
func DefaultMimeJSON() []byte {
	return defaultMimeJSON
}
