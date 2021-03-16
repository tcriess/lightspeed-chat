package ws

import (
	"log"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/tcriess/lightspeed-chat/filter"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/types"
)

func (c *Client) EvaluateFilterEvent(event *types.Event) bool {
	if event.TargetFilter == "" {
		return true
	}
	globals.AppLogger.Debug("creating filter program for", "event", event)
	prog, err := expr.Compile(event.TargetFilter, expr.Env(filter.Env{}))
	if err != nil {
		log.Printf("error: could not compile filter: %s", err)
		return false
	}
	return c.RunFilterEvent(event, prog)
}

func (c *Client) RunFilterEvent(event *types.Event, prog *vm.Program) bool {
	if event == nil {
		return false
	}
	if prog == nil {
		return true
	}
	env := filter.Env{
		Room: filter.Room{
			Id: c.hub.Room.Id,
			Owner: filter.User{
				Id:         c.hub.Room.Owner.Id,
				Nick:       c.hub.Room.Owner.Nick,
				Language:   c.hub.Room.Owner.Language,
				Tags:       c.hub.Room.Owner.Tags,
				LastOnline: c.hub.Room.Owner.LastOnline.Unix(),
			},
			Tags: c.hub.Room.Tags,
		},
		Source: filter.Source{
			User: filter.User{
				Id:         event.Source.User.Id,
				Nick:       event.Source.User.Nick,
				Language:   event.Source.User.Language,
				Tags:       event.Source.User.Tags,
				LastOnline: event.Source.User.LastOnline.Unix(),
			},
			PluginName: event.Source.PluginName,
		},
		Target: filter.Target{
			User: filter.User{
				Id:         c.user.Id,
				Nick:       c.user.Nick,
				Language:   c.user.Language,
				Tags:       c.user.Tags,
				LastOnline: c.user.LastOnline.Unix(),
			},
			Client: filter.Client{
				ClientLanguage: c.Language,
			},
		},
		Created:       event.Created.Unix(),
		Language:      event.Language,
		Name:          event.Name,
		Tags:          event.Tags,
		AsInt:         filter.AsInt,
		AsFloat:       filter.AsFloat,
		AsStringSlice: filter.AsStringSlice,
		AsIntSlice:    filter.AsIntSlice,
		AsFloatSlice:  filter.AsFloatSlice,
	}
	globals.AppLogger.Debug("running filter", "env.Target.Client.ClientLanguage", env.Client.ClientLanguage, "event.Language", event.Language, "env", env, "event", event)
	res, err := expr.Run(prog, env)
	if err != nil {
		globals.AppLogger.Error("error: could not run filter: %s", err)
		return false
	}
	globals.AppLogger.Debug("filter result:", "res", res)
	if bRes, ok := res.(bool); ok && bRes {
		return true
	}

	return false
}

func (h *Hub) EvaluatePluginFilterEvent(event *types.Event, pluginFilter string) bool {
	if event.TargetFilter == "" {
		return true
	}
	prog, err := expr.Compile(pluginFilter, expr.Env(filter.Env{}))
	if err != nil {
		log.Printf("error: could not compile filter: %s", err)
		return false
	}
	return h.RunPluginFilterEvent(event, prog)
}

func (h *Hub) RunPluginFilterEvent(event *types.Event, prog *vm.Program) bool {
	if event == nil {
		return false
	}
	if prog == nil {
		return true
	}
	env := filter.Env{
		Room: filter.Room{
			Id: h.Room.Id,
			Owner: filter.User{
				Id:         h.Room.Owner.Id,
				Nick:       h.Room.Owner.Nick,
				Language:   h.Room.Owner.Language,
				Tags:       h.Room.Owner.Tags,
				LastOnline: h.Room.Owner.LastOnline.Unix(),
			},
			Tags: h.Room.Tags,
		},
		Source: filter.Source{
			User: filter.User{
				Id:         event.Source.User.Id,
				Nick:       event.Source.User.Nick,
				Language:   event.Source.User.Language,
				Tags:       event.Source.User.Tags,
				LastOnline: event.Source.User.LastOnline.Unix(),
			},
			PluginName: event.Source.PluginName,
		},
		Target: filter.Target{
			User:   filter.User{},
			Client: filter.Client{},
		},
		Created:       event.Created.Unix(),
		Language:      event.Language,
		Name:          event.Name,
		Tags:          event.Tags,
		AsInt:         filter.AsInt,
		AsFloat:       filter.AsFloat,
		AsStringSlice: filter.AsStringSlice,
		AsIntSlice:    filter.AsIntSlice,
		AsFloatSlice:  filter.AsFloatSlice,
	}

	res, err := expr.Run(prog, env)
	if err != nil {
		log.Printf("error: could not run filter: %s", err)
		return false
	}
	if bRes, ok := res.(bool); ok {
		if bRes {
			return true
		}
	}

	return false
}
