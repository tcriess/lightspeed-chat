package main

import (
	"fmt"
	"plugin"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/mitchellh/mapstructure"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
)

const (
	baseCommandsNick     = "baseCommandsBot"
	baseCommandsText     = "baseCommandsBot active"
	baseCommandsHelpText = `### Base commands plugin ###
 -> /to <nick> <message> - send private message to <nick>
 -> /fg <color> <message> - use <color> as text color`
	helpCommand              = "/help"
	toCommand                = "/to"
	fgCommand                = "/fg"
	baseCommandsTextLanguage = "en-US"
	pluginName               = "base-commands"
)

type config struct {
	LogLevel string `mapstructure:"log_level"`
	CronSpec string `mapstructure:"cron_spec"`
}

var (
	pluginConfig config
)

var appLogger = hclog.New(&hclog.LoggerOptions{
	Name:  pluginName,
	Level: hclog.LevelFromString("DEBUG"),
})

func handleToCommand(inEvent *types.Event) ([]*types.Event, error) {
	if inEvent.Name != types.EventTypeCommand {
		return nil, nil
	}
	if cmd, ok := inEvent.Tags["command"]; !ok || cmd != toCommand {
		return nil, nil
	}
	var args string
	var ok bool
	if args, ok = inEvent.Tags["args"]; !ok || args == "" {
		return nil, nil
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return nil, nil
	}
	toNick := fields[0]
	var message string
	if message, ok = inEvent.Tags["message"]; !ok || !strings.HasPrefix(message, toCommand) || len(message) < len(toCommand)+1 {
		return nil, nil
	}
	message = strings.TrimSpace(message[len(toCommand):])
	fields = strings.Fields(message)
	if len(fields) == 0 {
		return nil, nil
	}
	if len(message) < len(fields[0]) {
		return nil, nil
	}
	message = strings.TrimSpace(message[len(fields[0]):])
	if len(message) == 0 {
		return nil, nil
	}
	targetFilter := fmt.Sprintf(`Target.User.Nick == %s`, strconv.Quote(toNick))
	if inEvent.Source.User.Id != "" && inEvent.Source.PluginName == "" {
		targetFilter = fmt.Sprintf(`(%s || Target.User.Id == %s)`, targetFilter, strconv.Quote(inEvent.Source.User.Id))
	} else {
		if inEvent.Source.User.Nick != "" && inEvent.Source.PluginName == "" {
			targetFilter = fmt.Sprintf(`(%s || Target.User.Nick == %s)`, targetFilter, strconv.Quote(inEvent.Source.User.Nick))
		}
	}
	if originalTargetFilter, ok := inEvent.Tags["original_target_filter"]; ok && originalTargetFilter != "" {
		targetFilter = fmt.Sprintf(`%s && (%s)`, targetFilter, originalTargetFilter)
	}
	mimeType := "text/plain"
	if mt, ok := inEvent.Tags["mime_type"]; ok {
		mimeType = mt
	}
	source := &types.Source{
		User:       inEvent.User,
		PluginName: pluginName,
	}
	tags := make(map[string]string)
	tags["message"] = message
	tags["mime_type"] = mimeType
	event := types.NewEvent(inEvent.Room, source, targetFilter, inEvent.Language, types.EventTypeChat, tags)
	events := []*types.Event{event}
	return events, nil
}

func handleFgCommand(inEvent *types.Event) ([]*types.Event, error) {
	if inEvent.Name != types.EventTypeCommand {
		return nil, nil
	}
	if cmd, ok := inEvent.Tags["command"]; !ok || cmd != fgCommand {
		return nil, nil
	}
	var args string
	var ok bool
	if args, ok = inEvent.Tags["args"]; !ok || args == "" {
		return nil, nil
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return nil, nil
	}
	fgColor := fields[0]
	var message string
	if message, ok = inEvent.Tags["message"]; !ok || !strings.HasPrefix(message, fgCommand) || len(message) < len(fgCommand)+1 {
		return nil, nil
	}
	message = strings.TrimSpace(message[len(fgCommand):])
	fields = strings.Fields(message)
	if len(fields) == 0 {
		return nil, nil
	}
	if len(message) < len(fields[0]) {
		return nil, nil
	}
	message = strings.TrimSpace(message[len(fields[0]):])
	if len(message) == 0 {
		return nil, nil
	}
	mimeType := "text/plain"
	if mt, ok := inEvent.Tags["mime_type"]; ok {
		mimeType = mt
	}
	source := &types.Source{
		User:       inEvent.User,
		PluginName: pluginName,
	}
	tags := make(map[string]string)
	tags["message"] = message
	tags["mime_type"] = mimeType
	tags["fg_color"] = fgColor
	event := types.NewEvent(inEvent.Room, source, "", inEvent.Language, types.EventTypeChat, tags)
	events := []*types.Event{event}
	return events, nil
}

