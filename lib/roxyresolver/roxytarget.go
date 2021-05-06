package roxyresolver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type RoxyTarget struct {
	Scheme     string
	Authority  string
	Endpoint   string
	ServerName string
	Query      url.Values
}

func (rt RoxyTarget) MarshalJSON() ([]byte, error) {
	return json.Marshal(rt.String())
}

func (rt RoxyTarget) AppendTo(out *strings.Builder) {
	out.WriteString(rt.Scheme)
	out.WriteString("://")
	escapeAuthorityTo(out, rt.Authority)
	out.WriteString("/")
	escapeEndpointTo(out, rt.Endpoint)
	appendQueryStringTo(out, rt.Query)
}

func (rt RoxyTarget) String() string {
	var buf strings.Builder
	buf.Grow(32)
	rt.AppendTo(&buf)
	return buf.String()
}

func (rt RoxyTarget) AsTarget() Target {
	var buf strings.Builder
	buf.Grow(len(rt.Endpoint))
	escapeEndpointTo(&buf, rt.Endpoint)
	appendQueryStringTo(&buf, rt.Query)
	return Target{
		Scheme:    rt.Scheme,
		Authority: escapeAuthority(rt.Authority),
		Endpoint:  buf.String(),
	}
}

func (rt *RoxyTarget) UnmarshalJSON(raw []byte) error {
	var str string
	err := json.Unmarshal(raw, &str)
	if err != nil {
		return err
	}
	return rt.Parse(str)
}

func (rt *RoxyTarget) Parse(str string) error {
	wantZero := true
	defer func() {
		if wantZero {
			*rt = RoxyTarget{}
		}
	}()

	match := regexp.MustCompile(`^([0-9A-Za-z+-]+):`).FindStringSubmatch(str)
	if match != nil {
		rt.Scheme = match[1]
		n := len(match[0])
		str = str[n:]
	}

	if strings.HasPrefix(str, "//") {
		str = str[2:]
		i := strings.IndexByte(str, '/')
		var authority string
		if i >= 0 {
			authority, str = str[:i], str[i+1:]
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
			return BadQueryStringError{QueryString: qs, Err: err}
		}
	}

	tmp, err := rt.postprocess()
	if err != nil {
		return err
	}

	*rt = tmp
	wantZero = false
	return nil
}

func RoxyTargetFromTarget(target Target) (RoxyTarget, error) {
	var zero RoxyTarget
	var rt RoxyTarget

	rt.Scheme = target.Scheme
	rt.Authority = unescapeAuthorityOrEndpoint(target.Authority)
	ep, qs, hasQS := splitPathAndQueryString(target.Endpoint)
	rt.Endpoint = unescapeAuthorityOrEndpoint(ep)

	if hasQS {
		var err error
		rt.Query, err = url.ParseQuery(qs)
		if err != nil {
			return zero, BadQueryStringError{QueryString: qs, Err: err}
		}
	}

	tmp, err := rt.postprocess()
	if err != nil {
		return zero, err
	}

	return tmp, nil
}

func (rt RoxyTarget) postprocess() (RoxyTarget, error) {
	var zero RoxyTarget

	if rt.Scheme == "" {
		rt.Scheme = passthroughScheme
	}

	switch rt.Scheme {
	case passthroughScheme:
		host, _, err := net.SplitHostPort(rt.Endpoint)
		if err != nil {
			h, _, err2 := net.SplitHostPort(rt.Endpoint + ":" + httpsPort)
			if err2 == nil {
				host, err = h, nil
			}
			if err != nil {
				return zero, err
			}
		}
		rt.ServerName = host

	case unixScheme:
		unixAddr, _, serverName, err := ParseUnixTarget(rt)
		if err != nil {
			return zero, err
		}
		if unixAddr.Name != "" && unixAddr.Name[0] == '\x00' {
			rt.Scheme = unixAbstractScheme
			rt.Endpoint = unixAddr.Name[1:]
		}
		rt.Authority = ""
		rt.ServerName = serverName

	case unixAbstractScheme:
		_, _, serverName, err := ParseUnixTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.Authority = ""
		rt.ServerName = serverName

	case ipScheme:
		_, _, serverName, err := ParseIPTarget(rt, httpsPort)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case dnsScheme:
		_, _, _, _, _, _, serverName, err := ParseDNSTarget(rt, httpsPort)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case srvScheme:
		_, _, _, _, _, _, serverName, err := ParseSRVTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case zkScheme:
		_, _, _, serverName, err := ParseZKTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case etcdScheme:
		_, _, _, serverName, err := ParseEtcdTarget(rt)
		if err != nil {
			return zero, err
		}
		rt.ServerName = serverName

	case atcScheme:
		panic(errors.New("not implemented"))

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
