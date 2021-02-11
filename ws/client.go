package ws

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mitchellh/mapstructure"
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

	Language string

	user *types.User

	pluginChan chan types.Event
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
		user:       user,
		Language:   lang,
		doneChan:   doneChan,
		pluginChan: make(chan types.Event, pluginChannelSize),
	}
}

func (c *Client) SendChatHistory(chatHistory []types.ChatMessage, wg *sync.WaitGroup) {
	log.Println("info: in SendChatHistory")
	if wg != nil {
		defer wg.Done()
	}
	for _, chatMsg := range chatHistory {
		if !c.EvaluateFilterMessage(&chatMsg) {
			continue
		}
		messageBytes, err := json.Marshal(types.WireChatMessage{
			ChatMessage: &chatMsg,
		})
		if err != nil {
			log.Printf("could not marshal message: %s", err)
			continue
		}
		c.hub.RLock()
		if _, ok := c.hub.clients[c]; ok {
			c.Send <- messageBytes
		}
		c.hub.RUnlock()
	}
}

func (c *Client) SendTranslationHistory(translationHistory []types.TranslationMessage, wg *sync.WaitGroup) {
	log.Println("info: in SendTranslationHistory")
	if wg != nil {
		defer wg.Done()
	}
	for _, translationMsg := range translationHistory {
		if !c.EvaluateFilterTranslation(&translationMsg) {
			continue
		}
		messageBytes, err := json.Marshal(types.WireTranslationMessage{TranslationMessage: &translationMsg})
		if err != nil {
			log.Printf("could not marshal message: %s", err)
			continue
		}
		c.hub.RLock()
		if _, ok := c.hub.clients[c]; ok {
			c.Send <- messageBytes
		}
		c.hub.RUnlock()
	}
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
		/*
			case MessageTypeLogin:
				respMsg := LoginMessage{Nick: c.username}
				loginMsgMap := make(map[string]interface{})
				err = json.Unmarshal(message.Data, &loginMsgMap)
				if err != nil {
					log.Printf("error: could not unmarshal login message: %s", err)
					respMsg.ErrorMessage = err.Error()
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
					return
				}
				loginMsg := LoginMessage{}
				err = mapstructure.WeakDecode(loginMsgMap, &loginMsg)
				if err != nil {
					log.Printf("error: could not decode chat message: %s", err)
					respMsg.ErrorMessage = err.Error()
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
					return
				}

				if loginMsg.IdToken == "" || c.hub.Cfg.OIDCConfig.ProviderUrl == "" {
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
					continue
				}

				provider, err := oidc.NewProvider(context.Background(), c.hub.Cfg.OIDCConfig.ProviderUrl)
				if err != nil {
					respMsg.ErrorMessage = err.Error()
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
				}
				config := oidc.Config{}
				if c.hub.Cfg.OIDCConfig.ClientId == "" {
					config.SkipClientIDCheck = true
				} else {
					config.ClientID = c.hub.Cfg.OIDCConfig.ClientId
				}
				verifier := provider.Verifier(&config)
				//keySet := oidc.NewRemoteKeySet(context.Background(), "https://www.googleapis.com/oauth2/v3/certs")
				//verifier := oidc.NewVerifier("https://accounts.google.com", keySet, &config)
				idToken, err := verifier.Verify(context.Background(), loginMsg.IdToken)
				if err != nil {
					log.Printf("error: could not verify token: %s", err)
					respMsg.ErrorMessage = err.Error()
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
					continue
				}
				log.Printf("idToken: %+v (verified)", *idToken)
				claims := struct {
					Email string `json:"email"`
				}{}
				err = idToken.Claims(&claims)
				if err != nil {
					log.Printf("error: could not parse claims: %s", err)
					respMsg.ErrorMessage = err.Error()
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
					continue
				}
				log.Printf("claims: %v", claims)
				if claims.Email != "" {
					c.username = claims.Email
					respMsg.Nick = c.username
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
				} else {
					respMsg.ErrorMessage = "empty e-mail address"
					resp, err := json.Marshal(respMsg)
					if err != nil {
						log.Printf("error: could not marshal login response")
						return
					}
					c.Send <- resp
				}
		*/
		case types.MessageTypeChat:
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
			err = chatMsg.CreateId()
			if err != nil {
				log.Printf("error: could not hash chat message: %s", err)
				return
			}
			if !strings.HasPrefix(chatMsg.Message, "/") {
				c.hub.ChatHistory <- &chatMsg
				c.hub.BroadcastChat <- &chatMsg
				c.pluginChan <- types.NewMessageEvent(chatMsg)
			} else {
				raw, err := json.Marshal(types.WireChatMessage{ChatMessage: &chatMsg})
				if err != nil {
					log.Printf("error: could not marshal chat message: %s", err)
					return
				}
				c.hub.RLock()
				if _, ok := c.hub.clients[c]; ok {
					c.Send <- raw
				}
				c.hub.RUnlock()
				c.pluginChan <- types.NewCommandEvent(types.Command{
					Command: chatMsg.Message,
					Nick:    chatMsg.Nick,
				})
			}
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
			_, _ = w.Write(message)

			//// Add queued messages to the current websocket message.
			//n := len(c.Send)
			//for i := 0; i < n; i++ {
			//	//_, _ = w.Write([]byte{'\n'})
			//	message = <-c.Send
			//	_, _ = w.Write(message)
			//}

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

// A per-client plugin loop. Reads the pluginChan and calls per-client plugins. Will be exited when the pluginChan is closed.
func (c *Client) PluginLoop() {
	for {
		select {
		case <-c.doneChan:
			log.Println("info: doneChan closed, exiting plugin loop")
			return

		default:
		}
		select {
		case event, ok := <-c.pluginChan:
			if !ok {
				log.Println("info: pluginChan closed, exiting client plugin loop")
				return
			}
			err := c.hub.handlePlugins([]types.Event{event})
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