func handleHelpCommand(inEvent *types.Event) ([]*types.Event, error) {
	source := &types.Source{
		User:       inEvent.User,
		PluginName: pluginName,
	}
	tags := map[string]string{
		"message":   baseCommandsHelpText,
		"mime_type": "text/plain",
	}
	outEvent := types.NewEvent(inEvent.Room, source, fmt.Sprintf(`Target.User.Id == %s`, strconv.Quote(inEvent.Source.User.Id)), baseCommandsTextLanguage, types.EventTypeChat, tags)
	events := []*types.Event{outEvent}
	return events, nil
}

type cmdHandlerFunc func(*types.Event) ([]*types.Event, error)

var (
	commands = map[string]cmdHandlerFunc{
		toCommand:   handleToCommand,
		fgCommand:   handleFgCommand,
		helpCommand: handleHelpCommand,
	}
)

// Here is a real implementation of the plugin interface
type EventHandler struct{}

func (m *EventHandler) HandleEvents(events []*types.Event) ([]*types.Event, error) {
	appLogger.Info("in HandleEvents")
	outEvents := make([]*types.Event, 0)

	for _, event := range events {
		switch event.Name {
		case types.EventTypeCommand:
			if cmd, ok := event.Tags["command"]; ok {
				if commandFunc, ok := commands[cmd]; ok {
					res, err := commandFunc(event)
					if err != nil {
						appLogger.Error("could not execute base command:", "error", err)
						continue
					}
					if len(res) > 0 {
						outEvents = append(outEvents, res...)
					}
				}
			}

		default:
			continue
		}

	}
	return outEvents, nil
}

func (m *EventHandler) Configure(val map[string]interface{}) (string, string, error) {
	err := mapstructure.WeakDecode(val, &pluginConfig)
	if err != nil {
		return "", "", err
	}
	if pluginConfig.LogLevel != "" {
		appLogger.SetLevel(hclog.LevelFromString(pluginConfig.LogLevel))
	}
	appLogger.Info("in plugin configure", "val", val)
	quotedCommands := []string{
		strconv.Quote(helpCommand),
		strconv.Quote(toCommand),
		strconv.Quote(fgCommand),
	}
	eventFilter := fmt.Sprintf(`Name=="command" && (Tags["command"] in [%s])`, strings.Join(quotedCommands, ","))
	return pluginConfig.CronSpec, eventFilter, nil
}

func (m *EventHandler) Cron(room *types.Room) ([]*types.Event, error) {
	tags := map[string]string{
		"message": baseCommandsText,
	}
	source := &types.Source{
		User:       &types.User{Nick: baseCommandsNick},
		PluginName: pluginName,
	}
	event := types.NewEvent(room, source, "", baseCommandsTextLanguage, types.EventTypeChat, tags)
	events := []*types.Event{event}
	outEvents, err := m.HandleEvents(events)
	if err != nil {
		return events, err
	}
	events = append(events, outEvents...)
	return events, nil
}

// make this run forever!
func (m *EventHandler) InitEmitEvents(room *types.Room, eh plugins.EmitEventsHelper) error {
	appLogger.Info("in plugin initEmitEvents")

	appLogger.Debug("start emit events loop")
	for {
		<-time.After(60 * time.Second)
	}

	return nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugins.Handshake,
		Plugins: map[string]plugin.Plugin{
			"eventhandler": &plugins.EventHandlerPlugin{Impl: &EventHandler{}},
		},

		// A non-nil value here enables gRPC serving for this plugin...
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
