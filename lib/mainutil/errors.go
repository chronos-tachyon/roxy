package mainutil

import (
	"errors"
	"fmt"
)

// ErrOperationFailed is a generic error for any failed operation.  Details
// should be provided via error wrapping.
var ErrOperationFailed = errors.New("operation failed")

// type OptionError {{{

// OptionError indicates an error while parsing options.
type OptionError struct {
	Name     string
	Value    string
	Err      error
	Complete bool
}

// Error fulfills the error interface.
func (err OptionError) Error() string {
	str := err.Name
	if err.Complete {
		str = err.Name + "=" + err.Value
	}
	return fmt.Sprintf("option %q: %v", str, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err OptionError) Unwrap() error {
	return err.Err
}

var _ error = OptionError{}

// }}}

// type UnknownOptionError {{{

// UnknownOptionError indicates that an unknown option was encountered.
type UnknownOptionError struct{}

// Error fulfills the error interface.
func (UnknownOptionError) Error() string {
	return "unknown option name"
}

var _ error = UnknownOptionError{}

// }}}

// type MissingOptionValueError {{{

// MissingOptionValueError indicates that an option was found with the value
// elided, but eliding the value doesn't make sense for that option.
type MissingOptionValueError struct{}

// Error fulfills the error interface.
func (MissingOptionValueError) Error() string {
	return "must specify a value"
}

var _ error = MissingOptionValueError{}

// }}}

// type CertPoolError {{{

// CertPoolError indicates a problem while loading an X.509 certificate pool.
type CertPoolError struct {
	Path string
	Err  error
}

// Error fulfills the error interface.
func (err CertPoolError) Error() string {
	if err.Path == "" {
		return fmt.Sprintf("failed to load system X.509 certificate pool: %v", err.Err)
	}
	return fmt.Sprintf("failed to load X.509 certificate pool from file: %q: %v", err.Path, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err CertPoolError) Unwrap() error {
	return err.Err
}

var _ error = CertPoolError{}

// }}}

// type CertKeyError {{{

// CertKeyError indicates a problem while loading an X.509 certificate/key pair.
type CertKeyError struct {
	CertPath string
	KeyPath  string
	Err      error
}

// Error fulfills the error interface.
func (err CertKeyError) Error() string {
	if err.CertPath == err.KeyPath {
		return fmt.Sprintf("failed to load X.509 cert and key from file: %q: %v", err.CertPath, err.Err)
	}
	return fmt.Sprintf("failed to load X.509 cert and key from files: cert=%q key=%q: %v", err.CertPath, err.KeyPath, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err CertKeyError) Unwrap() error {
	return err.Err
}

var _ error = CertKeyError{}

// }}}

// type ZKAddAuthError {{{

// ZKAddAuthError indicates a problem while authenticating to ZooKeeper.
type ZKAddAuthError struct {
	AuthScheme string
	Err        error
}

// Error fulfills the error interface.
func (err ZKAddAuthError) Error() string {
	return fmt.Sprintf("failed to (*zk.Conn).AddAuth with scheme=%q: %v", err.AuthScheme, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err ZKAddAuthError) Unwrap() error {
	return err.Err
}

var _ error = ZKAddAuthError{}

// }}}
