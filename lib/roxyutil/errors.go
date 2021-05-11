package roxyutil

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// type FailedMatchError {{{

type FailedMatchError struct {
	Input   string
	Pattern *regexp.Regexp
}

func (err FailedMatchError) Error() string {
	return fmt.Sprintf("input %q failed to match pattern /%s/", err.Input, err.Pattern.String())
}

var _ error = FailedMatchError{}

// }}}

// type BadBoolError {{{

type BadBoolError struct {
	Input string
}

func (err BadBoolError) Error() string {
	return fmt.Sprintf("input %q does not resemble a boolean value", err.Input)
}

var _ error = BadBoolError{}

// }}}

// type NotFoundError {{{

type NotFoundError struct {
	Key string
}

func (err NotFoundError) Error() string {
	return fmt.Sprintf("not found: %q", err.Key)
}

func (err NotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, err.Error())
}

func (err NotFoundError) Is(target error) bool {
	return target == fs.ErrNotExist
}

var _ error = NotFoundError{}

// }}}

// type BadSchemeError {{{

type BadSchemeError struct {
	Scheme string
	Err    error
}

func (err BadSchemeError) Error() string {
	return fmt.Sprintf("invalid scheme %q: %v", err.Scheme, err.Err)
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
	return fmt.Sprintf("invalid authority %q: %v", err.Authority, err.Err)
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
	return fmt.Sprintf("invalid endpoint %q: %v", err.Endpoint, err.Err)
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
	Port    string
	Err     error
	NamedOK bool
}

func (err BadPortError) Error() string {
	format := "invalid port number %q: %v"
	if err.NamedOK {
		format = "invalid port number or named port %q: %v"
	}
	return fmt.Sprintf(format, err.Port, err.Err)
}

func (err BadPortError) Unwrap() error {
	return err.Err
}

var _ error = BadPortError{}

// }}}

// type BadATCServiceNameError {{{

type BadATCServiceNameError struct {
	ServiceName string
	Err         error
}

func (err BadATCServiceNameError) Error() string {
	return fmt.Sprintf("invalid ATC service name %q: %v", err.ServiceName, err.Err)
}

func (err BadATCServiceNameError) Unwrap() error {
	return err.Err
}

var _ error = BadATCServiceNameError{}

// }}}

// type BadATCLocationError {{{

type BadATCLocationError struct {
	Location string
	Err      error
}

func (err BadATCLocationError) Error() string {
	return fmt.Sprintf("invalid ATC location name %q: %v", err.Location, err.Err)
}

func (err BadATCLocationError) Unwrap() error {
	return err.Err
}

var _ error = BadATCLocationError{}

// }}}

// type BadATCUniqueError {{{

type BadATCUniqueError struct {
	Unique string
	Err    error
}

func (err BadATCUniqueError) Error() string {
	return fmt.Sprintf("invalid ATC unique identifier %q: %v", err.Unique, err.Err)
}

func (err BadATCUniqueError) Unwrap() error {
	return err.Err
}

var _ error = BadATCUniqueError{}

// }}}

// type BadEnvVarError {{{

type BadEnvVarError struct {
	Var string
}

func (err BadEnvVarError) Error() string {
	return fmt.Sprintf("no such environment variable ${%s}", err.Var)
}

func (err BadEnvVarError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, err.Error())
}

func (err BadEnvVarError) Is(target error) bool {
	return target == fs.ErrNotExist
}

var _ error = BadEnvVarError{}

// }}}

// type FailedUserNameLookupError {{{

type FailedUserNameLookupError struct {
	Name string
	Err  error
}

func (err FailedUserNameLookupError) Error() string {
	if err.Name == "" {
		return fmt.Sprintf("\"os/user\".Current() failed: %v", err.Err)
	}
	return fmt.Sprintf("\"os/user\".Lookup(%q) failed: %v", err.Name, err.Err)
}

func (err FailedUserNameLookupError) Unwrap() error {
	return err.Err
}

var _ error = FailedUserNameLookupError{}

// }}}

// type FailedGroupNameLookupError {{{

type FailedGroupNameLookupError struct {
	Name string
	Err  error
}

func (err FailedGroupNameLookupError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupGroup(%q) failed: %v", err.Name, err.Err)
}

func (err FailedGroupNameLookupError) Unwrap() error {
	return err.Err
}

var _ error = FailedGroupNameLookupError{}

// }}}

// type FailedPathAbsError {{{

type FailedPathAbsError struct {
	Path string
	Err  error
}

func (err FailedPathAbsError) Error() string {
	return fmt.Sprintf("failed to make path absolute: %q: %v", err.Path, err.Err)
}

func (err FailedPathAbsError) Unwrap() error {
	return err.Err
}

var _ error = FailedPathAbsError{}

// }}}
