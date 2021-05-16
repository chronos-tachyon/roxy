// Package certnames contains helpers for validating X.509 client certificates.
package certnames

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

var oidEmail = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}

// ANY is the string representation of a CertNames that permits all certificates.
const ANY = "ANY"

// type CertNames {{{

// CertNames is a set of permitted Subject Distinguished Name and Subject
// Alternative Name components that a client certificate can match.
type CertNames struct {
	permitAll           bool
	organizations       map[string]struct{}
	organizationalUnits map[string]struct{}
	commonNames         map[string]struct{}
	emailAddresses      map[string]struct{}
}

// IsPermitAll returns true if all certificates are permitted.
func (cns CertNames) IsPermitAll() bool {
	return cns.permitAll
}

// Check returns true if the given cert matches at least one permitted name.
func (cns CertNames) Check(cert *x509.Certificate) bool {
	if cns.permitAll {
		return true
	}
	if cns.organizations != nil {
		for _, org := range cert.Subject.Organization {
			if _, found := cns.organizations[org]; found {
				return true
			}
		}
	}
	if cns.organizationalUnits != nil {
		for _, orgUnit := range cert.Subject.OrganizationalUnit {
			if _, found := cns.organizationalUnits[orgUnit]; found {
				return true
			}
		}
	}
	if cns.commonNames != nil {
		if commonName := cert.Subject.CommonName; commonName != "" {
			if _, found := cns.commonNames[commonName]; found {
				return true
			}
		}
	}
	if cns.emailAddresses != nil {
		for _, name := range cert.Subject.Names {
			if name.Type.Equal(oidEmail) {
				if value, ok := name.Value.(string); ok {
					emailAddress := canonicalEmailAddress(value)
					if _, found := cns.emailAddresses[emailAddress]; found {
						return true
					}
				}
			}
		}
		for _, item := range cert.EmailAddresses {
			emailAddress := canonicalEmailAddress(item)
			if _, found := cns.emailAddresses[emailAddress]; found {
				return true
			}
		}
	}
	return false
}

// AppendTo efficiently appends the string representation to the given Builder.
// See FromList for details about the format.
func (cns CertNames) AppendTo(out *strings.Builder) {
	if cns.permitAll {
		out.WriteString(ANY)
		return
	}

	first := true
	for org := range cns.organizations {
		if !first {
			out.WriteByte(':')
		}
		out.WriteString("O=")
		out.WriteString(org)
		first = false
	}
	for orgUnit := range cns.organizationalUnits {
		if !first {
			out.WriteByte(':')
		}
		out.WriteString("OU=")
		out.WriteString(orgUnit)
		first = false
	}
	for commonName := range cns.commonNames {
		if !first {
			out.WriteByte(':')
		}
		out.WriteString("CN=")
		out.WriteString(commonName)
		first = false
	}
	for emailAddress := range cns.emailAddresses {
		if !first {
			out.WriteByte(':')
		}
		out.WriteString("E=")
		out.WriteString(emailAddress)
		first = false
	}
	if first {
		out.WriteByte(':')
	}
}

// String returns a colon-delimited list of permitted names, or "ANY" if all
// certificates are permitted.  See FromList for details about the format.
func (cns CertNames) String() string {
	if cns.permitAll {
		return ANY
	}

	var length uint
	for org := range cns.organizations {
		length += 3 + uint(len(org))
	}
	for orgUnit := range cns.organizationalUnits {
		length += 4 + uint(len(orgUnit))
	}
	for commonName := range cns.commonNames {
		length += 4 + uint(len(commonName))
	}
	for emailAddress := range cns.emailAddresses {
		length += 3 + uint(len(emailAddress))
	}
	if length == 0 {
		return ":"
	}
	length--

	var buf strings.Builder
	buf.Grow(int(length))
	cns.AppendTo(&buf)
	return buf.String()
}

// List returns the list of permitted names, or ["ANY"] if all certificates are
// permitted.  See FromList for details about the format.
func (cns CertNames) List() []string {
	if cns.permitAll {
		return []string{ANY}
	}

	a := uint(len(cns.organizations))
	b := uint(len(cns.organizationalUnits))
	c := uint(len(cns.commonNames))
	d := uint(len(cns.emailAddresses))
	list := make([]string, 0, a+b+c+d)
	for _, org := range sortSet(cns.organizations) {
		str := "O=" + org
		list = append(list, str)
	}
	for _, orgUnit := range sortSet(cns.organizationalUnits) {
		str := "OU=" + orgUnit
		list = append(list, str)
	}
	for _, commonName := range sortSet(cns.commonNames) {
		str := "CN=" + commonName
		list = append(list, str)
	}
	for _, emailAddress := range sortSet(cns.emailAddresses) {
		str := "E=" + emailAddress
		list = append(list, str)
	}
	return list
}

