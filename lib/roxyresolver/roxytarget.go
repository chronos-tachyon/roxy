package roxyresolver

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"sort"
	"strings"

	"google.golang.org/grpc/resolver"

	"github.com/chronos-tachyon/roxy/internal/constants"
	"github.com/chronos-tachyon/roxy/internal/misc"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// Target represents a parsed Resolver target name.
type Target struct {
	// Scheme indicates which Resolver implementation to use.
	//
	// If not specified, "dns" is assumed.
	Scheme string

	// Authority provides information about which server to query to do the
	// resolving.
	//
	// Most schemes do not permit the Authority field to be specified.
	Authority string

	// Endpoint provides information about which name to resolve.
	//
	// This field is mandatory.
	Endpoint string

	// Query is a collection of key-value mappings.
	Query url.Values

	// ServerName is the suggested value for the
	// "crypto/tls".(*Config).ServerName field.
	//
	// It is filled in by Parse, FromGRPCTarget, or PostProcess.
	ServerName string

	// HasSlash is true if the string representation was originally of the
	// form "scheme://authority/endpoint?query", rather than
	// "scheme:endpoint?query".
	//
	// It is a Scheme-specific hint to PostProcess that a leading slash
	// before Endpoint can be inferred if necessary.
	HasSlash bool
}

// MarshalJSON fulfills json.Marshaler.
func (rt Target) MarshalJSON() ([]byte, error) {
	return json.Marshal(rt.String())
}

// AppendTo appends the string representation of this Target to the
// provided Builder.
func (rt Target) AppendTo(out *strings.Builder) {
	out.WriteString(rt.Scheme)
	out.WriteString("://")
	escapeAuthorityTo(out, rt.Authority)
	out.WriteString("/")
	escapeEndpointTo(out, rt.Endpoint)
	appendQueryStringTo(out, rt.Query)
}

// String returns the string representation of this Target.
func (rt Target) String() string {
	var buf strings.Builder
	buf.Grow(32)
	rt.AppendTo(&buf)
	return buf.String()
}

