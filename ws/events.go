package ws

import (
	"github.com/hashicorp/go-hclog"
	"github.com/tcriess/lightspeed-chat/types"
)

type emitEventsHelper struct {
	hub        *Hub
	pluginName string
}

// Here, we receive the events that were emitted by the plugins
func (eh *emitEventsHelper) EmitEvents(events []*types.Event) error {
	hclog.Default().Info("in main->emitEvents", "events", events)
	skipPlugins := make(map[string]struct{})
	skipPlugins[eh.pluginName] = struct{}{}
	err := eh.hub.handlePlugins(events, skipPlugins)
	if err != nil {
		hclog.Default().Error("could not handle events", "error", err)
		return err
	}
	err = eh.hub.handleEvents(events)
	if err != nil {
		hclog.Default().Error("could not handle events", "error", err)
		return err
	}
	return nil
}
