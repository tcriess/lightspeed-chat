package persistence

import (
	"time"

	"github.com/tcriess/lightspeed-chat/types"
)

type Persister interface {
	StoreEvents([]*types.Event) error
	GetEventHistory(time.Time, time.Time, int, int) ([]*types.Event, error)
	StoreUser(types.User) error
	GetUser(*types.User) error
	DeleteUser(*types.User) error
	StoreRoom(types.Room) error
	GetRoom(*types.Room) error
	GetRooms() ([]*types.Room, error)
	DeleteRoom(*types.Room) error
	Close() error
}
