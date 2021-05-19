package roxyutil

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNoHealthyBackends signals that a roxyresolver.Resolver was unable to find
// any healthy servers to talk to.
var ErrNoHealthyBackends = noHealthyBackendsError(0)

// ErrNotExist signals that something does not exist.
var ErrNotExist = notExistError(0)

// ErrFailedToMatch et al signal that input parsing has failed.
var (
	ErrFailedToMatch          = inputError("failed to match expected pattern")
	ErrExpectEmpty            = inputError("expected empty string")
	ErrExpectEmptyOrLocalhost = inputError("expected empty string or \"localhost\"")
	ErrExpectNonEmpty         = inputError("expected non-empty string")
	ErrExpectOneSlash         = inputError("expected one '/', found 2 or more")
	ErrExpectLeadingSlash     = inputError("expected path to start with '/'")
	ErrExpectTrailingSlash    = inputError("expected path to end with '/'")
	ErrExpectNoEndSlash       = inputError("did not expect path to end with '/'")
	ErrExpectNoDoubleSlash    = inputError("did not expect path to contain '//'")
	ErrExpectNoDot            = inputError("did not expect path to contain '/./'")
	ErrExpectNoDotDot         = inputError("did not expect path to contain '/../'")
)

type grpcStatusError interface {
	error
	GRPCStatus() *status.Status
}

type grpcStatusCodeError interface {
	error
	GRPCStatusCode() codes.Code
}

// type noHealthyBackendsError {{{

// noHealthyBackendsError represents failure to locate something.
type noHealthyBackendsError int

// Error fulfills the error interface.
func (err noHealthyBackendsError) Error() string {
	return "no healthy backends"
}

// GRPCStatusCode returns the GRPC status code.
func (err noHealthyBackendsError) GRPCStatusCode() codes.Code {
	return codes.Unavailable
}

var _ error = noHealthyBackendsError(0)

// }}}

// type notExistError {{{

// notExistError represents failure to locate something.
type notExistError int

// Error fulfills the error interface.
func (err notExistError) Error() string {
	return "does not exist"
}

// Is returns true for fs.ErrNotExist.
func (err notExistError) Is(other error) bool {
	return other == fs.ErrNotExist
}

// GRPCStatusCode returns the GRPC status code.
func (err notExistError) GRPCStatusCode() codes.Code {
	return codes.NotFound
}

var _ error = notExistError(0)

// }}}

// type inputError {{{

// inputError represents failure to parse an input.
type inputError string

// Error fulfills the error interface.
func (err inputError) Error() string {
	return string(err)
}

// GRPCStatusCode returns the GRPC status code.
func (err inputError) GRPCStatusCode() codes.Code {
	return codes.InvalidArgument
}

var _ error = inputError("")
var _ grpcStatusCodeError = inputError("")

// }}}

// type RegexpMatchError {{{

// RegexpMatchError represents failure to match a regular expression.
type RegexpMatchError struct {
	Input   string
	Pattern *regexp.Regexp
}

// Error fulfills the error interface.
func (err RegexpMatchError) Error() string {
	return fmt.Sprintf("input %q failed to match pattern /%s/", err.Input, err.Pattern.String())
}

// GRPCStatusCode returns the GRPC status code.
func (err RegexpMatchError) GRPCStatusCode() codes.Code {
	return codes.InvalidArgument
}

var _ error = RegexpMatchError{}

// }}}

// type BoolError {{{

// BoolError represents failure to parse the string representation of a
// boolean value.
type BoolError struct {
	Input string
	Err   error
}

