package ws

import (
	"github.com/hashicorp/go-hclog"
	"github.com/tcriess/lightspeed-chat/auth"
	"github.com/tcriess/lightspeed-chat/types"
)

type emitEventsHelper struct {
	hub        *Hub
	pluginName string
}

// Here, we receive the events that were emitted by the plugins
func (eh *emitEventsHelper) EmitEvents(events []*types.Event) error {
	hclog.Default().Info("in main->emitEvents", "events", events)
	keepEvents := events[:0]
	for _, event := range events {
		if event.Name == types.EventTypeInternal {
			// internal requests from plugins to main
			eh.handleInternalEvent(event)
		} else {
			keepEvents = append(keepEvents, event)
		}
	}
	// garbage collect
	for i := len(keepEvents); i < len(events); i++ {
		events[i] = nil
	}
	skipPlugins := make(map[string]struct{})
	skipPlugins[eh.pluginName] = struct{}{}
	err := eh.hub.handlePlugins(keepEvents, skipPlugins)
	if err != nil {
		hclog.Default().Error("could not handle events", "error", err)
		return err
	}
	err = eh.hub.handleEvents(keepEvents)
	if err != nil {
		hclog.Default().Error("could not handle events", "error", err)
		return err
	}
	return nil
}

func (eh *emitEventsHelper) handleInternalEvent(event *types.Event) {
	// TODO: for future use, currently no internal events are processed
}

func (eh *emitEventsHelper) AuthenticateUser(idToken string, provider string) (*types.User, error) {
	// TODO: possibly allow for new users to be accepted here
	userId, err := auth.Authenticate(idToken, provider, eh.hub.Cfg)
	if err != nil {
		return nil, err
	}
	user := &types.User{Id: userId}
	if userId != "" && eh.hub.Persister != nil {
		err := eh.hub.Persister.GetUser(user)
		if err != nil {
			return nil, err
		}
	}
	return user, nil
}

func (eh *emitEventsHelper) GetUser(userId string) (*types.User, error) {
	user := &types.User{Id: userId}
	if eh.hub.Persister != nil {
		err := eh.hub.Persister.GetUser(user)
		if err != nil {
			return nil, err
		}
	}
	return user, nil
}

func (eh *emitEventsHelper) GetRoom(roomId string) (*types.Room, error) {
	room := &types.Room{Id: roomId}
	if eh.hub.Persister != nil {
		err := eh.hub.Persister.GetRoom(room)
		if err != nil {
			return nil, err
		}
	}
	return room, nil
}

func (eh *emitEventsHelper) ChangeUserTags(userId string, updates []*types.TagUpdate) (*types.User, []bool, error) {
	resOk := make([]bool, len(updates))
	user := &types.User{Id: userId}
	if eh.hub.Persister != nil {
		var err error
		resOk, err = eh.hub.Persister.UpdateUserTags(user, updates)
		if err != nil {
			return nil, nil, err
		}
	}
	return user, resOk, nil
}

func (eh *emitEventsHelper) ChangeRoomTags(roomId string, updates []*types.TagUpdate) (*types.Room, []bool, error) {
	resOk := make([]bool, len(updates))
	room := &types.Room{Id: roomId}
	if eh.hub.Persister != nil {
		var err error
		resOk, err = eh.hub.Persister.UpdateRoomTags(room, updates)
		if err != nil {
			return nil, nil, err
		}
	}
	return room, resOk, nil
}
