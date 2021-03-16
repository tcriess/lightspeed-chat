package ws

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/types"
)

const (
	sendChannelSize   = 1000
	pluginChannelSize = 1000
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	Send chan []byte

	SendEvents chan []*types.Event

	Language string

	user *types.User

	PluginChan chan []*types.Event
	doneChan   chan struct{}

	// WaitGroup which keeps track of running read/write loops and write access to Send. If the WaitGroup is done,
	// it is safe to close all channels (all loops are done and there are no more write operations on the channels)
	sync.WaitGroup
}

func NewClient(hub *Hub, conn *websocket.Conn, user *types.User, language string, doneChan chan struct{}) *Client {
	lang := language
	if len(lang) > 2 {
		lang = lang[0:2]
	}
	if len(lang) < 2 {
		lang = "en"
	}
	return &Client{
		hub:        hub,
		conn:       conn,
		Send:       make(chan []byte, sendChannelSize),
		SendEvents: make(chan []*types.Event, sendChannelSize),
		user:       user,
		Language:   lang,
		doneChan:   doneChan,
		PluginChan: make(chan []*types.Event, pluginChannelSize),
	}
}

func (c *Client) SendHistory(events []*types.Event, wg *sync.WaitGroup) {
	log.Println("info: in SendHistory")
	if wg != nil {
		defer wg.Done()
	}
	c.hub.RLock()
	if _, ok := c.hub.clients[c]; ok {
		c.SendEvents <- events
	}
	c.hub.RUnlock()
}

// ReadLoop pumps messages from the websocket connection to the hub.
//
// The application runs ReadLoop in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) ReadLoop() {
	log.Println("info: in ReadLoop")
	defer func() {
		c.conn.Close()
		close(c.doneChan)
		c.Done()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	message := &types.WebsocketMessage{}
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("could not read message: %s", err)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println("ws closed unexpected")
			}
			return
		}

		err = json.Unmarshal(raw, &message)
		if err != nil {
			log.Printf("could not unmarshal ws message: %s", err)
			return
		}

		switch message.Event {
		case types.WireMessageTypeChat:
			chatMsgMap := make(map[string]interface{})
			err = json.Unmarshal(message.Data, &chatMsgMap)
			if err != nil {
				log.Printf("error: could not unmarshal chat message: %s", err)
				return
			}
			chatMsg := types.ChatMessage{}
			err = mapstructure.WeakDecode(chatMsgMap, &chatMsg)
			if err != nil {
				log.Printf("error: could not decode chat message: %s", err)
				return
			}
			chatMsg.Timestamp = time.Now()
			chatMsg.Nick = c.user.Nick
			source := &types.Source{
				User: c.user,
			}
			tags := map[string]string{
				"message":   html.EscapeString(chatMsg.Message),
				"mime_type": "text/plain",
			}
			if !strings.HasPrefix(chatMsg.Message, "/") {
				event := types.NewEvent(c.hub.Room, source, chatMsg.Filter, chatMsg.Language, types.EventTypeChat, tags)
				events := []*types.Event{event}
				c.hub.EventHistory <- events
				c.hub.BroadcastEvents <- events
				c.hub.RLock()
				if _, ok := c.hub.clients[c]; ok {
					c.PluginChan <- events
				}
				c.hub.RUnlock()
			} else {
				// set the filter to send commands only to the original sender
				filter := ""
				if chatMsg.Filter != "" {
					filter = "(" + chatMsg.Filter + ") && " + fmt.Sprintf(`Target.User.Id == %s`, strconv.Quote(c.user.Id))
				} else {
					filter = fmt.Sprintf(`Target.User.Id == %s`, strconv.Quote(c.user.Id))
				}
				fields := strings.Fields(chatMsg.Message)
				args := ""
				if len(fields) > 1 {
					args = strings.Join(fields[1:], " ")
				}
				tags["command"] = fields[0]
				tags["args"] = args
				cmdEvent := types.NewEvent(c.hub.Room, source, filter, chatMsg.Language, types.EventTypeCommand, tags)
				events := []*types.Event{cmdEvent}
				c.hub.RLock()
				if _, ok := c.hub.clients[c]; ok {
					c.SendEvents <- events
					c.PluginChan <- events
				}
				c.hub.RUnlock()
			}

		default:
			// the client sends "something". We assume it is an event and add source and room information.
			msgMap := make(map[string]interface{})
			err = json.Unmarshal(message.Data, &msgMap)
			if err != nil {
				log.Printf("error: could not unmarshal message: %s", err)
				return
			}
			msg := struct {
				TargetFilter string            `mapstructure:"target_filter"`
				Language     string            `mapstructure:"language"`
				Tags         map[string]string `mapstructure:"tags"`
			}{}
			err = mapstructure.WeakDecode(msgMap, &msg)
			if err != nil {
				log.Printf("error: could not decode message: %s", err)
				return
			}
			source := &types.Source{
				User: &types.User{
					Id:         c.user.Id,
					Nick:       c.user.Nick,
					Language:   c.user.Language,
					Tags:       c.user.Tags,
					LastOnline: c.user.LastOnline,
				},
				PluginName: "",
			}
			event := types.NewEvent(c.hub.Room, source, msg.TargetFilter, msg.Language, message.Event, msg.Tags)
			events := []*types.Event{event}
			c.hub.EventHistory <- events
			c.hub.BroadcastEvents <- events
			c.hub.RLock()
			if _, ok := c.hub.clients[c]; ok {
				c.PluginChan <- events
			}
			c.hub.RUnlock()
		}
	}
}

