package types

import "encoding/json"

type omit *struct{}

type WireEvent struct {
	*Event
	Name         omit `json:"name,omitempty"` // not required, wired as the "event"
	TargetFilter omit `json:"target_filter,omitempty"`
}

func (e WireEvent) MarshalJSON() ([]byte, error) {
	type localWireEvent WireEvent
	data, err := json.Marshal(localWireEvent(e))
	if err != nil {
		return nil, err
	}
	m := WebsocketMessage{
		Event: e.Event.Name,
		Data:  data,
	}
	return json.Marshal(m)
}