package roxyutil

import (
	"regexp"
)

var (
	reSSPort      = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[._+-][0-9A-Za-z]+)*$`)
	reATCService  = regexp.MustCompile(`^[A-Za-z][0-9A-Za-z]*(?:[._-][0-9A-Za-z]+)*$`)
	reATCLocation = regexp.MustCompile(`^(?:[A-Za-z]+:)?[A-Za-z][0-9A-Za-z]*(?:-[0-9A-Za-z]+)*$`)
	reATCUnique   = regexp.MustCompile(`^[0-9A-Za-z+/=]+$`)
)
