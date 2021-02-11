package types

const (
	EventTypeMessage     = 1
	EventTypeTranslation = 2
	EventTypeCommand     = 3
	EventTypeUserLogin   = 4
)

type Event interface {
	GetEventType() int
}

type EventBase struct {
	EventType int `json:"event_type"`
}

func (e *EventBase) GetEventType() int {
	return e.EventType
}

type EventCommand struct {
	*EventBase
	*Command
}

type EventMessage struct {
	*EventBase
	*ChatMessage
}

type EventTranslation struct {
	*EventBase
	*TranslationMessage
}

type EventUserLogin struct {
	*EventBase
	*User
}

func NewMessageEvent(message ChatMessage) *EventMessage {
	return &EventMessage{
		EventBase:   &EventBase{EventType: EventTypeMessage},
		ChatMessage: &message,
	}
}

func NewTranslationEvent(translation TranslationMessage) *EventTranslation {
	return &EventTranslation{
		EventBase:          &EventBase{EventType: EventTypeTranslation},
		TranslationMessage: &translation,
	}
}

func NewCommandEvent(command Command) *EventCommand {
	return &EventCommand{
		EventBase: &EventBase{EventType: EventTypeCommand},
		Command:   &command,
	}
}

func NewUserLoginEvent(user User) *EventUserLogin {
	return &EventUserLogin{
		EventBase: &EventBase{EventType: EventTypeUserLogin},
		User:      &user,
	}
}