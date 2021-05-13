package mainutil

import (
	_ "embed"
	"strings"
)

//go:embed version.txt
var roxyVersion string

var appVersion string = "unset"

// RoxyVersion returns the version of Roxy itself.
func RoxyVersion() string {
	return strings.Trim(roxyVersion, " \t\r\n")
}

// SetAppVersion changes the application version.
func SetAppVersion(version string) {
	appVersion = strings.Trim(version, " \t\r\n")
}

// AppVersion returns the application version.
func AppVersion() string {
	return appVersion
}
