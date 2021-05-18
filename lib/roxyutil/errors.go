package roxyutil

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNoHealthyBackends signals that a roxyresolver.Resolver was unable to find
// any healthy servers to talk to.
var ErrNoHealthyBackends = errors.New("no healthy backends")

// These are error constants returned by various Roxy libraries.
var (
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

// FailedMatchError represents failure to match a regular expression.
type FailedMatchError struct {
	Input   string
	Pattern *regexp.Regexp
}

// Error fulfills the error interface.
func (err FailedMatchError) Error() string {
	return fmt.Sprintf("input %q failed to match pattern /%s/", err.Input, err.Pattern.String())
}

var _ error = FailedMatchError{}

// }}}

// type BoolParseError {{{

// BoolParseError represents failure to parse the string representation of a
// boolean value.
type BoolParseError struct {
	Input string
}

// Error fulfills the error interface.
func (err BoolParseError) Error() string {
	return fmt.Sprintf("input %q does not resemble a boolean value", err.Input)
}

var _ error = BoolParseError{}

// }}}

// type NotFoundError {{{

// NotFoundError represents failure to find something.
type NotFoundError struct {
	Key string
}

// Error fulfills the error interface.
func (err NotFoundError) Error() string {
	return fmt.Sprintf("not found: %q", err.Key)
}

// GRPCStatus returns this error's representation as a gRPC Status.  It is
// detected by status.FromError.
func (err NotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, err.Error())
}

// Unwrap returns fs.ErrNotExist, so that errors.Is works.
func (err NotFoundError) Unwrap() error {
	return fs.ErrNotExist
}

var _ error = NotFoundError{}

// }}}

// type BadSchemeError {{{

// BadSchemeError represents failure to identify a URL scheme or a RoxyTarget
// scheme.
type BadSchemeError struct {
	Scheme string
	Err    error
}

