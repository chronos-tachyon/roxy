package enums

import (
	"fmt"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// type DigestType {{{

// DigestType represents an algorithm for the HTTP "Digest" header.
type DigestType uint8

const (
	// DigestMD5 represents the MD5 algorithm.
	// It is an obsolete/broken cryptographic hash.
	DigestMD5 DigestType = iota

	// DigestSHA1 represents the SHA-1 algorithm.
	// It is an obsolete/broken cryptographic hash.
	DigestSHA1

	// DigestSHA256 represents the SHA-2 algorithm in 256-bit mode.
	// It is a cryptographic hash.
	DigestSHA256
)

var digestTypeData = []roxyutil.EnumData{
	{
		GoName: "DigestMD5",
		Name:   "md5",
	},
	{
		GoName: "DigestSHA1",
		Name:   "sha1",
	},
	{
		GoName: "DigestSHA256",
		Name:   "sha256",
	},
}

// GoString returns the Go constant name.
func (t DigestType) GoString() string {
	return roxyutil.DereferenceEnumData("DigestType", digestTypeData, uint(t)).GoName
}

// String returns the HTTP "Digest" algorithm name.
func (t DigestType) String() string {
	return roxyutil.DereferenceEnumData("DigestType", digestTypeData, uint(t)).Name
}

var _ fmt.GoStringer = DigestType(0)
var _ fmt.Stringer = DigestType(0)

// }}}
