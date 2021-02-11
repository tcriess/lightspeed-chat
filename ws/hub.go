package ws

import (
	"container/ring"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/antonmedv/expr"
	"github.com/antonmedv/expr/vm"
	"github.com/robfig/cron/v3"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/filter"
	"github.com/tcriess/lightspeed-chat/persistence"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
)

const (
	maxMessageSize                = 4096
	pongWait                      = 2 * time.Minute
	pingPeriod                    = time.Minute
	writeWait                     = 10 * time.Second
	defaultChatHistorySize        = 20
	defaultTranslationHistorySize = 100
	broadcastChannelSize          = 1000
	historyChannelSize            = 1000
)

type Hub struct {
	// there is one hub per room
	roomName string

	// Registered clients.
	clients map[*Client]struct{}

	// Broadcast messages to all clients.
	Broadcast chan []byte

	BroadcastChat        chan *types.ChatMessage
	BroadcastTranslation chan *types.TranslationMessage

	// Register a new client to the hub.
	Register chan *Client

	// Unregister a client from the hub.
	Unregister chan *Client

	// keep the chat history in a ring buffer
	ChatHistory                                    chan *types.ChatMessage
	chatHistoryStart, chatHistoryEnd               *ring.Ring
	TranslationHistory                             chan *types.TranslationMessage
	translationHistoryStart, translationHistoryEnd *ring.Ring

	// global configuration
	Cfg *config.Config

	// persistence
	Persister persistence.Persister

	// global plugins map
	pluginMap map[string]plugins.PluginSpec

	// mutex for manipulating the clients
	sync.RWMutex
}

func NewHub(roomName string, cfg *config.Config, persister persistence.Persister, pluginMap map[string]plugins.PluginSpec) *Hub {
	chatHistorySize := defaultChatHistorySize
	translationHistorySize := defaultTranslationHistorySize
	if cfg.HistoryConfig != nil {
		if cfg.HistoryConfig.HistorySize > 0 {
			chatHistorySize = cfg.HistoryConfig.HistorySize
		}
		if cfg.HistoryConfig.TranslationHistorySize > 0 {
			translationHistorySize = cfg.HistoryConfig.TranslationHistorySize
		}
	}
	chatHistory := ring.New(chatHistorySize)
	translationHistory := ring.New(translationHistorySize)
	hub := &Hub{
		roomName:                roomName,
		clients:                 make(map[*Client]struct{}),
		Broadcast:               make(chan []byte, broadcastChannelSize),
		BroadcastChat:           make(chan *types.ChatMessage, broadcastChannelSize),
		BroadcastTranslation:    make(chan *types.TranslationMessage, broadcastChannelSize),
		Register:                make(chan *Client),
		Unregister:              make(chan *Client),
		ChatHistory:             make(chan *types.ChatMessage, historyChannelSize),
		chatHistoryStart:        chatHistory,
		chatHistoryEnd:          chatHistory,
		TranslationHistory:      make(chan *types.TranslationMessage, historyChannelSize),
		translationHistoryStart: translationHistory,
		translationHistoryEnd:   translationHistory,
		Cfg:                     cfg,
		Persister:               persister,
		pluginMap:               pluginMap,
	}
	if persister != nil {
		var t time.Time
		n := time.Now().Add(time.Minute)
		chatMessages, err := persister.GetChatHistory(t, n, 0, chatHistorySize)
		if err != nil {
			log.Printf("error: could not load persisted chat messages: %s", err)
		}
		for _, cm := range chatMessages {
			hub.chatHistoryEnd.Value = cm
			hub.chatHistoryEnd = hub.chatHistoryEnd.Next()
			if hub.chatHistoryEnd == hub.chatHistoryStart {
				hub.chatHistoryStart = hub.chatHistoryStart.Next()
			}
		}
		translationMessages, err := persister.GetTranslationHistory(t, n, 0, translationHistorySize)
		if err != nil {
			log.Printf("error: could not load persisted translations: %s", err)
		}
		for _, tm := range translationMessages {
			hub.translationHistoryEnd.Value = tm
			hub.translationHistoryEnd = hub.translationHistoryEnd.Next()
			if hub.translationHistoryEnd == hub.translationHistoryStart {
				hub.translationHistoryStart = hub.translationHistoryStart.Next()
			}
		}
	}
	for pluginName, plg := range pluginMap {
		eh := emitEventsHelper{
			hub:        hub,
			pluginName: pluginName,
		}
		go func(eeh emitEventsHelper) {
			for {
				err := plg.Plugin.InitEmitEvents(&eeh) // never exits
				if err != nil {
					log.Printf("error: could not init emit events for plugin %s", pluginName)
					<-time.After(time.Second)
				}
			}
		}(eh)
	}
	return hub
}

