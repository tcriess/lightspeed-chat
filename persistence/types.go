package persistence

import (
	"time"

	"github.com/tcriess/lightspeed-chat/types"
)

type Persister interface {
	StoreChatMessage(types.ChatMessage) error
	StoreTranslationMessage(types.TranslationMessage) error
	GetChatHistory(time.Time, time.Time, int, int) ([]types.ChatMessage, error)
	GetTranslationHistory(time.Time, time.Time, int, int) ([]types.TranslationMessage, error)
	StoreUser(types.User) error
	GetUser(*types.User) error
	Close() error
}
