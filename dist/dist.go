// Package dist contains embedded copies of files distributed with Roxy.
package dist

import _ "embed"

//go:embed roxy.mime.json
var DefaultMimeJSON string