// Parse parses a colon-delimited list of names.  See FromList for details
// about the format.
func (cns *CertNames) Parse(str string) error {
	if str == ANY {
		*cns = CertNames{permitAll: true}
		return nil
	}
	if str == "" || str == ":" {
		*cns = CertNames{}
		return nil
	}

	list := strings.Split(str, ":")
	return cns.FromList(list)
}

// FromList parses a list of names.
//
//	- "ANY" permits all certificates
//
//	- "O=<org>" permits certs with a Subject.Organization of <org>
//
//	- "OU=<unit>" permits certs with a Subject.OrganizationalUnit of <unit>
//
//	- "CN=<name>" permits certs with a Subject.CommonName of <name>
//
//	- "E=<email>" permits certs with a Subject.Name of Type
//	  OID(1.2.840.113549.1.9.1) (the obsolete DN field "emailAddress") and
//	  value <email>, or with an EmailAddress SAN of <email>
//
//	- If the list item isn't "KEY=VALUE"-shaped, then the parser will
//	  make an educated guess as to whether the list item is meant to be
//	  a commonName or an emailAddress.
//
func (cns *CertNames) FromList(list []string) error {
	*cns = CertNames{}
	if len(list) == 0 {
		return nil
	}

	cns.organizations = make(map[string]struct{}, len(list))
	cns.organizationalUnits = make(map[string]struct{}, len(list))
	cns.commonNames = make(map[string]struct{}, len(list))
	cns.emailAddresses = make(map[string]struct{}, len(list))

	for _, item := range list {
		if item == "" {
			continue
		}

		if item == ANY {
			cns.permitAll = true
			continue
		}

		var key, value string
		i := strings.IndexByte(item, '=')
		if i >= 0 {
			key = strings.ToUpper(item[:i])
			value = item[i+1:]
		} else {
			key = "CN"
			value = item
			if strings.Contains(value, "@") {
				key = "E"
			}
		}

		switch key {
		case "O":
			org := value
			cns.organizations[org] = struct{}{}
		case "OU":
			orgUnit := value
			cns.organizationalUnits[orgUnit] = struct{}{}
		case "CN":
			commonName := value
			cns.commonNames[commonName] = struct{}{}
		case "E":
			emailAddress := canonicalEmailAddress(value)
			cns.emailAddresses[emailAddress] = struct{}{}
		default:
			return ParseError{key, value}
		}
	}

	if len(cns.organizations) == 0 {
		cns.organizations = nil
	}
	if len(cns.organizationalUnits) == 0 {
		cns.organizationalUnits = nil
	}
	if len(cns.commonNames) == 0 {
		cns.commonNames = nil
	}
	if len(cns.emailAddresses) == 0 {
		cns.emailAddresses = nil
	}

	return nil
}

// MarshalJSON fulfills json.Marshaler.
func (cns CertNames) MarshalJSON() ([]byte, error) {
	return json.Marshal(cns.String())
}

// UnmarshalJSON fulfills json.Unmarshaler.
func (cns *CertNames) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, constants.NullBytes) {
		return nil
	}

	var str string
	err0 := json.Unmarshal(raw, &str)
	if err0 == nil {
		return cns.Parse(str)
	}

	var list []string
	err1 := json.Unmarshal(raw, &list)
	if err1 == nil {
		return cns.FromList(list)
	}

	return err0
}

// }}}

// type ParseError {{{

// ParseError represents a parsing error in CertNames.FromList.
type ParseError struct {
	Key   string
	Value string
}

// Error fulfills the error interface.
func (err ParseError) Error() string {
	return fmt.Sprintf("failed to parse %q: unknown key %q", err.Key+"="+err.Value, err.Key)
}

var _ error = ParseError{}

// }}}

func sortSet(set map[string]struct{}) []string {
	if set == nil {
		return nil
	}
	list := make([]string, 0, len(set))
	for item := range set {
		list = append(list, item)
	}
	sort.Strings(list)
	return list
}

func canonicalEmailAddress(in string) string {
	var user, domain string
	i := strings.IndexByte(in, '@')
	if i >= 0 {
		user, domain = in[:i], in[i+1:]
		domain = strings.ToLower(domain)
	} else {
		user = in
		domain = "localhost"
	}
	return user + "@" + domain
}
