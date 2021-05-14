package mainutil

import (
	"os"
	"strings"
)

func init() {
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname, err := os.Hostname()
		if err == nil {
			hostname = strings.TrimRight(hostname, ".")
			_ = os.Setenv("HOSTNAME", hostname)
		} else {
			_ = os.Setenv("HOSTNAME", "localhost")
		}
	}
}
