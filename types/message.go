package types

import (
	"encoding/json"
	"time"
)

const (
	WireMessageTypeInfo         = "info"
	WireMessageTypeChat         = "chat"
	WireMessageTypeLogin        = "login"
	WireMessageTypeLogout       = "logout"
	WireMessageTypeUser         = "user"
	WireMessageTypeTranslation  = "translation"
	WireMessageTypeCommand      = "command"
	WireMessageTypeGeneric      = "generic"
	WireMessageTypeChats        = "chats"
	WireMessageTypeTranslations = "translations"
	WireMessageTypeUsers        = "users"
	WireMessageTypeCommands     = "commands"
	WireMessageTypeGenerics     = "generics"
)

// JSON-serialized WebsocketMessage is what is actually sent via the Websocket connection
type WebsocketMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// The different types of messages transferred from the client to here.

// ChatMessage is a basic chat message, contains the sender nick
type ChatMessage struct {
	Nick      string    `json:"nick" mapstructure:"-"`                   // sender nick, outgoing
	Timestamp time.Time `json:"timestamp" mapstructure:"-"`              // sent time, outgoing
	Message   string    `json:"message" mapstructure:"message"`          // actual message, incoming + outgoing
	Language  string    `json:"language" hash:"ignore" mapstructure:"-"` // language of the message (for future use), outgoing
	Filter    string    `json:"filter" mapstructure:"filter"`            // filter expression incoming
}

// LoginMessage is sent when a client logs in and contains the id token, the provider and the user's language setting
type LoginMessage struct {
	IdToken  string `json:"id_token" mapstructure:"id_token"`
	Provider string `json:"provider" mapstructure:"provider"`
	Language string `json:"language" mapstructure:"language"`
}
