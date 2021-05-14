package roxyresolver

import (
	"errors"
	"fmt"
)

type Event struct {
	Type              EventType
	Err               error
	Key               string
	Data              Resolved
	ServiceConfigJSON string
}

//nolint:gocyclo
func (event *Event) Check() {
	if checkDisabled {
		return
	}
	if event == nil {
		panic(errors.New("*Event is nil"))
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
