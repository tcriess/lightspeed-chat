package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/folkengine/goname"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/spf13/pflag"
	"github.com/tcriess/lightspeed-chat/auth"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/persistence"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/tcriess/lightspeed-chat/ws"
	"github.com/tidwall/buntdb"
)

var (
	configPath          = pflag.StringP("config", "c", "", "path to config file or directory")
	eventHandlerPlugins = pflag.StringSliceP("plugin", "p", nil, "path(s) to event handler plugin(s)")
	addr                = pflag.String("addr", "localhost:8000", "ws service address (including port)")
	sslCert             = pflag.String("ssl-cert", "", "SSL cert for websocket (optional)")
	sslKey              = pflag.String("ssl-key", "", "SSL key for websocket (optional)")

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	hubs          map[string]*ws.Hub = make(map[string]*ws.Hub)
	hubsLock      sync.RWMutex
	globalPlugins map[string]plugins.PluginSpec = make(map[string]plugins.PluginSpec)
)

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		plugin.CleanupClients()
		log.Fatal("interrupted!")
	}()

	pflag.Parse()
	log.SetFlags(0)

	globalConfig, pluginConfigs, err := config.ReadConfiguration(*configPath)
	if err != nil {
		panic(err)
	}

	if globalConfig.LogLevel != nil {
		globals.AppLogger.SetLevel(hclog.LevelFromString(*globalConfig.LogLevel))
	}

	persister, err := persistence.NewBuntPersister(globalConfig)
	if err != nil {
		panic(err)
	}
	if persister != nil {
		defer persister.Close()
	}

	eventHandlers := make([]plugins.EventHandler, 0)
	for _, ehp := range *eventHandlerPlugins {
		pluginClient := plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  plugins.Handshake,
			Plugins:          plugins.PluginMap,
			Cmd:              exec.Command("sh", "-c", ehp),
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Managed:          true,
		})

		// Connect via RPC
		rpcClient, err := pluginClient.Client()
		if err != nil {
			fmt.Println("Error:", err.Error())
			os.Exit(1)
		}

		// Request the plugin
		raw, err := rpcClient.Dispense("eventhandler")
		if err != nil {
			fmt.Println("Error:", err.Error())
			os.Exit(1)
		}

		eventHandler := raw.(plugins.EventHandler)
		eventHandlers = append(eventHandlers, eventHandler)

		pluginName := filepath.Base(ehp)
		if strings.HasPrefix(pluginName, "lightspeed-chat-") {
			pluginName = pluginName[len("lightspeed-chat-"):]
		}
		if strings.HasSuffix(pluginName, "-plugin") {
			pluginName = pluginName[:len(pluginName)-len("-plugin")]
		}
		pluginName = strings.ToLower(pluginName)
		if pluginName == "main" {
			globals.AppLogger.Warn(`"main" is not a valid plugin name, skipping`)
			continue
		}
		pluginSpec := plugins.PluginSpec{
			Name:   pluginName,
			Plugin: eventHandler,
		}
		globals.AppLogger.Debug("pluginName", "pluginName", pluginName)
		if cfg, ok := pluginConfigs[pluginName]; ok {
			globals.AppLogger.Debug("found config", "config", cfg)
			spec, err := eventHandler.GetSpec()
			if err != nil {
				panic(fmt.Sprintf("could not get plugin config spec: %s", err))
			}
			globals.AppLogger.Debug("spec", "spec", spec)
			val, _ := hcldec.Decode(cfg, spec, nil)
			cronSpec, eventFilter, err := eventHandler.Configure(val)
			if err != nil {
				panic(fmt.Sprintf("could not configure plugin %s: %s", pluginName, err))
			}
			pluginSpec.CronSpec = cronSpec
			pluginSpec.EventFilter = eventFilter
		}
		globalPlugins[pluginName] = pluginSpec
	}
	defer plugin.CleanupClients()

	var rooms []*types.Room
	if persister != nil {
		var err error
		rooms, err = persister.GetRooms()
		if err != nil {
			panic(err)
		}
		if len(rooms) == 0 {
			// no room in the db, create a default room
			room := &types.Room{
				Id:    "default",
				Owner: &types.User{},
				Tags:  make(map[string]string),
			}
			err := persister.StoreRoom(*room)
			if err != nil {
				panic(err)
			}
			rooms = []*types.Room{room}
		}
	} else {
		room := &types.Room{
			Id:    "default",
			Owner: &types.User{},
			Tags:  make(map[string]string),
		}
		rooms = []*types.Room{room}
	}

	for _, room := range rooms {
		globals.AppLogger.Debug("creating room", "room", *room)
		hub := ws.NewHub(room, globalConfig, persister, globalPlugins)
		hubs[room.Id] = hub
		go hub.Run()
	}
	setupRoutes()
	// start HTTP server
	if *sslCert != "" && *sslKey != "" {
		err = http.ListenAndServeTLS(*addr, *sslCert, *sslKey, nil)
	} else {
		err = http.ListenAndServe(*addr, nil)
	}
	globals.AppLogger.Error("stopped listening", "error", err)
}

