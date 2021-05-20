package roxyutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/chronos-tachyon/roxy/internal/constants"
)

// ErrIsNull indicates that a JSON null value was parsed.
var ErrIsNull = isNullError(0)

// EnumData holds data about one particular enum value.
type EnumData struct {
	// GoName is the Go constant name for this enum value.
	GoName string

	// Name is the string representation of this enum value.
	Name string

	// JSON is the JSON representation of this enum value.
	//
	// Optional; it is inferred from Name if not set.
	JSON []byte

	// Aliases is a list of zero or more aliases for this enum value.
	//
	// Optional.
	Aliases []string
}

// MakeAllowedEnumNames returns the list of canonical string representations
// for this enum.
func MakeAllowedEnumNames(enumData []EnumData) []string {
	out := make([]string, len(enumData))
	for i, row := range enumData {
		out[i] = row.Name
	}
	return out
}

// DereferenceEnumData returns enumData[value] or panics with InvalidEnumValueError.
func DereferenceEnumData(enumName string, enumData []EnumData, value uint) EnumData {
	if limit := uint(len(enumData)); value >= limit {
		panic(InvalidEnumValueError{
			Type:  enumName,
			Value: value,
			Limit: limit,
		})
	}
	return enumData[value]
}

// MarshalEnumToJSON marshals this enum value to JSON.  It may panic with
// InvalidEnumValueError if the enum value is out of range.
func MarshalEnumToJSON(enumName string, enumData []EnumData, value uint) ([]byte, error) {
	row := DereferenceEnumData(enumName, enumData, value)
	if row.JSON == nil {
		return json.Marshal(row.Name)
	}
	return row.JSON, nil
}

// ParseEnum parses an enum value.  Returns InvalidEnumNameError if the string
// cannot be parsed.
func ParseEnum(enumName string, enumData []EnumData, str string) (uint, error) {
	for index, row := range enumData {
		if strings.EqualFold(str, row.Name) || strings.EqualFold(str, row.GoName) {
			return uint(index), nil
		}
		for _, alias := range row.Aliases {
			if strings.EqualFold(str, alias) {
				return uint(index), nil
			}
		}
	}

	return 0, InvalidEnumNameError{
		Type:    enumName,
		Name:    str,
		Allowed: MakeAllowedEnumNames(enumData),
	}
}

// UnmarshalEnumFromJSON unmarshals an enum value from JSON.  Returns
// ErrIsNull, InvalidEnumNameError, or InvalidEnumValueError if a JSON value
// was parsed but could not be unmarshaled as an enum value.
func UnmarshalEnumFromJSON(enumName string, enumData []EnumData, raw []byte) (uint, error) {
	if raw == nil {
		panic(errors.New("[]byte is nil"))
	}

	if bytes.Equal(raw, constants.NullBytes) {
		return 0, ErrIsNull
	}

	for index, row := range enumData {
		if row.JSON != nil && bytes.Equal(raw, row.JSON) {
			return uint(index), nil
		}
	}

	var str string
	err0 := json.Unmarshal(raw, &str)
	if err0 == nil {
		return ParseEnum(enumName, enumData, str)
	}

	var num uint
	err1 := json.Unmarshal(raw, &num)
	limit := uint(len(enumData))
	if err1 == nil && num >= limit {
		return 0, InvalidEnumValueError{
			Type:  enumName,
			Value: num,
			Limit: limit,
		}
	}
	if err1 == nil {
		return num, nil
	}

	return 0, err0

}

type isNullError int

func (isNullError) Error() string {
	return "JSON value is null"
}

var _ error = isNullError(0)