// NoClients returns the number of clients registered
func (h *Hub) NoClients() int {
	log.Println("info: in NoClients")
	h.RLock()
	defer h.RUnlock()
	return len(h.clients)
}

func (h *Hub) handlePlugins(events []types.Event) error {
	for pluginName, plg := range h.pluginMap {
		log.Printf("calling plugin: %s", pluginName)
		passEvents := make([]types.Event, 0)
		for _, event := range events {
			if plg.EventFilter != "" {
				if h.EvaluateFilterEvent(event, plg.EventFilter) {
					passEvents = append(passEvents, event)
				}
			} else {
				passEvents = append(passEvents, event)
			}
		}
		if len(passEvents) == 0 {
			continue
		}
		resEvents, err := plg.Plugin.HandleEvents(passEvents)
		if err != nil {
			log.Printf("error: could not call plugin to handle message: %s", err)
			continue
		}
		err = h.handleEvents(resEvents)
		if err != nil {
			log.Printf("error: could not handle events: %s", err)
			continue
		}
	}
	return nil
}

func (h *Hub) handleEvents(events []types.Event) error {
	for _, event := range events {
		switch event.GetEventType() {
		case types.EventTypeMessage:
			msgEvent := event.(*types.EventMessage)
			h.BroadcastChat <- msgEvent.ChatMessage
			h.ChatHistory <- msgEvent.ChatMessage

		case types.EventTypeTranslation:
			transEvent := event.(*types.EventTranslation)
			h.BroadcastTranslation <- transEvent.TranslationMessage
			h.TranslationHistory <- transEvent.TranslationMessage

		case types.EventTypeCommand:
			// TODO

		case types.EventTypeUserLogin:
			// TODO
		}
	}
	return nil
}