// Error fulfills the error interface.
func (err BadSchemeError) Error() string {
	return fmt.Sprintf("invalid scheme %q: %v", err.Scheme, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadSchemeError) Unwrap() error {
	return err.Err
}

var _ error = BadSchemeError{}

// }}}

// type BadAuthorityError {{{

// BadAuthorityError represents failure to parse a URL authority section or a
// RoxyTarget authority section.
type BadAuthorityError struct {
	Authority string
	Err       error
}

// Error fulfills the error interface.
func (err BadAuthorityError) Error() string {
	return fmt.Sprintf("invalid authority %q: %v", err.Authority, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadAuthorityError) Unwrap() error {
	return err.Err
}

var _ error = BadAuthorityError{}

// }}}

// type BadEndpointError {{{

// BadEndpointError represents failure to parse a RoxyTarget endpoint section.
type BadEndpointError struct {
	Endpoint string
	Err      error
}

// Error fulfills the error interface.
func (err BadEndpointError) Error() string {
	return fmt.Sprintf("invalid endpoint %q: %v", err.Endpoint, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadEndpointError) Unwrap() error {
	return err.Err
}

var _ error = BadEndpointError{}

// }}}

// type BadPathError {{{

// BadPathError represents failure to parse a path of some sort, such as a URL
// path or a ZooKeeper path.
type BadPathError struct {
	Path string
	Err  error
}

// Error fulfills the error interface.
func (err BadPathError) Error() string {
	return fmt.Sprintf("invalid path %q: %v", err.Path, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadPathError) Unwrap() error {
	return err.Err
}

var _ error = BadPathError{}

// }}}

// type BadQueryStringError {{{

// BadQueryStringError represents failure to parse a URL query string or a
// RoxyTarget query string.
type BadQueryStringError struct {
	QueryString string
	Err         error
}

// Error fulfills the error interface.
func (err BadQueryStringError) Error() string {
	return fmt.Sprintf("invalid query string %q: %v", err.QueryString, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadQueryStringError) Unwrap() error {
	return err.Err
}

var _ error = BadQueryStringError{}

// }}}

// type BadQueryParamError {{{

// BadQueryParamError represents failure to parse the value of a specific URL
// query parameter or a specific RoxyTarget query parameter.
type BadQueryParamError struct {
	Name  string
	Value string
	Err   error
}

// Error fulfills the error interface.
func (err BadQueryParamError) Error() string {
	return fmt.Sprintf("invalid query param %s=%q: %v", err.Name, err.Value, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadQueryParamError) Unwrap() error {
	return err.Err
}

var _ error = BadQueryParamError{}

// }}}

// type BadHostPortError {{{

// BadHostPortError represents failure to parse a "host:port"-shaped string.
type BadHostPortError struct {
	HostPort string
	Err      error
}

// Error fulfills the error interface.
func (err BadHostPortError) Error() string {
	return fmt.Sprintf("invalid <host>:<port> string %q: %v", err.HostPort, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadHostPortError) Unwrap() error {
	return err.Err
}

var _ error = BadHostPortError{}

// }}}

// type BadHostError {{{

// BadHostError represents failure to parse a hostname string.
type BadHostError struct {
	Host string
	Err  error
}

// Error fulfills the error interface.
func (err BadHostError) Error() string {
	return fmt.Sprintf("invalid hostname %q: %v", err.Host, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadHostError) Unwrap() error {
	return err.Err
}

var _ error = BadHostError{}

// }}}

// type BadIPError {{{

// BadIPError represents failure to parse an IP address string.
type BadIPError struct {
	IP string
}

// Error fulfills the error interface.
func (err BadIPError) Error() string {
	return fmt.Sprintf("invalid IP address %q", err.IP)
}

var _ error = BadIPError{}

// }}}

// type BadPortError {{{

// BadPortError represents failure to parse a port number string.
type BadPortError struct {
	Port    string
	Err     error
	NamedOK bool
}

// Error fulfills the error interface.
func (err BadPortError) Error() string {
	format := "invalid port number %q: %v"
	if err.NamedOK {
		format = "invalid port number or named port %q: %v"
	}
	return fmt.Sprintf(format, err.Port, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadPortError) Unwrap() error {
	return err.Err
}

var _ error = BadPortError{}

// }}}

// type BadATCServiceNameError {{{

// BadATCServiceNameError represents failure to parse an ATC service name.
type BadATCServiceNameError struct {
	ServiceName string
	Err         error
}

// Error fulfills the error interface.
func (err BadATCServiceNameError) Error() string {
	return fmt.Sprintf("invalid ATC service name %q: %v", err.ServiceName, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadATCServiceNameError) Unwrap() error {
	return err.Err
}

var _ error = BadATCServiceNameError{}

// }}}

// type BadATCLocationError {{{

// BadATCLocationError represents failure to parse an ATC location.
type BadATCLocationError struct {
	Location string
	Err      error
}

// Error fulfills the error interface.
func (err BadATCLocationError) Error() string {
	return fmt.Sprintf("invalid ATC location name %q: %v", err.Location, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadATCLocationError) Unwrap() error {
	return err.Err
}

var _ error = BadATCLocationError{}

// }}}

// type BadATCUniqueError {{{

// BadATCUniqueError represents failure to parse an ATC unique client ID or
// unique server ID.
type BadATCUniqueError struct {
	Unique string
	Err    error
}

// Error fulfills the error interface.
func (err BadATCUniqueError) Error() string {
	return fmt.Sprintf("invalid ATC unique identifier %q: %v", err.Unique, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BadATCUniqueError) Unwrap() error {
	return err.Err
}

var _ error = BadATCUniqueError{}

// }}}

// type EnvVarNotFoundError {{{

// EnvVarNotFoundError represents failure to expand an environment variable
// reference because the named environment variable does not exist.
type EnvVarNotFoundError struct {
	Var string
}

// Error fulfills the error interface.
func (err EnvVarNotFoundError) Error() string {
	return fmt.Sprintf("no such environment variable ${%s}", err.Var)
}

// GRPCStatus returns this error's representation as a gRPC Status.  It is
// detected by status.FromError.
func (err EnvVarNotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, err.Error())
}

// Unwrap returns fs.ErrNotExist, so that errors.Is works.
func (err EnvVarNotFoundError) Unwrap() error {
	return fs.ErrNotExist
}

var _ error = EnvVarNotFoundError{}

// }}}

// type UserIDLookupError {{{

// UserIDLookupError represents failure to look up an OS user by UID.
type UserIDLookupError struct {
	ID  uint32
	Err error
}

// Error fulfills the error interface.
func (err UserIDLookupError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupId(%d) failed: %v", err.ID, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err UserIDLookupError) Unwrap() error {
	return err.Err
}

var _ error = UserIDLookupError{}

// }}}

// type UserNameLookupError {{{

// UserNameLookupError represents failure to look up an OS user by name.
type UserNameLookupError struct {
	Name string
	Err  error
}

// Error fulfills the error interface.
func (err UserNameLookupError) Error() string {
	if err.Name == "" {
		return fmt.Sprintf("\"os/user\".Current() failed: %v", err.Err)
	}
	return fmt.Sprintf("\"os/user\".Lookup(%q) failed: %v", err.Name, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err UserNameLookupError) Unwrap() error {
	return err.Err
}

var _ error = UserNameLookupError{}

// }}}

// type GroupIDLookupError {{{

// GroupIDLookupError represents failure to look up an OS group by GID.
type GroupIDLookupError struct {
	ID  uint32
	Err error
}

// Error fulfills the error interface.
func (err GroupIDLookupError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupGroupId(%d) failed: %v", err.ID, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err GroupIDLookupError) Unwrap() error {
	return err.Err
}

var _ error = GroupIDLookupError{}

// }}}

// type GroupNameLookupError {{{

// GroupNameLookupError represents failure to look up an OS group by name.
type GroupNameLookupError struct {
	Name string
	Err  error
}

// Error fulfills the error interface.
func (err GroupNameLookupError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupGroup(%q) failed: %v", err.Name, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err GroupNameLookupError) Unwrap() error {
	return err.Err
}

var _ error = GroupNameLookupError{}

// }}}

// type FailedPathAbsError {{{

// FailedPathAbsError represents failure to make a file path absolute.
type FailedPathAbsError struct {
	Path string
	Err  error
}

// Error fulfills the error interface.
func (err FailedPathAbsError) Error() string {
	return fmt.Sprintf("failed to make path absolute: %q: %v", err.Path, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err FailedPathAbsError) Unwrap() error {
	return err.Err
}

var _ error = FailedPathAbsError{}

// }}}
