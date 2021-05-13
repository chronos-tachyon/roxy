package misc

import (
	"encoding/base64"
	"fmt"
)

func TryBase64DecodeString(str string) ([]byte, error) {
	if str == "" {
		return nil, nil
	}

	var firstErr error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	} {
		raw, err := enc.DecodeString(str)
		if err == nil {
			return raw, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, fmt.Errorf("failed to decode base-64 string %q: %w", str, firstErr)
}
