package main

import (
	_ "embed"
	"strings"
)

//go:embed ".version"
var versionString string

func Version() string {
	return strings.Trim(versionString, " \t\r\n")
}
