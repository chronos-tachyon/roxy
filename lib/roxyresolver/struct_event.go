package roxyresolver

import (
	"fmt"
)

// Event represents the Resolver state changes triggered by some event.
type Event struct {
	// Type is the type of event, i.e. which data has changed.
	Type EventType

	// Err is the global error.
	//
	// Valid for: ErrorEvent.
	Err error

	// Key is the unique identifier for this address.
	//
	// Valid for: UpdateEvent, DeleteEvent, BadDataEvent, StatusChangeEvent.
	Key string

	// Data is the resolved address data.
	//
	// Valid for: UpdateEvent, StatusChangeEvent.
	Data Resolved

	// ServiceConfigJSON is the new gRPC service config.
	//
	// Valid for: NewServiceConfigEvent.
	ServiceConfigJSON string
}

// Check verifies the data integrity of all fields.
func (event Event) Check() {
	if checkDisabled {
		return
	}

	var (
		expectErr     bool
		expectKey     bool
		expectData    bool
		expectDataErr bool
		expectSC      bool
	)
	switch event.Type {
	case NoOpEvent:
		// pass
	case ErrorEvent:
		expectErr = true
	case UpdateEvent:
		expectKey = true
		expectData = true
	case DeleteEvent:
		expectKey = true
	case BadDataEvent:
		expectKey = true
		expectData = true
		expectDataErr = true
	case StatusChangeEvent:
		expectKey = true
		expectData = true
	case NewServiceConfigEvent:
		expectSC = true
	}

	if expectErr && event.Err == nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Err is nil", event.Type))
	}
	if !expectErr && event.Err != nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Err is non-nil", event.Type))
	}
	if expectKey && event.Key == "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.Key is empty", event.Type))
	}
	if !expectKey && event.Key != "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.Key is non-empty", event.Type))
	}
	if expectData && event.Data.Dynamic == nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Data is nil", event.Type))
	}
	if !expectData && event.Data.Dynamic != nil {
		panic(fmt.Errorf("Event.Type is %#v but Event.Data is non-nil", event.Type))
	}
	if expectSC && event.ServiceConfigJSON == "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.ServiceConfigJSON is empty", event.Type))
	}
	if !expectSC && event.ServiceConfigJSON != "" {
		panic(fmt.Errorf("Event.Type is %#v but Event.ServiceConfigJSON is non-empty", event.Type))
	}
	if expectData {
		event.Data.Check()
		if event.Key != event.Data.Unique {
			panic(fmt.Errorf("Event.Key is %q but Event.Data.Unique is %q", event.Key, event.Data.Unique))
		}
		if expectDataErr && event.Data.Err == nil {
			panic(fmt.Errorf("Event.Type is %#v but Event.Data.Err is nil", event.Type))
		}
		if !expectDataErr && event.Data.Err != nil {
			panic(fmt.Errorf("Event.Type is %#v but Event.Data.Err is non-nil", event.Type))
		}
	}
}
