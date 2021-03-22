package persistence

import (
	"time"

	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/types"
)

type Persister interface {
	StoreEvents(*types.Room, []*types.Event) error
	GetEventHistory(*types.Room, time.Time, time.Time, int, int) ([]*types.Event, error)
	StoreUser(types.User) error
	GetUser(*types.User) error
	GetUsers() ([]*types.User, error)
	UpdateUserTags(*types.User, []*types.TagUpdate) ([]bool, error)
	DeleteUser(*types.User) error
	StoreRoom(types.Room) error
	GetRoom(*types.Room) error
	GetRooms() ([]*types.Room, error)
	UpdateRoomTags(*types.Room, []*types.TagUpdate) ([]bool, error)
	DeleteRoom(*types.Room) error
	Close() error
}

func NewPersister(globalConfig *config.Config) (Persister, error) {
	persister, err := NewSQLitePersister(globalConfig)
	if err != nil {
		return nil, err
	}
	if persister == nil {
		persister, err = NewBuntPersister(globalConfig)
		if err != nil {
			return nil, err
		}
	}
	return persister, nil
}
