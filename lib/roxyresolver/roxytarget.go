package roxyresolver

import (
	"encoding/json"
	"fmt"
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

	// ServerName is the value to use for the
	// "crypto/tls".(*Config).ServerName field.
	ServerName string

	// Query is a collection of key-value mappings.
	Query url.Values
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
	wantZero := true
	defer func() {
		if wantZero {
			*rt = Target{}
		}
	}()

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

	hasSlash := false
	if strings.HasPrefix(str, "//") {
		str = str[2:]
		i := strings.IndexByte(str, '/')
		var authority string
		if i >= 0 {
			authority, str = str[:i], str[i+1:]
			hasSlash = true
		} else {
			authority, str = str, ""
		}
		rt.Authority = unescapeAuthorityOrEndpoint(authority)
	}

	ep, qs, hasQS := splitPathAndQueryString(str)
	rt.Endpoint = unescapeAuthorityOrEndpoint(ep)
	if hasQS {
		var err error
		rt.Query, err = url.ParseQuery(qs)
		if err != nil {
			return roxyutil.QueryStringError{QueryString: qs, Err: err}
		}
	}

	tmp, err := rt.postprocess(hasSlash)
	if err != nil {
		return err
	}

	*rt = tmp
	wantZero = false
	return nil
}

// FromGRPCTarget tries to make this Target identical to the given resolver.Target.
func (rt *Target) FromGRPCTarget(target resolver.Target) error {
	wantZero := true
	defer func() {
		if wantZero {
			*rt = Target{}
		}
	}()

	rt.Scheme = target.Scheme
	rt.Authority = unescapeAuthorityOrEndpoint(target.Authority)
	ep, qs, hasQS := splitPathAndQueryString(target.Endpoint)
	rt.Endpoint = unescapeAuthorityOrEndpoint(ep)

	if hasQS {
		var err error
		rt.Query, err = url.ParseQuery(qs)
		if err != nil {
			return roxyutil.QueryStringError{QueryString: qs, Err: err}
		}
	}

	tmp, err := rt.postprocess(false)
	if err != nil {
		return err
	}

	*rt = tmp
	wantZero = false
	return nil
}

func (rt Target) postprocess(hasSlash bool) (Target, error) {
	var zero Target

	if rt.Scheme == constants.SchemeEmpty {
		rt.Scheme = constants.SchemeDNS
	}

	switch rt.Scheme {
	case constants.SchemePassthrough:
		host, _, err := misc.SplitHostPort(rt.Endpoint, constants.PortHTTPS)
		if err != nil {
			return zero, err
		}
		rt.ServerName = host

	case constants.SchemeUnix:
		if hasSlash && (rt.Endpoint == "" || (rt.Endpoint[0] != '/' && rt.Endpoint[0] != '@' && rt.Endpoint[0] != '\x00')) {
			rt.Endpoint = "/" + rt.Endpoint
		}
		unixAddr, _, serverName, err := ParseUnixTarget(rt)
		if err != nil {
			return zero, err
		}
		if unixAddr.Name != "" && unixAddr.Name[0] == '\x00' {
			rt.Scheme = constants.SchemeUnixAbstract
			rt.Endpoint = unixAddr.Name[1:]
		}
		rt.Authority = ""
		rt.ServerName = serverName

	case constants.SchemeUnixAbstract:
		_, _, serverName, err := ParseUnixTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.Authority = ""
		rt.ServerName = serverName

	case constants.SchemeIP:
		_, _, serverName, err := ParseIPTarget(rt, constants.PortHTTPS)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case constants.SchemeDNS:
		_, _, _, _, _, _, serverName, err := ParseDNSTarget(rt, constants.PortHTTPS)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case constants.SchemeSRV:
		_, _, _, _, _, _, serverName, err := ParseSRVTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case constants.SchemeZK:
		if rt.Endpoint == "" || rt.Endpoint[0] != '/' {
			rt.Endpoint = "/" + rt.Endpoint
		}
		_, _, _, serverName, err := ParseZKTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case constants.SchemeEtcd:
		_, _, _, serverName, err := ParseEtcdTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case constants.SchemeATC:
		_, _, _, _, _, serverName, err := ParseATCTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	default:
		return zero, fmt.Errorf("scheme %q is not supported", rt.Scheme)
	}

	return rt, nil
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
