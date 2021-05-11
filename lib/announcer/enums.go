package announcer

import (
	"bytes"
	"fmt"
)

var nullBytes = []byte("null")

type Format uint8

const (
	RoxyFormat Format = iota
	FinagleFormat
	GRPCFormat
)

var announceFormatNames = []string{
	"roxy",
	"finagle",
	"grpc",
}

var announceFormatJSON = [][]byte{
	[]byte(`"roxy"`),
	[]byte(`"finagle"`),
	[]byte(`"grpc"`),
}

func (f Format) String() string {
	if uint(f) >= uint(len(announceFormatNames)) {
		panic(fmt.Errorf("bad value Format(%d)", uint(f)))
	}
	return announceFormatNames[f]
}

func (f Format) MarshalJSON() ([]byte, error) {
	if uint(f) >= uint(len(announceFormatJSON)) {
		return nil, fmt.Errorf("bad value Format(%d)", uint(f))
	}
	return announceFormatJSON[f], nil
}

func (f *Format) Parse(str string) error {
	if str == "" {
		return nil
	}
	for index, value := range announceFormatNames {
		if str == value {
			*f = Format(index)
			return nil
		}
	}
	return fmt.Errorf("expected \"roxy\", \"finagle\", or \"grpc\"; got %q", str)
}

func (f *Format) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(raw, nullBytes) {
		return nil
	}
	for index, value := range announceFormatJSON {
		if bytes.Equal(raw, value) {
			*f = Format(index)
			return nil
		}
	}
	return fmt.Errorf("expected \"roxy\", \"finagle\", or \"grpc\"; got %s", string(raw))
}