// WriteLoop pumps messages from the hub to the websocket connection.
//
// A goroutine running WriteLoop is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) WriteLoop() {
	log.Println("info: in WriteLoop")
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		c.Done()
	}()
	for {
		select {
		case <-c.doneChan:
			log.Println("info: doneChan closed, exiting plugin loop")
			return
		default:
		}
		select {
		case events, ok := <-c.SendEvents:
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				log.Println("info: Send event channel closed, exiting write loop")
				return
			}

			eventsSlices := make(map[string][]json.RawMessage)
			for _, event := range events {
				if event.TargetFilter != "" {
					if !c.EvaluateFilterEvent(event) {
						continue
					}
				}
				w, err := json.Marshal(types.WireEvent{Event: event})
				if err != nil {
					globals.AppLogger.Error("could not marshal event", "error", err)
					continue
				}
				if _, ok := eventsSlices[event.Name]; !ok {
					eventsSlices[event.Name] = make([]json.RawMessage, 0, len(events))
				}
				eventsSlices[event.Name] = append(eventsSlices[event.Name], w)
			}
			for eventType, outEvents := range eventsSlices {
				outEventType := eventType + "s" // make plural...
				data, err := json.Marshal(outEvents)
				if err != nil {
					globals.AppLogger.Error("could not marshal events", "error", err)
					continue
				}
				msg := types.WebsocketMessage{
					Event: outEventType,
					Data:  data,
				}
				w, err := json.Marshal(msg)
				if err != nil {
					globals.AppLogger.Error("could not marshal events", "error", err)
					continue
				}
				c.Send <- w
			}

		case message, ok := <-c.Send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				log.Println("info: Send channel closed, exiting write loop")
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Println("info could not write to ws connection, exiting write loop")
				return
			}
			_, err = w.Write(message)
			if err != nil {
				log.Printf("error: could not send message: %s", err)
				w.Close()
				return
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("info: could not send ping message, exiting write loop")
				return
			}

		case <-c.doneChan:
			log.Println("info: doneChan closed, exiting write loop")
			return
		}
	}
}

// A per-client plugin loop. Reads the PluginChan and calls per-client plugins. Will be exited when the PluginChan is closed.
func (c *Client) PluginLoop() {
	for {
		select {
		case <-c.doneChan:
			log.Println("info: doneChan closed, exiting plugin loop")
			return

		default:
		}
		select {
		case events, ok := <-c.PluginChan:
			if !ok {
				log.Println("info: PluginChan closed, exiting client plugin loop")
				return
			}
			skipPlugins := make(map[string]struct{})
			err := c.hub.handlePlugins(events, skipPlugins)
			if err != nil {
				log.Printf("error: %s", err)
				continue
			}

		case <-c.doneChan:
			log.Println("info: doneChan closed, exiting plugin loop")
			return
		}
	}
}
