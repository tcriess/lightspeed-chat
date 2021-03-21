package plugins

import (
	"fmt"
	"sync"

	"github.com/tcriess/lightspeed-chat/types"
)

type HelperFunctionsType struct {
	implEmitEvents       func([]*types.Event) error
	implGetRoom          func(string) (*types.Room, error)
	implGetUser          func(string) (*types.User, error)
	implAuthenticateUser func(string, string) (*types.User, error)
	implChangeRoomTags   func(string, []*types.TagUpdate) (*types.Room, []bool, error)
	implChangeUserTags   func(string, []*types.TagUpdate) (*types.User, []bool, error)
	sync.RWMutex
}

func (h *HelperFunctionsType) Clear() {
	h.Lock()
	defer h.Unlock()
	h.implEmitEvents = nil
	h.implGetRoom = nil
	h.implGetUser = nil
	h.implAuthenticateUser = nil
	h.implChangeRoomTags = nil
	h.implChangeUserTags = nil
}

func (h *HelperFunctionsType) Set(eh EmitEventsHelper) {
	h.Lock()
	defer h.Unlock()
	h.implEmitEvents = eh.EmitEvents
	h.implGetRoom = eh.GetRoom
	h.implGetUser = eh.GetUser
	h.implAuthenticateUser = eh.AuthenticateUser
	h.implChangeRoomTags = eh.ChangeRoomTags
	h.implChangeUserTags = eh.ChangeUserTags
}

func (h *HelperFunctionsType) EmitEvents(events []*types.Event) error {
	h.RLock()
	if e := h.implEmitEvents; e != nil {
		h.RUnlock()
		return e(events)
	}
	h.RUnlock()
	return fmt.Errorf("lost connection")
}

func (h *HelperFunctionsType) GetRoom(roomId string) (*types.Room, error) {
	h.RLock()
	if r := h.implGetRoom; r != nil {
		h.RUnlock()
		return r(roomId)
	}
	h.RUnlock()
	return nil, fmt.Errorf("lost connection")
}

func (h *HelperFunctionsType) GetUser(userId string) (*types.User, error) {
	h.RLock()
	if u := h.implGetUser; u != nil {
		h.RUnlock()
		return u(userId)
	}
	h.RUnlock()
	return nil, fmt.Errorf("lost connection")
}

func (h *HelperFunctionsType) AuthenticateUser(token string, provider string) (*types.User, error) {
	h.RLock()
	if u := h.implAuthenticateUser; u != nil {
		h.RUnlock()
		return u(token, provider)
	}
	h.RUnlock()
	return nil, fmt.Errorf("lost connection")
}

func (h *HelperFunctionsType) ChangeRoomTags(roomId string, updates []*types.TagUpdate) (*types.Room, []bool, error) {
	h.RLock()
	if r := h.implChangeRoomTags; r != nil {
		h.RUnlock()
		return r(roomId, updates)
	}
	h.RUnlock()
	return nil, nil, fmt.Errorf("lost connection")
}

func (h *HelperFunctionsType) ChangeUserTags(userId string, updates []*types.TagUpdate) (*types.User, []bool, error) {
	h.RLock()
	if u := h.implChangeUserTags; u != nil {
		h.RUnlock()
		return u(userId, updates)
	}
	h.RUnlock()
	return nil, nil, fmt.Errorf("lost connection")
}
