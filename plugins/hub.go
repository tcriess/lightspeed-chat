package plugins

import (
	"context"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/tcriess/lightspeed-chat/proto"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/zclconf/go-cty/cty"
	"google.golang.org/grpc"
)

/*
Per-hub (room) plugins interface.
*/

// Handshake is a common handshake that is shared by plugin and host.
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "LIGHTSPEED_CHAT_PLUGIN",
	MagicCookieValue: "388d8ee683cf839480ca5ba518a867b4924d1738d57ac88f4d90ba0730aaed86",
}

// PluginMap is the map of plugins we can dispense.
var PluginMap = map[string]plugin.Plugin{
	"eventhandler": &EventHandlerPlugin{},
}

type EmitEventsHelper interface {
	EmitEvents([]types.Event) error
}

// KV is the interface that we're exposing as a plugin.
type EventHandler interface {
	// GetSpec returns the HCL specification of the configuration block for the plugin
	GetSpec() (*hcldec.BlockSpec, error)

	// Configure returns the cron spec and the events filter
	Configure(cty.Value) (cronSpec string, eventsFilter string, err error)

	// Cron is invoked from the main process according to the cronSpec as returned by Configure.
	// Cron returns []types.Event to be emitted
	Cron() ([]types.Event, error)

	// HandleEvents is invoked every time a new event occurs, currently defined events are
	// new chat message, new translation, new command, new user login
	// the plugin only receives events that pass the eventsFilter returned by Configure
	HandleEvents([]types.Event) ([]types.Event, error)

	// InitEmitEvents never exits, it creates a permanent connection between the main program an the plugin
	// allowing the plugin to emit events at will
	InitEmitEvents(eh EmitEventsHelper) error
}

// This is the implementation of plugin.Plugin so we can serve/consume this.
// We also implement GRPCPlugin so that this plugin can be served over
// gRPC.
type EventHandlerPlugin struct {
	plugin.NetRPCUnsupportedPlugin

	// Concrete implementation, written in Go. This is only used for plugins
	// that are written in Go.
	Impl EventHandler
}

func (p *EventHandlerPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterEventHandlerServer(s, &GRPCServer{
		Impl:   p.Impl,
		broker: broker,
	})
	return nil
}

func (p *EventHandlerPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{
		client: proto.NewEventHandlerClient(c),
		broker: broker,
	}, nil
}

var _ plugin.GRPCPlugin = &EventHandlerPlugin{}