// AsGRPCTarget converts this Target to an equivalent resolver.Target.
func (rt Target) AsGRPCTarget() resolver.Target {
	var buf strings.Builder
	buf.Grow(len(rt.Endpoint))
	escapeEndpointTo(&buf, rt.Endpoint)
	appendQueryStringTo(&buf, rt.Query)
	return resolver.Target{
		Scheme:    rt.Scheme,
		Authority: escapeAuthority(rt.Authority),
		Endpoint:  buf.String(),
	}
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (rt *Target) UnmarshalJSON(raw []byte) error {
	var str string
	err := json.Unmarshal(raw, &str)
	if err != nil {
		*rt = Target{}
		return err
	}

	err = rt.Parse(str)
	if err != nil {
		return err
	}

	return nil
}

// Parse parses the given string representation.
func (rt *Target) Parse(str string) error {
	if rt == nil {
		panic(errors.New("*Target is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*rt = Target{}
		}
	}()

	var err error

	if str == "" {
		return roxyutil.EndpointError{Endpoint: str, Err: roxyutil.ErrExpectNonEmpty}
	}

	if str == constants.NullString {
		return roxyutil.EndpointError{Endpoint: str, Err: roxyutil.ErrFailedToMatch}
	}

	match := reTargetScheme.FindStringSubmatch(str)
	if match != nil {
		rt.Scheme = match[1]
		n := len(match[0])
		str = str[n:]
	}

	rt.HasSlash = false
	if strings.HasPrefix(str, "//") {
		str = str[2:]
		i := strings.IndexByte(str, '/')
		var authority string
		if i >= 0 {
			authority, str = str[:i], str[i+1:]
			rt.HasSlash = true
		} else {
			authority, str = str, ""
		}
		rt.Authority = unescapeAuthorityOrEndpoint(authority)
	}

	ep, qs, hasQS := splitPathAndQueryString(str)
	rt.Endpoint = unescapeAuthorityOrEndpoint(ep)
	if hasQS {
		rt.Query, err = url.ParseQuery(qs)
		if err != nil {
			return roxyutil.QueryStringError{QueryString: qs, Err: err}
		}
	}

	err = rt.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// FromGRPCTarget tries to make this Target identical to the given resolver.Target.
func (rt *Target) FromGRPCTarget(target resolver.Target) error {
	if rt == nil {
		panic(errors.New("*Target is nil"))
	}

	wantZero := true
	defer func() {
		if wantZero {
			*rt = Target{}
		}
	}()

	var err error

	rt.Scheme = target.URL.Scheme

	rt.Authority = target.URL.Host

	if strings.HasPrefix(target.URL.Path, "/") {
		rt.HasSlash = true
		rt.Endpoint = strings.TrimLeft(target.URL.Path, "/")
	} else if target.URL.Path != "" {
		rt.HasSlash = false
		rt.Endpoint = target.URL.Path
	} else {
		rt.HasSlash = false
		rt.Endpoint = target.URL.Opaque
	}

	if target.URL.RawQuery != "" {
		qs := target.URL.RawQuery
		rt.Query, err = url.ParseQuery(qs)
		if err != nil {
			return roxyutil.QueryStringError{QueryString: qs, Err: err}
		}
	} else if target.URL.ForceQuery {
		rt.Query = make(url.Values, 0)
	}

	err = rt.PostProcess()
	if err != nil {
		return err
	}

	wantZero = false
	return nil
}

// PostProcess performs data integrity checks and input post-processing.
func (rt *Target) PostProcess() error {
	if rt == nil {
		panic(errors.New("*Target is nil"))
	}

	if rt.Scheme == constants.SchemeEmpty {
		rt.Scheme = constants.SchemeDNS
	}

	switch rt.Scheme {
	case constants.SchemePassthrough:
		return rt.postProcessPassthrough()
	case constants.SchemeUnix:
		return rt.postProcessUnix()
	case constants.SchemeUnixAbstract:
		return rt.postProcessUnixAbstract()
	case constants.SchemeIP:
		return rt.postProcessIP()
	case constants.SchemeDNS:
		return rt.postProcessDNS()
	case constants.SchemeSRV:
		return rt.postProcessSRV()
	case constants.SchemeZK:
		return rt.postProcessZK()
	case constants.SchemeEtcd:
		return rt.postProcessEtcd()
	case constants.SchemeATC:
		return rt.postProcessATC()

	default:
		return roxyutil.SchemeError{
			Scheme: rt.Scheme,
			Err:    roxyutil.ErrNotExist,
		}
	}
}

func (rt *Target) postProcessPassthrough() error {
	host, port, err := misc.SplitHostPort(rt.Endpoint, constants.PortHTTPS)
	if err != nil {
		return err
	}
	rt.Endpoint = net.JoinHostPort(host, port)
	rt.ServerName = host
	return nil
}

func (rt *Target) postProcessUnix() error {
	if rt.HasSlash && (rt.Endpoint == "" || (rt.Endpoint[0] != '/' && rt.Endpoint[0] != '@' && rt.Endpoint[0] != '\x00')) {
		rt.Endpoint = "/" + rt.Endpoint
	}
	var t UnixTarget
	err := t.FromTarget(*rt)
	if err != nil {
		return err
	}
	if t.IsAbstract {
		rt.Scheme = constants.SchemeUnixAbstract
		rt.Endpoint = t.Addr.Name[1:]
	}
	rt.Authority = ""
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessUnixAbstract() error {
	var t UnixTarget
	err := t.FromTarget(*rt)
	if err != nil {
		return err
	}
	rt.Authority = ""
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessIP() error {
	var t IPTarget
	err := t.FromTarget(*rt, constants.PortHTTPS)
	if err != nil {
		return err
	}
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessDNS() error {
	var t DNSTarget
	err := t.FromTarget(*rt, constants.PortHTTPS)
	if err != nil {
		return err
	}
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessSRV() error {
	var t SRVTarget
	err := t.FromTarget(*rt)
	if err != nil {
		return err
	}
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessZK() error {
	if rt.HasSlash && (rt.Endpoint == "" || rt.Endpoint[0] != '/') {
		rt.Endpoint = "/" + rt.Endpoint
	}
	var t ZKTarget
	err := t.FromTarget(*rt)
	if err != nil {
		return err
	}
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessEtcd() error {
	var t EtcdTarget
	err := t.FromTarget(*rt)
	if err != nil {
		return err
	}
	rt.ServerName = t.ServerName
	return nil
}

func (rt *Target) postProcessATC() error {
	var t ATCTarget
	err := t.FromTarget(*rt)
	if err != nil {
		return err
	}
	rt.ServerName = t.ServerName
	return nil
}

func splitPathAndQueryString(str string) (path string, qs string, hasQS bool) {
	i := strings.IndexByte(str, '?')
	if i >= 0 {
		return str[:i], str[i+1:], true
	}
	return str, "", false
}

func escapeAuthority(str string) string {
	var buf strings.Builder
	buf.Grow(len(str))
	escapeAuthorityTo(&buf, str)
	return buf.String()
}

func escapeAuthorityTo(out *strings.Builder, str string) {
	for _, ch := range str {
		switch ch {
		case '%':
			out.WriteString("%25")
		case '/':
			out.WriteString("%2f")
		case ';':
			out.WriteString("%3b")
		case '?':
			out.WriteString("%3f")
		default:
			out.WriteRune(ch)
		}
	}
}

func escapeEndpointTo(out *strings.Builder, str string) {
	for _, ch := range str {
		switch ch {
		case '%':
			out.WriteString("%25")
		case ';':
			out.WriteString("%3b")
		case '?':
			out.WriteString("%3f")
		default:
			out.WriteRune(ch)
		}
	}
}

func unescapeAuthorityOrEndpoint(str string) string {
	var buf strings.Builder
	buf.Grow(len(str))
	runes := []rune(str)
	i, length := 0, len(runes)
	for i < length {
		ch := runes[i]
		if ch == '%' && (i+2) < length {
			hi, hiOK := decodeHexDigit(runes[i+1])
			lo, loOK := decodeHexDigit(runes[i+2])
			if hiOK && loOK {
				buf.WriteByte((hi << 4) | lo)
				i += 3
				continue
			}
		}
		buf.WriteRune(ch)
		i++
	}
	return buf.String()
}

func decodeHexDigit(ch rune) (byte, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return byte(ch - '0'), true
	case ch >= 'A' && ch <= 'F':
		return 0xa + byte(ch-'A'), true
	case ch >= 'a' && ch <= 'f':
		return 0xa + byte(ch-'a'), true
	default:
		return 0, false
	}
}

func appendQueryStringTo(out *strings.Builder, query url.Values) {
	if len(query) != 0 {
		out.WriteString("?")
		keys := make([]string, 0, len(query))
		for key := range query {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		needAmp := false
		for _, key := range keys {
			values := query[key]
			for _, value := range values {
				if needAmp {
					out.WriteString("&")
				}
				out.WriteString(url.QueryEscape(key))
				out.WriteString("=")
				out.WriteString(url.QueryEscape(value))
				needAmp = true
			}
		}
	}
}
