package enums

import (
	"fmt"
)

// type DigestType {{{

type DigestType uint8

const (
	DigestMD5 = iota
	DigestSHA1
	DigestSHA256
)

var digestTypeData = []enumData{
	{"DigestMD5", "md5"},
	{"DigestSHA1", "sha1"},
	{"DigestSHA256", "sha256"},
}

func (t DigestType) String() string {
	// out of range => intentional panic
	return digestTypeData[t].Name
}

func (t DigestType) GoString() string {
	if uint(t) >= uint(len(digestTypeData)) {
		return fmt.Sprintf("DigestType(%d)", uint(t))
	}
	return digestTypeData[t].GoName
}

var _ fmt.Stringer = DigestType(0)
var _ fmt.GoStringer = DigestType(0)

// }}}
