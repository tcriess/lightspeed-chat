package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	translate "cloud.google.com/go/translate/apiv3"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/golang-lru"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/zclconf/go-cty/cty"
	translatepb "google.golang.org/genproto/googleapis/cloud/translate/v3"
)

const (
	translatorNick         = "translatorBot"
	translatorText         = "translatorBot active"
	translatorTextLanguage = "en-US"
	pluginName             = "google-translate"
)

var (
	projectId string
	languages []string
	cache     *lru.ARCCache
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
	appLogger.Info("in HandleEvents", "events", events, "projectId", projectId, "languages", languages)
	outEvents := make([]*types.Event, 0)
	for _, event := range events {
		if event.Name != types.EventTypeChat {
			continue
		}
		message, ok := event.Tags["message"]
		if !ok {
			continue
		}
		if strings.HasPrefix(message, "/") {
			continue
		}
		source := &types.Source{
			User:       event.User,
			PluginName: pluginName,
		}
		for _, language := range languages {
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
	}
	return outEvents, nil
}

func (m *EventHandler) GetSpec() (*hcldec.BlockSpec, error) {
	spec := hcldec.BlockSpec{
		TypeName: "config",
		Nested: &hcldec.ObjectSpec{
			"project_id": &hcldec.AttrSpec{
				Name: "project_id",
				Type: cty.String,
			},
			"languages": &hcldec.AttrSpec{
				Name: "languages",
				Type: cty.List(cty.String),
			},
			"cron_spec": &hcldec.AttrSpec{
				Name: "cron_spec",
				Type: cty.String,
			},
			"cache_size": &hcldec.AttrSpec{
				Name: "cache_size",
				Type: cty.Number,
			},
			"log_level": &hcldec.AttrSpec{
				Name:     "log_level",
				Type:     cty.String,
				Required: false,
			},
		},
		Required: false,
	}
	return &spec, nil
}

func (m *EventHandler) Configure(val cty.Value) (string, string, error) {
	logLevelAttr := val.GetAttr("log_level")
	if !logLevelAttr.IsNull() {
		logLevel := logLevelAttr.AsString()
		if logLevel != "" {
			appLogger.SetLevel(hclog.LevelFromString(logLevel))
		}
	}
	appLogger.Info("in plugin configure", "val", val)
	projectIdAttr := val.GetAttr("project_id")
	projectId = projectIdAttr.AsString()
	appLogger.Debug("got", "projectId", projectId)
	languagesAttr := val.GetAttr("languages")
	languagesVals := languagesAttr.AsValueSlice()
	languages = make([]string, len(languagesVals))
	for i, lv := range languagesVals {
		languages[i] = lv.AsString()
	}
	cronSpec := val.GetAttr("cron_spec").AsString()
	appLogger.Debug("got", "cronSpec", cronSpec)
	appLogger.Debug("got", "languages", languages)
	cacheSizeFl := val.GetAttr("cache_size").AsBigFloat()
	cacheSize, _ := cacheSizeFl.Uint64()
	if cacheSize > 0 {
		if c, err := lru.NewARC(int(cacheSize)); err == nil {
			cache = c
		} else {
			appLogger.Error("could not create lru cache", "error", err)
		}
	}
	eventFilter := `Name == "chat"`
	return cronSpec, eventFilter, nil
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
		Parent:             fmt.Sprintf("projects/%s/locations/global", projectId),
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
			translations[toTranslateIdx[i]] = t.TranslatedText
			k := cacheKey{
				TargetLanguage: language,
				Text:           srcText[toTranslateIdx[i]],
			}
			cache.Add(k, t.TranslatedText)
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
