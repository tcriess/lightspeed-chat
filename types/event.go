package types

import (
	"fmt"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/tcriess/lightspeed-chat/globals"
)

const (
	EventTypeInfo         = "info"
	EventTypeChat         = "chat"
	EventTypeCommand      = "command"
	EventTypeUser         = "user"
	EventTypeTranslation  = "translation"
	EventTypeChats        = "chats"
	EventTypeCommands     = "commands"
	EventTypeUsers        = "users"
	EventTypeTranslations = "translations"
)

type Source struct {
	*User      `json:"user"`
	PluginName string `json:"plugin_name"`
}

type Event struct {
	Id       string `json:"id" hash:"ignore"`
	*Room    `json:"room"`
	*Source  `json:"source"`
	Created  time.Time         `json:"created"`
	Language string            `json:"language"`
	Name     string            `json:"name"`
	Tags     map[string]string `json:"tags"`
	IntTags  map[string]int64  `json:"int_tags"`
	History  bool              `json:"history"` // set to true is this event is sent from history

	// the following fields are not part of the filter.Env!
	Sent         time.Time `json:"sent" hash:"ignore"`
	TargetFilter string    `json:"target_filter"`
}

// NewEvent creates a new event with the given parameters.
//
// The resulting *Event has no `nil` values, the Created timestamp is set to now.
func NewEvent(room *Room, source *Source, targetFilter string, language string, name string, tags map[string]string, intTags map[string]int64) *Event {
	if room == nil {
		room = &Room{}
	}
	if room.Owner == nil {
		room.Owner = &User{}
	}
	if room.Owner.Tags == nil {
		room.Owner.Tags = make(map[string]string)
	}
	if room.Owner.IntTags == nil {
		room.Owner.IntTags = make(map[string]int64)
	}
	if source == nil {
		source = &Source{}
	}
	if source.User == nil {
		source.User = &User{}
	}
	if source.User.Tags == nil {
		source.User.Tags = make(map[string]string)
	}
	if source.User.IntTags == nil {
		source.User.IntTags = make(map[string]int64)
	}
	if tags == nil {
		tags = make(map[string]string)
	}
	if intTags == nil {
		intTags = make(map[string]int64)
	}
	room.Owner.LastOnline = room.Owner.LastOnline.In(time.UTC)
	source.User.LastOnline = source.User.LastOnline.In(time.UTC)
	evt := &Event{
		Room:         room,
		Source:       source,
		Created:      time.Now().In(time.UTC),
		Language:     language,
		Name:         name,
		Tags:         tags,
		IntTags:      intTags,
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