// Run is the main hub event loop handling register, unregister and broadcast events.
func (h *Hub) Run() {
	cronRunner := cron.New(cron.WithLocation(time.UTC), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	for pluginName, plg := range h.pluginMap {
		if plg.CronSpec != "" && pluginName != "" {
			if plg, ok := h.pluginMap[pluginName]; ok {
				entryId, err := cronRunner.AddFunc(plg.CronSpec, func() {
					events, err := plg.Plugin.Cron()
					if err != nil {
						log.Printf("error calling cron: %s", err)
						return
					}
					err = h.handleEvents(events)
					if err != nil {
						log.Printf("error handling events: %s", err)
						return
					}
					//cronFunc(h, pluginName)
				})
				defer cronRunner.Remove(entryId)
				if err != nil {
					panic(err)
				}
			}
		}
	}
	defer cronRunner.Stop()
	cronRunner.Start()
	for {
		log.Println("info: start hub run loop")
		select {
		case client := <-h.Register:
			log.Println("info: register new client")
			h.Lock()
			h.clients[client] = struct{}{}
			h.Unlock()
			go h.SendInfo(h.GetInfo())

		case client := <-h.Unregister:
			go func() {
				h.RLock()
				if _, ok := h.clients[client]; ok {
					h.RUnlock()
					log.Println("info: unregister client")

					h.Lock()
					delete(h.clients, client)
					log.Println("close connection (probably already is closed, just to make sure)")
					client.conn.Close()
					log.Println("wait for all loops and write operations to be finished")
					client.Wait()
					log.Println("close send channel")
					// here we have two options, both have their drawbacks:
					// - have some locking mechanism in place to avoid writing to a closed channel, because
					//   the Send channel is used at several places in multiple goroutines (use hub.RLock!)
					// - leave the channel open and let the gc handle it (but then we need to make sure the goroutines
					//   writing to Send do not block forever, i.e. somehow drain the channel and hope that there is
					//   no new write operation between the draining and the stopping of the remaining goroutines
					// IMO there is no "best" solution, I opted for the locking, which simply seems cleaner,
					// but I'm open to suggestions (see f.e. https://go101.org/article/channel-closing.html)
					//
					// drain could look like this:
					// so we start one final goroutine here to drain the channel
					// go func() {
					// 	for {
					// 		_, ok := <-client.Send
					// 		if !ok {
					//			return
					//		}
					// 	}
					// }()
					// close the channel and hope there is no more write to it
					close(client.Send)
					log.Println("close plugin channel")
					close(client.pluginChan)
					h.Unlock()
					log.Println("broadcast new room info")
					go h.SendInfo(h.GetInfo()) // this way the number of clients does not change between calling the goroutine and executing it
				} else {
					h.RUnlock()
				}
			}()

		case message := <-h.Broadcast:
			log.Printf("info: broadcast %s to all clients", message)
			go func() {
				var wg sync.WaitGroup
				h.RLock()
				for client := range h.clients {
					wg.Add(1)
					client.Add(1)
					go func(c *Client) {
						defer wg.Done()
						defer c.Done()
						c.Send <- message
					}(client)
				}
				log.Println("info: wait for broadcast to finish")
				wg.Wait()
				h.RUnlock()
				log.Println("info: broadcast done.")
			}()
		case message := <-h.BroadcastChat:
			var prog *vm.Program
			if message.Filter != "" {
				var err error
				prog, err = expr.Compile(message.Filter, expr.Env(filter.Env{}))
				if err != nil {
					log.Printf("error: could not compile filter: %s", err)
				}
			}
			go func() {
				var wg sync.WaitGroup
				h.RLock()
				for client := range h.clients {
					if !client.RunFilterMessage(message, prog) {
						continue
					}
					if data, err := json.Marshal(types.WireChatMessage{ChatMessage: message}); err == nil {
						wg.Add(1)
						client.Add(1)
						go func(c *Client) {
							defer wg.Done()
							defer c.Done()
							c.Send <- data
						}(client)
					}
				}
				log.Println("info: wait for broadcast to finish")
				wg.Wait()
				h.RUnlock()
			}()

		case message := <-h.BroadcastTranslation:
			var prog *vm.Program
			if message.Filter != "" {
				var err error
				prog, err = expr.Compile(message.Filter, expr.Env(filter.Env{}))
				if err != nil {
					log.Printf("error: could not compile filter: %s", err)
				}
			}
			go func() {
				var wg sync.WaitGroup
				h.RLock()
				for client := range h.clients {
					if !client.RunFilterTranslation(message, prog) {
						continue
					}
					if data, err := json.Marshal(types.WireTranslationMessage{TranslationMessage: message}); err == nil {
						wg.Add(1)
						client.Add(1)
						go func(c *Client) {
							defer wg.Done()
							defer c.Done()
							c.Send <- data
						}(client)
					}
				}
				log.Println("info: wait for broadcast to finish")
				wg.Wait()
				h.RUnlock()
			}()

		case chatMessage := <-h.ChatHistory:
			log.Printf("info: attach message %s to history", chatMessage)
			h.chatHistoryEnd.Value = *chatMessage
			h.chatHistoryEnd = h.chatHistoryEnd.Next()
			if h.chatHistoryEnd == h.chatHistoryStart {
				h.chatHistoryStart = h.chatHistoryStart.Next()
			}
			if h.Persister != nil {
				err := h.Persister.StoreChatMessage(*chatMessage)
				if err != nil {
					log.Printf("error: could not persist chat message: %s", err)
				}
			}

		case translationMessage := <-h.TranslationHistory:
			h.translationHistoryEnd.Value = *translationMessage
			h.translationHistoryEnd = h.translationHistoryEnd.Next()
			if h.translationHistoryEnd == h.translationHistoryStart {
				h.translationHistoryStart = h.translationHistoryStart.Next()
			}
			if h.Persister != nil {
				err := h.Persister.StoreTranslationMessage(*translationMessage)
				if err != nil {
					log.Printf("error: could not persist translation message: %s", err)
				}
			}
		}
	}
}

func (h *Hub) GetInfo() types.InfoMessage {
	log.Println("info: in GetInfo")
	return types.InfoMessage{
		RoomName:      h.roomName,
		NoConnections: h.NoClients(),
	}
}

func (h *Hub) GetChatHistory() []types.ChatMessage {
	chatHistory := make([]types.ChatMessage, 0)
	current := h.chatHistoryStart
	for ; current != h.chatHistoryEnd; current = current.Next() {
		chatHistory = append(chatHistory, current.Value.(types.ChatMessage))
	}
	log.Printf("chatHistory %+v", chatHistory)
	return chatHistory
}

func (h *Hub) GetTranslationHistory() []types.TranslationMessage {
	log.Println("In GetTranslationHistory")
	translationHistory := make([]types.TranslationMessage, 0)
	current := h.translationHistoryStart
	for ; current != h.translationHistoryEnd; current = current.Next() {
		translationHistory = append(translationHistory, current.Value.(types.TranslationMessage))
	}
	log.Printf("translationHistory: %+v", translationHistory)
	return translationHistory
}

// SendInfo broadcasts hub statistics to all clients.
func (h *Hub) SendInfo(info types.InfoMessage) {
	msg, err := json.Marshal(types.WireInfoMessage{InfoMessage: &info})
	if err != nil {
		log.Printf("could not marshal ws info: %s", err)
		return
	}
	h.Broadcast <- msg
}
