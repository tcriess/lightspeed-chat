package types

import (
	"fmt"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/tcriess/lightspeed-chat/globals"
)

const (
	EventTypeInfo        = "info"
	EventTypeChat        = "chat"
	EventTypeCommand     = "command"
	EventTypeUser        = "user"
	EventTypeTranslation = "translation"
	EventTypeInternal    = "_internal"
)

type Source struct {
	*User      `json:"user"`
	PluginName string `json:"plugin_name"`
}

type Event struct {
	Id       string `json:"id" hash:"ignore"`
	*Room    `json:"room"`
	*Source  `json:"source"`
	Created  time.Time     `json:"created" gorm:"autoCreateTime"`
	Language string        `json:"language"`
	Name     string        `json:"name"`
	Tags     JSONStringMap `json:"tags"`
	History  bool          `json:"history" gorm:"-"` // set to true is this event is sent from history

	// the following fields are not part of the filter.Env!
	Sent         time.Time `json:"sent" hash:"ignore"`
	TargetFilter string    `json:"target_filter"`
}

// NewEvent creates a new event with the given parameters.
//
// The resulting *Event has no `nil` values, the Created timestamp is set to now.
func NewEvent(room *Room, source *Source, targetFilter string, language string, name string, tags map[string]string) *Event {
	if source == nil {
		source = &Source{}
	}
	if source.User == nil {
		source.User = &User{}
	}
	source.User.LastOnline = source.User.LastOnline.In(time.UTC)
	if source.User.Tags == nil {
		source.User.Tags = make(map[string]string)
	}
	if room == nil {
		room = &Room{}
	}
	if room.Owner == nil {
		room.Owner = &User{}
	}
	if room.Owner.Tags == nil {
		room.Owner.Tags = make(map[string]string)
	}
	if room.Tags == nil {
		room.Tags = make(map[string]string)
	}
	if tags == nil {
		tags = make(map[string]string)
	}
	evt := &Event{
		Room:         room,
		Source:       source,
		Created:      time.Now().In(time.UTC),
		Language:     language,
		Name:         name,
		Tags:         tags,
		TargetFilter: targetFilter,
	}
	hash, err := hashstructure.Hash(evt, hashstructure.FormatV2, nil)
	if err != nil {
		globals.AppLogger.Error("could not hash event", "error", err)
	} else {
		evt.Id = fmt.Sprintf("%016X", hash)
	}
	return evt
}
