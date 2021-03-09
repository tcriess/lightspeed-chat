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
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/persistence"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
)

const (
	maxMessageSize          = 4096
	pongWait                = 2 * time.Minute
	pingPeriod              = time.Minute
	writeWait               = 10 * time.Second
	defaultEventHistorySize = 100
	broadcastChannelSize    = 1000
	historyChannelSize      = 1000
)

type Hub struct {
	// there is one hub per room
	*types.Room

	// Registered clients.
	clients map[*Client]struct{}

	// Broadcast events to all clients.
	BroadcastEvents chan []*types.Event

	// Register a new client to the hub.
	Register chan *Client

	// Unregister a client from the hub.
	Unregister chan *Client

	// keep the chat history in a ring buffer
	EventHistory                       chan []*types.Event
	eventHistoryStart, eventHistoryEnd *ring.Ring
	lockEventHistory                   sync.RWMutex

	// global configuration
	Cfg *config.Config

	// persistence
	Persister persistence.Persister

	// global plugins map
	pluginMap map[string]plugins.PluginSpec

	// mutex for manipulating the clients
	sync.RWMutex
}

func NewHub(room *types.Room, cfg *config.Config, persister persistence.Persister, pluginMap map[string]plugins.PluginSpec) *Hub {
	eventHistorySize := defaultEventHistorySize
	if cfg.HistoryConfig != nil {
		if cfg.HistoryConfig.HistorySize > 0 {
			eventHistorySize = cfg.HistoryConfig.HistorySize
		}
	}
	eventHistory := ring.New(eventHistorySize)
	hub := &Hub{
		Room:              room,
		clients:           make(map[*Client]struct{}),
		BroadcastEvents:   make(chan []*types.Event, broadcastChannelSize),
		Register:          make(chan *Client),
		Unregister:        make(chan *Client),
		EventHistory:      make(chan []*types.Event, historyChannelSize),
		eventHistoryStart: eventHistory,
		eventHistoryEnd:   eventHistory,
		Cfg:               cfg,
		Persister:         persister,
		pluginMap:         pluginMap,
	}
	if persister != nil {
		var t time.Time
		n := time.Now().Add(time.Minute)
		events, err := persister.GetEventHistory(hub.Room, t, n, 0, eventHistorySize)
		if err != nil {
			globals.AppLogger.Error("could not load persisted events", "error", err)
		}
		globals.AppLogger.Debug("loaded events", "events", events)
		hub.lockEventHistory.Lock()
		for _, event := range events {
			hub.eventHistoryEnd.Value = event
			hub.eventHistoryEnd = hub.eventHistoryEnd.Next()
			if hub.eventHistoryEnd == hub.eventHistoryStart {
				hub.eventHistoryStart = hub.eventHistoryStart.Next()
			}
		}
		hub.lockEventHistory.Unlock()
	}
	for pluginName, plg := range pluginMap {
		eh := emitEventsHelper{
			hub:        hub,
			pluginName: pluginName,
		}
		go func(eeh emitEventsHelper, plg plugins.PluginSpec) {
			for {
				err := plg.Plugin.InitEmitEvents(hub.Room, &eeh) // never exits
				if err != nil {
					log.Printf("error: could not init emit events for plugin %s", pluginName)
					<-time.After(time.Second)
				}
			}
		}(eh, plg)
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

func (h *Hub) handlePlugins(events []*types.Event, skipPlugins map[string]struct{}) error {
	log.Printf("in handlePlugins, skipPlugins=%+v", skipPlugins)

	for pluginName, plg := range h.pluginMap {
		if _, ok := skipPlugins[pluginName]; ok {
			continue
		}
		log.Printf("calling plugin: %s", pluginName)
		passEvents := make([]*types.Event, 0)
		for _, event := range events {
			if plg.EventFilter != "" {
				if h.EvaluatePluginFilterEvent(event, plg.EventFilter) {
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
		newSkipPlugins := make(map[string]struct{})
		for key, val := range skipPlugins {
			newSkipPlugins[key] = val
		}
		newSkipPlugins[pluginName] = struct{}{}
		err = h.handlePlugins(resEvents, newSkipPlugins)
		if err != nil {
			log.Printf("error: could not handle plugins: %s", err)
			continue
		}
		globals.AppLogger.Info("plugin handled", "plugin", pluginName, "resEvents", resEvents)
		err = h.handleEvents(resEvents)
		if err != nil {
			log.Printf("error: could not handle events: %s", err)
			continue
		}
	}
	return nil
}

func (h *Hub) handleEvents(events []*types.Event) error {
	globals.AppLogger.Debug("in main handle Events", "events", events)
	if len(events) > 0 {
		h.BroadcastEvents <- events
		h.EventHistory <- events
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
					events, err := plg.Plugin.Cron(h.Room)
					if err != nil {
						log.Printf("error calling cron: %s", err)
						return
					}
					skipPlugins := make(map[string]struct{})
					skipPlugins[pluginName] = struct{}{}
					err = h.handlePlugins(events, skipPlugins)
					if err != nil {
						log.Printf("error handling plugins: %s", err)
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
			client.Done()
			go h.SendInfo(h.GetInfo())

		case client := <-h.Unregister:
			go func() {
				h.RLock()
				if _, ok := h.clients[client]; ok {
					h.RUnlock()
					log.Println("info: unregister client")

					h.Lock()
					delete(h.clients, client)
					h.Unlock()
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
					close(client.SendEvents)
					log.Println("close plugin channel")
					close(client.PluginChan)
					log.Println("broadcast new room info")
					go h.SendInfo(h.GetInfo()) // this way the number of clients does not change between calling the goroutine and executing it
				} else {
					h.RUnlock()
				}
			}()

		case events := <-h.BroadcastEvents:
			for i, event := range events {
				if event.Name == types.EventTypeInternal {
					continue
				}
				var prog *vm.Program
				if event.TargetFilter != "" {
					var err error
					prog, err = expr.Compile(event.TargetFilter, expr.Env(filter.Env{}))
					if err != nil {
						log.Printf("error: could not compile filter: %s", err)
					}
				}
				globals.AppLogger.Debug("checking event", "event", event)
				go func(evt *types.Event, prg *vm.Program) {
					var wg sync.WaitGroup
					h.RLock()
					for client := range h.clients {
						if !client.RunFilterEvent(evt, prg) {
							globals.AppLogger.Debug("filter prevented event!", "client", client, "client.Language", client.Language)
							continue
						}
						globals.AppLogger.Debug("event passed filter!", "client", client, "client.Language", client.Language)
						if data, err := json.Marshal(types.WireEvent{Event: evt}); err == nil {
							wg.Add(1)
							client.Add(1)
							go func(c *Client, d []byte) {
								defer wg.Done()
								defer c.Done()
								globals.AppLogger.Debug("about to send", "data", string(d))
								c.Send <- d
							}(client, data)
						}
					}
					log.Println("info: wait for broadcast to finish")
					wg.Wait()
					h.RUnlock()
				}(events[i], prog)
			}

		case events := <-h.EventHistory:
			h.lockEventHistory.Lock()
			for _, event := range events {
				h.eventHistoryEnd.Value = event
				h.eventHistoryEnd = h.eventHistoryEnd.Next()
				if h.eventHistoryEnd == h.eventHistoryStart {
					h.eventHistoryStart = h.eventHistoryStart.Next()
				}
			}
			h.lockEventHistory.Unlock()

			if h.Persister != nil {
				err := h.Persister.StoreEvents(h.Room, events)
				if err != nil {
					globals.AppLogger.Error("could not persist events", "error", err)
				}
			}
		}
	}
}

/*

 */

func (h *Hub) GetHistory() []*types.Event {
	history := make([]*types.Event, 0)
	h.lockEventHistory.RLock()
	defer h.lockEventHistory.RUnlock()
	current := h.eventHistoryStart
	for ; current != h.eventHistoryEnd; current = current.Next() {
		history = append(history, current.Value.(*types.Event))
	}
	return history
}

func (h *Hub) GetInfo() *types.Event {
	log.Println("info: in GetInfo")
	tags := make(map[string]string)
	h.RLock()
	for c := range h.clients {
		tags[c.user.Id] = c.user.Nick
	}
	h.RUnlock()
	source := &types.Source{
		PluginName: "main",
	}
	return types.NewEvent(h.Room, source, "", "", types.EventTypeInfo, tags)
}

// SendInfo broadcasts hub statistics to all clients.
func (h *Hub) SendInfo(event *types.Event) {
	h.BroadcastEvents <- []*types.Event{event}
}
