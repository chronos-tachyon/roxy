package mainutil

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var versionString string

func Version() string {
	return strings.Trim(versionString, " \t\r\n")
}
