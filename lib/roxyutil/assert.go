package roxyutil

import (
	"fmt"
	"reflect"
)

// Assert panics with CheckError if cond is false.
func Assert(cond bool, message string) {
	if cond {
		return
	}
	panic(CheckError{Message: message})
}

// Assertf panics with CheckError if cond is false.
func Assertf(cond bool, format string, v ...interface{}) {
	if cond {
		return
	}
	message := fmt.Sprintf(format, v...)
	panic(CheckError{Message: message})
}

// AssertNotNil takes a pointer to a nil-able type (pointer, interface, etc)
// and panics with CheckError if the pointed-to value is nil.
func AssertNotNil(v interface{}) {
	r0 := reflect.ValueOf(v)
	r1 := r0.Elem()
	if !r1.IsNil() {
		return
	}
	message := fmt.Sprintf("%s is nil", r1.Type().String())
	panic(CheckError{Message: message})
}
