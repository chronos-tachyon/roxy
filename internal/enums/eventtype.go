package enums

import (
	"encoding/json"
	"fmt"
	"strings"
)

type EventType uint8

const (
	ErrorEvent EventType = iota
	UpdateEvent
	DeleteEvent
	CorruptEvent
	StatusChangeEvent
)

var eventTypeData = []enumData{
	{"ErrorEvent", "error"},
	{"UpdateEvent", "update"},
	{"DeleteEvent", "delete"},
	{"CorruptEvent", "corruptionDetected"},
	{"StatusChangeEvent", "statusChange"},
}

var eventTypeMap = map[string]EventType{
	"":                   ErrorEvent,
	"error":              ErrorEvent,
	"update":             UpdateEvent,
	"delete":             DeleteEvent,
	"corruptionDetected": CorruptEvent,
	"statusChange":       StatusChangeEvent,
}

func (t EventType) String() string {
	if uint(t) >= uint(len(eventTypeData)) {
		return fmt.Sprintf("#%d", uint(t))
	}
	return eventTypeData[t].Name
}

func (t EventType) GoString() string {
	if uint(t) >= uint(len(eventTypeData)) {
		return fmt.Sprintf("EventType(%d)", uint(t))
	}
	return eventTypeData[t].GoName
}

func (t EventType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (ptr *EventType) UnmarshalJSON(raw []byte) error {
	*ptr = 0

	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}

	if num, ok := eventTypeMap[strings.ToLower(str)]; ok {
		*ptr = num
		return nil
	}

	return fmt.Errorf("illegal event type %q; expected one of %q", str, makeAllowedNames(eventTypeData))
}

var _ fmt.Stringer = EventType(0)
var _ fmt.GoStringer = EventType(0)
var _ json.Marshaler = EventType(0)
var _ json.Unmarshaler = (*EventType)(nil)
