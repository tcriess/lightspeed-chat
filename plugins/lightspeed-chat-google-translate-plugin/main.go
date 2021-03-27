package main

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	translate "cloud.google.com/go/translate/apiv3"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/golang-lru"
	"github.com/mitchellh/mapstructure"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
	translatepb "google.golang.org/genproto/googleapis/cloud/translate/v3"
)

const (
	translatorNick     = "translatorBot"
	translatorText     = "translatorBot active"
	translatorHelpText = `### Translator plugin ###
 -> all chat messages are automatically translated into all supported languages, no further commands are required`
	helpCommand            = "/help"
	translatorTextLanguage = "en-US"
	pluginName             = "google-translate"
)

type config struct {
	LogLevel  string   `mapstructure:"log_level"`
	CronSpec  string   `mapstructure:"cron_spec"`
	ProjectId string   `mapstructure:"project_id"`
	Languages []string `mapstructure:"languages"`
	CacheSize int      `mapstructure:"cache_size"`
}

var (
	pluginConfig config
	cache        *lru.ARCCache
)

type cacheKey struct {
	TargetLanguage string
	Text           string
}

var appLogger = hclog.New(&hclog.LoggerOptions{
	Name:  pluginName,
	Level: hclog.LevelFromString("DEBUG"),
})

// Here is a real implementation of the plugin interface
type EventHandler struct{}

func (m *EventHandler) HandleEvents(events []*types.Event) ([]*types.Event, error) {
	appLogger.Info("in HandleEvents", "events", events, "projectId", pluginConfig.ProjectId, "languages", pluginConfig.Languages)
	outEvents := make([]*types.Event, 0)

	for _, event := range events {
		source := &types.Source{
			User:       event.User,
			PluginName: pluginName,
		}
		switch event.Name {
		case types.EventTypeChat:
			message, ok := event.Tags["message"]
			if !ok {
				continue
			}
			if strings.HasPrefix(message, "/") { // should never happen, messages starting with "/" are commands
				continue
			}

			for _, language := range pluginConfig.Languages {
				isoLang := language[0:2]
				res, err := translation([]string{message}, language)
				if err != nil {
					return outEvents, err
				}
				if len(res) == 0 {
					appLogger.Info("no translation")
					continue
				}
				if res[0] != "" {
					filter := event.TargetFilter
					if filter != "" {
						filter = fmt.Sprintf(`( %s ) && Target.Client.ClientLanguage startsWith %s`, filter, strconv.Quote(isoLang))
					} else {
						filter = fmt.Sprintf(`Target.Client.ClientLanguage startsWith %s`, strconv.Quote(isoLang))
					}
					tags := map[string]string{
						"message":   res[0],
						"source_id": event.Id,
					}
					outEvent := types.NewEvent(event.Room, source, filter, isoLang, types.EventTypeTranslation, tags)
					outEvents = append(outEvents, outEvent)
				}
			}

		case types.EventTypeCommand:
			command, ok := event.Tags["command"]
			if !ok {
				continue
			}
			switch command {
			case helpCommand:
				tags := map[string]string{
					"message": translatorHelpText,
				}
				outEvent := types.NewEvent(event.Room, source, fmt.Sprintf(`Target.User.Id == %s`, strconv.Quote(event.Source.User.Id)), translatorTextLanguage, types.EventTypeChat, tags)
				translatedEvents, _ := m.HandleEvents([]*types.Event{outEvent})
				outEvents = append(outEvents, outEvent)
				if len(translatedEvents) > 0 {
					outEvents = append(outEvents, translatedEvents...)
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
	if pluginConfig.CacheSize > 0 {
		if c, err := lru.NewARC(int(pluginConfig.CacheSize)); err == nil {
			cache = c
		} else {
			appLogger.Error("could not create lru cache", "error", err)
		}
	}
	eventFilter := fmt.Sprintf(`(Name=="command" && (Tags["command"] in [%s])) || Name == "chat"`, strconv.Quote(helpCommand))
	return pluginConfig.CronSpec, eventFilter, nil
}

func (m *EventHandler) Cron(room *types.Room) ([]*types.Event, error) {
	tags := map[string]string{
		"message": translatorText,
	}
	source := &types.Source{
		User:       &types.User{Nick: translatorNick},
		PluginName: pluginName,
	}
	event := types.NewEvent(room, source, "", translatorTextLanguage, types.EventTypeChat, tags)
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

func translation(srcText []string, language string) ([]string, error) {
	appLogger.Info("in translation", "srcText", srcText, "language", language)
	translations := make([]string, len(srcText))
	if len(srcText) == 0 {
		return translations, nil
	}
	toTranslateIdx := make([]int, 0)
	for i, s := range srcText {
		k := cacheKey{
			TargetLanguage: language,
			Text:           s,
		}
		if v, ok := cache.Get(k); ok {
			translations[i] = v.(string)
			appLogger.Debug("found translation in cache!")
		} else {
			toTranslateIdx = append(toTranslateIdx, i)
		}
	}
	if len(toTranslateIdx) == 0 {
		return translations, nil
	}
	toTranslate := make([]string, len(toTranslateIdx))
	for i, idx := range toTranslateIdx {
		toTranslate[i] = srcText[idx]
	}
	ctx := context.Background()
	ctx, _ = context.WithTimeout(ctx, time.Second)
	c, err := translate.NewTranslationClient(ctx)
	if err != nil {
		appLogger.Error("could not create translation client", "error", err)
		return nil, err
	}
	req := &translatepb.TranslateTextRequest{
		Contents: toTranslate, // srcText,
		//MimeType:           "",
		//SourceLanguageCode: "",
		TargetLanguageCode: language,
		Parent:             fmt.Sprintf("projects/%s/locations/global", pluginConfig.ProjectId),
		//Model:              "",
		//GlossaryConfig:     nil,
		//Labels:             nil,
	}
	resp, err := c.TranslateText(ctx, req)
	if err != nil {
		appLogger.Error("could not translate", "error", err)
		return nil, err
	}
	for i, t := range resp.Translations {
		if t.DetectedLanguageCode[:2] != language[:2] {
			translations[toTranslateIdx[i]] = html.UnescapeString(t.TranslatedText)
			k := cacheKey{
				TargetLanguage: language,
				Text:           srcText[toTranslateIdx[i]],
			}
			cache.Add(k, html.UnescapeString(t.TranslatedText))
		}
	}
	appLogger.Debug("translated", "translations", translations)
	return translations, nil
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
