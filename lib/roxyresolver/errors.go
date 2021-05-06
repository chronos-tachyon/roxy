package roxyresolver

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/chronos-tachyon/roxy/lib/membership"
)

var (
	ErrNoHealthyBackends      = errors.New("no healthy backends")
	ErrExpectEmpty            = errors.New("expected empty string")
	ErrExpectEmptyOrLocalhost = errors.New("expected empty string or \"localhost\"")
	ErrExpectNonEmpty         = errors.New("expected non-empty string")
	ErrExpectOneSlash         = errors.New("expected one '/', found 2 or more")
	ErrExpectLeadingSlash     = errors.New("expected path to start with '/'")
	ErrExpectTrailingSlash    = errors.New("expected path to end with '/'")
	ErrExpectNoEndSlash       = errors.New("did not expect path to end with '/'")
	ErrExpectNoDoubleSlash    = errors.New("did not expect path to contain '//'")
	ErrExpectNoDot            = errors.New("did not expect path to contain '/./'")
	ErrExpectNoDotDot         = errors.New("did not expect path to contain '/../'")
	ErrFailedToMatch          = errors.New("failed to match expected pattern")
)

// type BadSchemeError {{{

type BadSchemeError struct {
	Scheme string
	Err    error
}

func (err BadSchemeError) Error() string {
	return fmt.Sprintf("invalid Target.Scheme %q: %v", err.Scheme, err.Err)
}

func (err BadSchemeError) Unwrap() error {
	return err.Err
}

var _ error = BadSchemeError{}

// }}}

// type BadAuthorityError {{{

type BadAuthorityError struct {
	Authority string
	Err       error
}

func (err BadAuthorityError) Error() string {
	return fmt.Sprintf("invalid Target.Authority %q: %v", err.Authority, err.Err)
}

func (err BadAuthorityError) Unwrap() error {
	return err.Err
}

var _ error = BadAuthorityError{}

// }}}

// type BadEndpointError {{{

type BadEndpointError struct {
	Endpoint string
	Err      error
}

func (err BadEndpointError) Error() string {
	return fmt.Sprintf("invalid Target.Endpoint %q: %v", err.Endpoint, err.Err)
}

func (err BadEndpointError) Unwrap() error {
	return err.Err
}

var _ error = BadEndpointError{}

// }}}

// type BadPathError {{{

type BadPathError struct {
	Path string
	Err  error
}

func (err BadPathError) Error() string {
	return fmt.Sprintf("invalid path %q: %v", err.Path, err.Err)
}

func (err BadPathError) Unwrap() error {
	return err.Err
}

var _ error = BadPathError{}

// }}}

// type BadQueryStringError {{{

type BadQueryStringError struct {
	QueryString string
	Err         error
}

func (err BadQueryStringError) Error() string {
	return fmt.Sprintf("invalid query string %q: %v", err.QueryString, err.Err)
}

func (err BadQueryStringError) Unwrap() error {
	return err.Err
}

var _ error = BadQueryStringError{}

// }}}

// type BadQueryParamError {{{

type BadQueryParamError struct {
	Name  string
	Value string
	Err   error
}

func (err BadQueryParamError) Error() string {
	return fmt.Sprintf("invalid query param %s=%q: %v", err.Name, err.Value, err.Err)
}

func (err BadQueryParamError) Unwrap() error {
	return err.Err
}

var _ error = BadQueryParamError{}

// }}}

// type BadHostPortError {{{

type BadHostPortError struct {
	HostPort string
	Err      error
}

func (err BadHostPortError) Error() string {
	return fmt.Sprintf("invalid <host>:<port> string %q: %v", err.HostPort, err.Err)
}

func (err BadHostPortError) Unwrap() error {
	return err.Err
}

var _ error = BadHostPortError{}

// }}}

// type BadHostError {{{

type BadHostError struct {
	Host string
	Err  error
}

func (err BadHostError) Error() string {
	return fmt.Sprintf("invalid hostname %q: %v", err.Host, err.Err)
}

func (err BadHostError) Unwrap() error {
	return err.Err
}

var _ error = BadHostError{}

// }}}

// type BadIPError {{{

type BadIPError struct {
	IP string
}

func (err BadIPError) Error() string {
	return fmt.Sprintf("invalid IP address %q", err.IP)
}

var _ error = BadIPError{}

// }}}

// type BadPortError {{{

type BadPortError struct {
	Port string
	Err  error
}

func (err BadPortError) Error() string {
	return fmt.Sprintf("invalid port %q: %v", err.Port, err.Err)
}

func (err BadPortError) Unwrap() error {
	return err.Err
}

var _ error = BadPortError{}

// }}}

// type BadServiceNameError {{{

type BadServiceNameError struct {
	ServiceName string
	Err         error
}

func (err BadServiceNameError) Error() string {
	return fmt.Sprintf("invalid load balancer target name %q: %v", err.ServiceName, err.Err)
}

func (err BadServiceNameError) Unwrap() error {
	return err.Err
}

var _ error = BadServiceNameError{}

// }}}

// type BadLocationError {{{

type BadLocationError struct {
	Location string
	Err      error
}

func (err BadLocationError) Error() string {
	return fmt.Sprintf("invalid load balancer target name %q: %v", err.Location, err.Err)
}

func (err BadLocationError) Unwrap() error {
	return err.Err
}

var _ error = BadLocationError{}

// }}}

// type BadStatusError {{{

type BadStatusError struct {
	Status membership.ServerSetStatus
}

func (err BadStatusError) Error() string {
	return fmt.Sprintf("status is %v", err.Status)
}

func (err BadStatusError) Is(other error) bool {
	switch other {
	case err:
		return true
	case fs.ErrNotExist:
		return true
	default:
		return false
	}
}

var _ error = BadStatusError{}

// }}}

// type childExitError {{{

type childExitError struct {
	Path string
}

func (err childExitError) Error() string {
	return fmt.Sprintf("child thread for path %q has exited", err.Path)
}

var _ error = childExitError{}

// }}}
