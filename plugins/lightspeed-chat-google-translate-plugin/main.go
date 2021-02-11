package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	translate "cloud.google.com/go/translate/apiv3"
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

// Here is a real implementation of KV that writes to a local file with
// the key name and the contents are the value of the key.
type EventHandler struct{}

func (m *EventHandler) HandleEvents(events []types.Event) ([]types.Event, error) {
	log.Printf("in HandleEvents, about to translate %v", events)
	log.Printf("project id: %v", projectId)
	log.Printf("languages: %v", languages)
	outEvents := make([]types.Event, 0)
	for _, event := range events {
		if event.GetEventType() != types.EventTypeMessage {
			continue
		}
		msgEvent := event.(*types.EventMessage)
		message := msgEvent.ChatMessage
		if strings.HasPrefix(message.Message, "/") {
			return outEvents, nil
		}
		//translations := make([]types.TranslationMessage, 0)
		for _, language := range languages {
			isoLang := language[0:2]
			res, err := translation([]string{message.Message}, language)
			if err != nil {
				return outEvents, err
			}
			if len(res) == 0 {
				log.Println("no translation")
				continue
			}
			if res[0] != "" {
				f := message.Filter
				if f != "" {
					f = fmt.Sprintf(`( %s ) && ClientLanguage startsWith %s`, f, strconv.Quote(isoLang))
				} else {
					f = fmt.Sprintf(`ClientLanguage startsWith %s`, strconv.Quote(isoLang))
				}
				trans := types.TranslationMessage{
					SourceId:  message.Id,
					Timestamp: message.Timestamp,
					Language:  isoLang,
					Message:   res[0],
					Filter:    f,
				}
				outEvent := types.NewTranslationEvent(trans)
				outEvents = append(outEvents, outEvent)
			}
		}
	}
	return outEvents, nil
}

/*
func (m *EventHandler) HandleMessage(message types.ChatMessage, bc plugins.BroadcastHelper) error {
	log.Printf("in HandleMessage, about to translate %v", message)
	log.Printf("project id: %v", projectId)
	log.Printf("languages: %v", languages)
	if strings.HasPrefix(message.Message, "/") {
		return nil
	}
	translations := make([]types.TranslationMessage, 0)
	for _, language := range languages {
		isoLang := language[0:2]
		res, err := translation([]string{message.Message}, language)
		if err != nil {
			return err
		}
		if len(res) == 0 {
			log.Println("no translation")
			return nil
		}
		if res[0] != "" {
			f := message.Filter
			if f != "" {
				f = fmt.Sprintf(`( %s ) && Client.Language startsWith %s`, f, strconv.Quote(isoLang))
			} else {
				f = fmt.Sprintf(`Client.Language startsWith %s`, strconv.Quote(isoLang))
			}
			trans := types.TranslationMessage{
				SourceId:  message.Id,
				Timestamp: message.Timestamp,
				Language:  isoLang,
				Message:   res[0],
				Filter:    f,
			}
			translations = append(translations, trans)
		}
	}
	err := bc.Broadcast(nil, translations, false, nil)
	if err != nil {
		return err
	}
	return nil
}
*/

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
		},
		Required: false,
	}
	return &spec, nil
}

func (m *EventHandler) Configure(val cty.Value) (string, string, error) {
	log.Printf("in plugin configure, value: %+v", val)
	projectIdAttr := val.GetAttr("project_id")
	projectId = projectIdAttr.AsString()
	log.Printf("got projectId = %s", projectId)
	languagesAttr := val.GetAttr("languages")
	languagesVals := languagesAttr.AsValueSlice()
	languages = make([]string, len(languagesVals))
	for i, lv := range languagesVals {
		languages[i] = lv.AsString()
	}
	cronSpec := val.GetAttr("cron_spec").AsString()
	log.Printf("cronspec: %v", cronSpec)
	log.Printf("languages: %v", languages)
	log.Printf("val: %+v", val)
	cacheSizeFl := val.GetAttr("cache_size").AsBigFloat()
	cacheSize, _ := cacheSizeFl.Uint64()
	if cacheSize > 0 {
		if c, err := lru.NewARC(int(cacheSize)); err == nil {
			cache = c
		} else {
			log.Printf("error: could not create lru cache: %s", err)
		}
	}
	eventFilter := ""
	return cronSpec, eventFilter, nil
}

