package misc

import (
	"encoding/base64"
	"fmt"

	multierror "github.com/hashicorp/go-multierror"
)

// TryBase64DecodeString attempts to decode base-64 byte array using any of the
// standard base-64 encodings.
func TryBase64DecodeString(str string) ([]byte, error) {
	if str == "" {
		return nil, nil
	}

	var errs multierror.Error
	errs.Errors = make([]error, 0, 4)

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
		errs.Errors = append(errs.Errors, err)
	}

	err := ErrorOrNil(errs)
	return nil, Base64DecodeError{Input: str, Err: err}
}

// type Base64DecodeError {{{

// Base64DecodeError represents failure to convert a base-64 string to []byte.
type Base64DecodeError struct {
	Input string
	Err   error
}

// Error fulfills the error interface.
func (err Base64DecodeError) Error() string {
	input := err.Input
	if len(input) > 40 {
		input = input[:37] + "..."
	}
	return fmt.Sprintf("failed to decode base-64 string %q: %v", input, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err Base64DecodeError) Unwrap() error {
	return err.Err
}

var _ error = Base64DecodeError{}

// }}}