// Error fulfills the error interface.
func (err BoolError) Error() string {
	return fmt.Sprintf("invalid boolean value %q: %v", err.Input, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err BoolError) Unwrap() error {
	return err.Err
}

var _ error = BoolError{}

// }}}

// type SchemeError {{{

// SchemeError represents failure to identify a URL scheme or a Target scheme.
type SchemeError struct {
	Scheme string
	Err    error
}

// Error fulfills the error interface.
func (err SchemeError) Error() string {
	return fmt.Sprintf("invalid scheme %q: %v", err.Scheme, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err SchemeError) Unwrap() error {
	return err.Err
}

var _ error = SchemeError{}

// }}}

// type AuthorityError {{{

// AuthorityError represents failure to parse a URL authority section or a
// Target authority section.
type AuthorityError struct {
	Authority string
	Err       error
}

// Error fulfills the error interface.
func (err AuthorityError) Error() string {
	return fmt.Sprintf("invalid authority %q: %v", err.Authority, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err AuthorityError) Unwrap() error {
	return err.Err
}

var _ error = AuthorityError{}

// }}}

// type EndpointError {{{

// EndpointError represents failure to parse a Target endpoint section.
type EndpointError struct {
	Endpoint string
	Err      error
}

// Error fulfills the error interface.
func (err EndpointError) Error() string {
	return fmt.Sprintf("invalid endpoint %q: %v", err.Endpoint, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err EndpointError) Unwrap() error {
	return err.Err
}

var _ error = EndpointError{}

// }}}

// type PathError {{{

// PathError represents failure to parse a path of some sort, such as a URL
// path or a ZooKeeper path.
type PathError struct {
	Path string
	Err  error
}

// Error fulfills the error interface.
func (err PathError) Error() string {
	return fmt.Sprintf("invalid path %q: %v", err.Path, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err PathError) Unwrap() error {
	return err.Err
}

var _ error = PathError{}

// }}}

// type QueryStringError {{{

// QueryStringError represents failure to parse a URL query string or a Target
// query string.
type QueryStringError struct {
	QueryString string
	Err         error
}

// Error fulfills the error interface.
func (err QueryStringError) Error() string {
	return fmt.Sprintf("invalid query string %q: %v", err.QueryString, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err QueryStringError) Unwrap() error {
	return err.Err
}

var _ error = QueryStringError{}

// }}}

// type QueryParamError {{{

// QueryParamError represents failure to parse the value of a specific URL
// query parameter or a specific Target query parameter.
type QueryParamError struct {
	Name  string
	Value string
	Err   error
}

// Error fulfills the error interface.
func (err QueryParamError) Error() string {
	return fmt.Sprintf("invalid query param %s=%q: %v", err.Name, err.Value, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err QueryParamError) Unwrap() error {
	return err.Err
}

var _ error = QueryParamError{}

// }}}

// type HostPortError {{{

// HostPortError represents failure to parse a "host:port"-shaped string.
type HostPortError struct {
	HostPort string
	Err      error
}

// Error fulfills the error interface.
func (err HostPortError) Error() string {
	return fmt.Sprintf("invalid <host>:<port> string %q: %v", err.HostPort, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err HostPortError) Unwrap() error {
	return err.Err
}

var _ error = HostPortError{}

// }}}

// type HostError {{{

// HostError represents failure to parse a hostname string.
type HostError struct {
	Host string
	Err  error
}

// Error fulfills the error interface.
func (err HostError) Error() string {
	return fmt.Sprintf("invalid hostname %q: %v", err.Host, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err HostError) Unwrap() error {
	return err.Err
}

var _ error = HostError{}

// }}}

// type IPError {{{

// IPError represents failure to parse an IP address string.
type IPError struct {
	IP  string
	Err error
}

// Error fulfills the error interface.
func (err IPError) Error() string {
	return fmt.Sprintf("invalid IP address %q: %v", err.IP, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err IPError) Unwrap() error {
	return err.Err
}

var _ error = IPError{}

// }}}

// type PortError {{{

// PortError represents failure to parse a port number string.
type PortError struct {
	Port    string
	Err     error
	NamedOK bool
}

// Error fulfills the error interface.
func (err PortError) Error() string {
	format := "invalid port number %q: %v"
	if err.NamedOK {
		format = "invalid port number or named port %q: %v"
	}
	return fmt.Sprintf(format, err.Port, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err PortError) Unwrap() error {
	return err.Err
}

var _ error = PortError{}

// }}}

// type ATCServiceNameError {{{

// ATCServiceNameError represents failure to parse an ATC service name.
type ATCServiceNameError struct {
	ServiceName string
	Err         error
}

// Error fulfills the error interface.
func (err ATCServiceNameError) Error() string {
	return fmt.Sprintf("invalid ATC service name %q: %v", err.ServiceName, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err ATCServiceNameError) Unwrap() error {
	return err.Err
}

var _ error = ATCServiceNameError{}

// }}}

// type ATCLocationError {{{

// ATCLocationError represents failure to parse an ATC location.
type ATCLocationError struct {
	Location string
	Err      error
}

// Error fulfills the error interface.
func (err ATCLocationError) Error() string {
	return fmt.Sprintf("invalid ATC location name %q: %v", err.Location, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err ATCLocationError) Unwrap() error {
	return err.Err
}

var _ error = ATCLocationError{}

// }}}

// type ATCUniqueError {{{

// ATCUniqueError represents failure to parse an ATC unique client ID or
// unique server ID.
type ATCUniqueError struct {
	Unique string
	Err    error
}

// Error fulfills the error interface.
func (err ATCUniqueError) Error() string {
	return fmt.Sprintf("invalid ATC unique identifier %q: %v", err.Unique, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err ATCUniqueError) Unwrap() error {
	return err.Err
}

var _ error = ATCUniqueError{}

// }}}

// type EnvVarLookupError {{{

// EnvVarLookupError represents failure to look up an environment variable.
type EnvVarLookupError struct {
	Var string
	Err error
}

// Error fulfills the error interface.
func (err EnvVarLookupError) Error() string {
	return fmt.Sprintf("invalid environment variable ${%s}: %v", err.Var, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err EnvVarLookupError) Unwrap() error {
	return err.Err
}

var _ error = EnvVarLookupError{}

// }}}

// type LookupUserByIDError {{{

// LookupUserByIDError represents failure to look up an OS user by UID.
type LookupUserByIDError struct {
	ID  uint32
	Err error
}

// Error fulfills the error interface.
func (err LookupUserByIDError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupId(%d) failed: %v", err.ID, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err LookupUserByIDError) Unwrap() error {
	return err.Err
}

var _ error = LookupUserByIDError{}

// }}}

// type LookupUserByNameError {{{

// LookupUserByNameError represents failure to look up an OS user by name.
type LookupUserByNameError struct {
	Name string
	Err  error
}

// Error fulfills the error interface.
func (err LookupUserByNameError) Error() string {
	if err.Name == "" {
		return fmt.Sprintf("\"os/user\".Current() failed: %v", err.Err)
	}
	return fmt.Sprintf("\"os/user\".Lookup(%q) failed: %v", err.Name, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err LookupUserByNameError) Unwrap() error {
	return err.Err
}

var _ error = LookupUserByNameError{}

// }}}

// type LookupGroupByIDError {{{

// LookupGroupByIDError represents failure to look up an OS group by GID.
type LookupGroupByIDError struct {
	ID  uint32
	Err error
}

// Error fulfills the error interface.
func (err LookupGroupByIDError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupGroupId(%d) failed: %v", err.ID, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err LookupGroupByIDError) Unwrap() error {
	return err.Err
}

var _ error = LookupGroupByIDError{}

// }}}

// type LookupGroupByNameError {{{

// LookupGroupByNameError represents failure to look up an OS group by name.
type LookupGroupByNameError struct {
	Name string
	Err  error
}

// Error fulfills the error interface.
func (err LookupGroupByNameError) Error() string {
	return fmt.Sprintf("\"os/user\".LookupGroup(%q) failed: %v", err.Name, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err LookupGroupByNameError) Unwrap() error {
	return err.Err
}

var _ error = LookupGroupByNameError{}

// }}}

// type PathAbsError {{{

// PathAbsError represents failure to make a file path absolute.
type PathAbsError struct {
	Path string
	Err  error
}

// Error fulfills the error interface.
func (err PathAbsError) Error() string {
	return fmt.Sprintf("failed to make path absolute: %q: %v", err.Path, err.Err)
}

// Unwrap returns the underlying cause of this error.
func (err PathAbsError) Unwrap() error {
	return err.Err
}

var _ error = PathAbsError{}

// }}}

// type GRPCStatusError {{{

// GRPCStatusError represents an error with an associated GRPC status code.
type GRPCStatusError struct {
	Code codes.Code
	Err  error
}

// MakeGRPCStatusError attempts to autodetect the correct status code.
func MakeGRPCStatusError(err error) GRPCStatusError {
	if err == nil {
		panic(errors.New("err is nil"))
	}

	if err0, ok := err.(GRPCStatusError); ok {
		return err0
	}

	var err1 GRPCStatusError
	if errors.As(err, &err1) {
		return err1
	}

	var err2 grpcStatusError
	if errors.As(err, &err2) {
		s := err2.GRPCStatus()
		code := s.Code()
		return GRPCStatusError{Code: code, Err: err2}
	}

	var err3 grpcStatusCodeError
	code := codes.Unknown
	switch {
	case errors.As(err, &err3):
		code = err3.GRPCStatusCode()
	case errors.Is(err, fs.ErrInvalid):
		code = codes.InvalidArgument
	case errors.Is(err, fs.ErrPermission):
		code = codes.PermissionDenied
	case errors.Is(err, fs.ErrNotExist):
		code = codes.NotFound
	case errors.Is(err, fs.ErrExist):
		code = codes.AlreadyExists
	case errors.Is(err, fs.ErrClosed):
		code = codes.FailedPrecondition
	case errors.Is(err, context.Canceled):
		code = codes.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	}
	return GRPCStatusError{Code: code, Err: err}
}

// Error fulfills the error interface.
func (err GRPCStatusError) Error() string {
	return err.Err.Error()
}

// GRPCStatus returns this error's representation as a gRPC Status.
func (err GRPCStatusError) GRPCStatus() *status.Status {
	return status.New(err.Code, err.Error())
}

// Unwrap returns the underlying cause of this error.
func (err GRPCStatusError) Unwrap() error {
	return err.Err
}

var _ error = GRPCStatusError{}
var _ grpcStatusError = GRPCStatusError{}

// }}}