func (m *EventHandler) Cron() ([]types.Event, error) {
	msg, err := types.NewChatMessage(translatorNick, time.Now(), translatorText, translatorTextLanguage)
	if err != nil {
		return nil, err
	}
	events := []types.Event{types.NewMessageEvent(*msg)}
	outEvents, err := m.HandleEvents(events)
	if err != nil {
		return events, err
	}
	events = append(events, outEvents...)
	return events, nil
}

// make this run forever!
func (m *EventHandler) InitEmitEvents(eh plugins.EmitEventsHelper) error {
	log.Println("in plugin initEmitEvents")

	cm := types.ChatMessage{
		Id:        "TRANS",
		Nick:      "TRANS",
		Timestamp: time.Now(),
		Message:   "TEST",
		Language:  "en",
		Filter:    `User.Id startsWith "tcriess"`,
	}

	log.Println("start emit events loop")
	for {
		<-time.After(60 * time.Second)
		cm.Timestamp = time.Now()
		//events := []types.Event{types.NewMessageEvent(cm)}
		log.Println("about to emit events")
		//err := eh.EmitEvents(events)
		//if err != nil {
		//	log.Printf("error: %s", err)
		//}
	}

	return nil
}

/*
func (m *EventHandler) Cron(bc plugins.BroadcastHelper) error {
	msg, err := types.NewChatMessage(translatorNick, time.Now(), translatorText, translatorTextLanguage)
	if err != nil {
		return err
	}
	err = m.HandleMessage(*msg, bc)
	if err != nil {
		return err
	}
	return bc.Broadcast([]types.ChatMessage{*msg}, nil, false, nil)
}
*/

/*
func (m *EventHandler) EventStream(outEvents chan<- types.Event) (<-chan types.Event, error) {
	inEvents := make(<-chan types.Event)
	go func() {
		log.Println("start event stream loop")
		for {
			event, ok := <-inEvents
			if !ok {
				return
			}
			// do stuff with the event
			log.Printf("received event: %+v", event)

			respEvent := &types.EventTranslation{
				EventBase:          &types.EventBase{EventType: types.EventTypeTranslation},
				TranslationMessage: &types.TranslationMessage{
					SourceId:  "TEST",
					Timestamp: time.Now(),
					Language:  "de",
					Message:   "TEST",
					Filter:    "",
				},
			}
			outEvents <- respEvent
		}
	}()
	return inEvents, nil
}
*/

/*
func (m *EventHandler) InitEventStream(eh plugins.EventStreamHelper) error {
	eventsToMain := make(chan<- types.Event)
	eventsFromMain, err := eh.EventStream(eventsToMain)
	if err != nil {
		return err
	}
	go func() {
		for {
			event, ok := <-eventsFromMain
			if !ok {
				return
			}
			log.Printf("got event from main: %+v", event)
		}
	}()
	go func() {
		defer close(eventsToMain)
		for {
			<-time.After(10 * time.Second)
			event := &types.EventMessage{
				EventBase:   &types.EventBase{EventType: types.EventTypeMessage},
				ChatMessage: &types.ChatMessage{
					Id:        "TEST",
					Nick:      "NICK",
					Timestamp: time.Now(),
					Message:   "MSG",
					Language:  "",
					Filter:    "",
				},
			}
			log.Printf("about to send event to main: %v", event)
			eventsToMain <- event
			log.Printf("done sending event to main")
		}
	}()
	return nil
}
*/

func translation(srcText []string, language string) ([]string, error) {
	log.Printf("in translation srcText: %+v language: %s", srcText, language)
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
			log.Printf("found translation in cache!")
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
	log.Println("translation client created")
	if err != nil {
		log.Printf("error: could not create translation client: %v", err)
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
		log.Printf("error: could not translate: %v", err)
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
	log.Printf("translated: %v", translations)
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
