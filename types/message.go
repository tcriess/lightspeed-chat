package types

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mitchellh/hashstructure/v2"
)

const (
	MessageTypeInfo        = "info"
	MessageTypeChat        = "chat"
	MessageTypeLogin       = "login"
	MessageTypeTranslation = "translation"
)

type omit *struct{}

// JSON-serialized WebsocketMessage is what is actually sent via the Websocket connection
type WebsocketMessage struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// The different types of messages

type InfoMessage struct {
	RoomName      string `json:"room_name"`
	NoConnections int    `json:"no_connections"`
}

type WireInfoMessage struct {
	*InfoMessage
}

func (msg WireInfoMessage) MarshalJSON() ([]byte, error) {
	type localWireInfoMessage WireInfoMessage
	data, err := json.Marshal(localWireInfoMessage(msg))
	if err != nil {
		return nil, err
	}
	m := WebsocketMessage{
		Event: MessageTypeInfo,
		Data:  data,
	}
	return json.Marshal(m)
}

// LoginMessage is sent to every client after connection
type LoginMessage struct {
	Nick string `json:"nick" mapstructure:"-"` // user nick, outgoing
}

type WireLoginMessage struct {
	*LoginMessage
}

func (msg WireLoginMessage) MarshalJSON() ([]byte, error) {
	type localWireLoginMessage WireLoginMessage
	data, err := json.Marshal(localWireLoginMessage(msg))
	if err != nil {
		return nil, err
	}
	m := WebsocketMessage{
		Event: MessageTypeLogin,
		Data:  data,
	}
	return json.Marshal(m)
}

// ChatMessage is a basic chat message, contains the sender nick
type ChatMessage struct {
	Id        string    `json:"id" hash:"ignore" mapstructure:"-"`       // Hash of the ChatMessage object, outgoing
	Nick      string    `json:"nick" mapstructure:"-"`                   // sender nick, outgoing
	Timestamp time.Time `json:"timestamp" mapstructure:"-"`              // sent time, outgoing
	Message   string    `json:"message" mapstructure:"message"`          // actual message, incoming + outgoing
	Language  string    `json:"language" hash:"ignore" mapstructure:"-"` // language of the message (for future use), outgoing
	Filter    string    `json:"filter" mapstructure:"filter"`            // filter expression incoming
}

type WireChatMessage struct {
	*ChatMessage
	Filter omit `json:"filter,omitempty"`
}

func (msg WireChatMessage) MarshalJSON() ([]byte, error) {
	type localWireChatMessage WireChatMessage
	data, err := json.Marshal(localWireChatMessage(msg))
	if err != nil {
		return nil, err
	}
	m := WebsocketMessage{
		Event: MessageTypeChat,
		Data:  data,
	}
	return json.Marshal(m)
}

// TranslationMessage contains a translation of a ChatMessage (SourceId contains the Id of that ChatMessage) into a specific Language
type TranslationMessage struct {
	SourceId  string    `json:"source_id" mapstructure:"-"` // Id of the corresponding ChatMessage, outgoing
	Timestamp time.Time `json:"timestamp" mapstructure:"-"` // sent time, outgoing (same is source message)
	Language  string    `json:"language" mapstructure:"-"`  // language code (ISO 639), outgoing
	Message   string    `json:"message" mapstructure:"-"`   // translated message, outgoing
	Filter    string    `json:"filter" mapstructure:"-"`    // same as source message
}

type WireTranslationMessage struct {
	*TranslationMessage
	Filter omit `json:"filter,omitempty"`
}

func (msg WireTranslationMessage) MarshalJSON() ([]byte, error) {
	type localWireTranslationMessage WireTranslationMessage
	data, err := json.Marshal(localWireTranslationMessage(msg))
	if err != nil {
		return nil, err
	}
	m := WebsocketMessage{
		Event: MessageTypeTranslation,
		Data:  data,
	}
	return json.Marshal(m)
}

// NewChatMessage return a pointer to a new ChatMessage with the given content and the correct Id
func NewChatMessage(nick string, timestamp time.Time, text string, language string) (*ChatMessage, error) {
	msg := &ChatMessage{
		Nick:      nick,
		Timestamp: timestamp,
		Message:   text,
		Language:  language,
	}
	err := msg.CreateId()
	return msg, err
}

func (msg *ChatMessage) CreateId() error {
	hash, err := hashstructure.Hash(msg, hashstructure.FormatV2, nil)
	if err != nil {
		log.Printf("error: could not hash chat message: %s", err)
		return err
	}
	msg.Id = fmt.Sprintf("%016X", hash)
	return nil
}
