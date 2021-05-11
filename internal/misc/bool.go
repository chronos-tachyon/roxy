package misc

import (
	"strings"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

var boolMap = map[string]bool{
	"0": false,
	"1": true,

	"off": false,
	"on":  true,

	"n":   false,
	"no":  false,
	"y":   true,
	"yes": true,

	"f":     false,
	"false": false,
	"t":     true,
	"true":  true,
}

func ParseBool(str string) (bool, error) {
	if value, found := boolMap[str]; found {
		return value, nil
	}
	for name, value := range boolMap {
		if strings.EqualFold(str, name) {
			return value, nil
		}
	}
	return false, roxyutil.BadBoolError{Input: str}
}