func setupRoutes() {
	router := mux.NewRouter()
	router.HandleFunc("/chat/{room:[a-z][a-z0-9_-]+}", websocketHandler).Methods(http.MethodGet)
	http.Handle("/", router)
}

// Handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	globals.AppLogger.Info("in websocketHandler")

	vars := mux.Vars(r)
	roomName := vars["room"]
	if roomName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	globals.AppLogger.Debug("looking for room", "room", roomName)
	var hub *ws.Hub
	hubsLock.RLock()
	if h, ok := hubs[roomName]; !ok {
		hubsLock.RUnlock()
		w.WriteHeader(http.StatusBadRequest)
		return
	} else {
		hubsLock.RUnlock()
		hub = h
	}
	globals.AppLogger.Debug("found room!")

	userId := ""
	vals := r.URL.Query()
	globals.AppLogger.Debug("checking id token")
	if idToken := vals.Get("id_token"); idToken != "" {
		globals.AppLogger.Debug("token", "idtoken", idToken)
		if provider := vals.Get("provider"); provider != "" {
			globals.AppLogger.Debug("found oidc provider", "provider", provider)
			userId, _ = auth.Authenticate(idToken, provider, hub.Cfg)
		}
	}
	language := vals.Get("language")

	// Upgrade HTTP request to Websocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		globals.AppLogger.Error("websocket upgrade error", "error", err)
		return
	}

	// When this frame returns close the Websocket
	defer conn.Close() //nolint

	doneChan := make(chan struct{})

	nick := userId
	if nick == "" {
		nick = goname.New(goname.FantasyMap).FirstLast() + " (guest)"
		userId = nick
	}
	user := types.User{
		Id:         userId,
		Nick:       nick,
		Language:   "",
		Tags:       make(map[string]string),
		LastOnline: time.Time{},
	}
	if userId != "" && hub.Persister != nil {
		err = hub.Persister.GetUser(&user)
		if err == buntdb.ErrNotFound {
			user.Language = "en"
			user.LastOnline = time.Now()
			err := hub.Persister.StoreUser(user)
			if err != nil {
				globals.AppLogger.Error("could not store user", "error", err)
				return
			}
		} else {
			if err != nil {
				globals.AppLogger.Error("could not get user", "error", err)
				return
			}
			nick = user.Nick
		}
	}
	for k := range user.Tags {
		if strings.HasPrefix(k, "_") { // remove internal tags
			delete(user.Tags, k)
		}
	}
	c := ws.NewClient(hub, conn, &user, language, doneChan)
	go c.PluginLoop()

	// Add to the hub
	c.Add(1)
	globals.AppLogger.Debug("about to register")
	hub.Register <- c
	globals.AppLogger.Debug("put client in register chan")
	// actually, it is not guaranteed that the client really _is_ registered at this point, as the read-out of the hub's
	// register channel happens asynchronously.
	// maybe we should wait here for the client to be actually registered, so the following broadcast calls
	// also reach the new client
	globals.AppLogger.Debug("waiting for client to actually register")
	c.Wait()
	globals.AppLogger.Debug("client registered")
	defer func() {
		hub.Unregister <- c
	}()
	c.Add(2)
	go c.ReadLoop()
	go c.WriteLoop()

	source := &types.Source{
		User:       &user,
		PluginName: "main",
	}

	tags := map[string]string{
		"action": "login",
	}
	userEvent := types.NewEvent(hub.Room, source, "", "", types.EventTypeUser, tags)

	wg := &sync.WaitGroup{}
	wg.Add(3)
	go func(evt *types.Event, wg *sync.WaitGroup) {
		defer wg.Done()
		hub.BroadcastEvents <- []*types.Event{evt}
	}(userEvent, wg)
	go func(evt *types.Event, wg *sync.WaitGroup) {
		defer wg.Done()
		c.PluginChan <- []*types.Event{evt}
	}(userEvent, wg)
	go c.SendHistory(hub.GetHistory(), wg)
	// make sure those 3 are done before closing the send channel
	globals.AppLogger.Debug("wait for client wg chan")
	wg.Wait()
	globals.AppLogger.Debug("done waiting for client wg chan, waiting for doneChan")
	<-doneChan
	globals.AppLogger.Info("doneChan closed, exiting ws handler")
}
