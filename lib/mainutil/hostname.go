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
			os.Setenv("HOSTNAME", hostname)
		} else {
			os.Setenv("HOSTNAME", "localhost")
		}
	}
}
