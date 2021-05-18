package enums

import (
	"fmt"
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

var digestTypeData = []enumData{
	{"DigestMD5", "md5"},
	{"DigestSHA1", "sha1"},
	{"DigestSHA256", "sha256"},
}

// GoString returns the Go constant name.
func (t DigestType) GoString() string {
	if uint(t) >= uint(len(digestTypeData)) {
		return fmt.Sprintf("DigestType(%d)", uint(t))
	}
	return digestTypeData[t].GoName
}

// String returns the HTTP "Digest" algorithm name.
func (t DigestType) String() string {
	// out of range => intentional panic
	return digestTypeData[t].Name
}

var _ fmt.GoStringer = DigestType(0)
var _ fmt.Stringer = DigestType(0)

// }}}
