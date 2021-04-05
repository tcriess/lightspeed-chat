package main

import (
	"database/sql"
	"fmt"
	"gorm.io/gorm"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/folkengine/goname"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
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
	log.SetFlags(0)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		plugin.CleanupClients()
		log.Fatal("interrupted!")
	}()

	flagSet := config.GetFlagSet()
	pflag.CommandLine.AddFlagSet(flagSet)

	pflag.Parse()

	globalConfig, err := config.ReadConfiguration(*configPath, flagSet)
	if err != nil {
		panic(err)
	}

	globals.AppLogger.SetLevel(hclog.LevelFromString(globalConfig.LogLevel))

	persister, err := persistence.NewPersister(globalConfig)
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
		for _, pluginCfg := range globalConfig.PluginConfigs {
			if pluginCfg.Name == pluginName {
				globals.AppLogger.Debug("found config", "config", pluginCfg.RawPluginConfig)
				cronSpec, eventFilter, err := eventHandler.Configure(pluginCfg.RawPluginConfig)
				if err != nil {
					panic(fmt.Sprintf("could not configure plugin %s: %s", pluginName, err))
				}
				pluginSpec.CronSpec = cronSpec
				pluginSpec.EventFilter = eventFilter
				break
			}
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
		for _, r := range rooms {
			globals.AppLogger.Debug("room", "room", *r, "id", r.Id)
		}
		globals.AppLogger.Debug("all rooms", "rooms", rooms)
		if len(rooms) == 0 {
			adminUser := types.User{Id: globalConfig.AdminUser}
			err := persister.GetUser(&adminUser)
			if err == gorm.ErrRecordNotFound || err == sql.ErrNoRows || err == buntdb.ErrNotFound {
				adminUser.Tags = make(map[string]string)
				adminUser.Language = "en"
				adminUser.Nick = globalConfig.AdminUser
				err := persister.StoreUser(adminUser)
				if err != nil {
					panic(err)
				}
			} else {
				if err != nil {
					panic(err)
				}
			}
			// no room in the db, create a default room
			tags := make(map[string]string)
			tags["_allow_guests"] = "true"
			room := &types.Room{
				Id:    "default",
				Owner: &adminUser,
				Tags:  tags,
			}
			err = persister.StoreRoom(*room)
			if err != nil {
				panic(err)
			}
			rooms = []*types.Room{room}
		}
	} else {
		tags := make(map[string]string)
		tags["_allow_guests"] = "true"
		room := &types.Room{
			Id:    "default",
			Owner: &types.User{Id: globalConfig.AdminUser, Nick: globalConfig.AdminUser, Language: "en", Tags: make(map[string]string)},
			Tags:  tags,
		}
		rooms = []*types.Room{room}
	}

	for _, room := range rooms {
		globals.AppLogger.Debug("creating room", "id", room.Id, "room", *room)
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
		globals.AppLogger.Debug("room not found")
		hubsLock.RUnlock()
		w.WriteHeader(http.StatusBadRequest)
		return
	} else {
		globals.AppLogger.Debug("room found")
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
			var err error
			globals.AppLogger.Debug("found oidc provider", "provider", provider)
			userId, err = auth.Authenticate(idToken, provider, hub.Cfg)
			if err != nil {
				globals.AppLogger.Error("could not authenticate", "error", err)
			}
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
	defer conn.Close()

	doneChan := make(chan struct{})

	nick := userId
	if nick == "" {
		nick = goname.New(goname.FantasyMap).FirstLast() + " (guest)"
		if ag, ok := hub.Room.Tags["_allow_guests"]; ok {
			if allowGuests, err := strconv.ParseBool(ag); err == nil && allowGuests {
				userId = nick
			}
		}
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
		if err == gorm.ErrRecordNotFound || err == buntdb.ErrNotFound || err == sql.ErrNoRows {
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

	wg := &sync.WaitGroup{}
	if user.Id != "" {
		wg.Add(2)
		source := &types.Source{
			User:       &user,
			PluginName: "main",
		}

		tags := map[string]string{
			"action": "login",
		}
		userEvent := types.NewEvent(hub.Room, source, "", "", types.EventTypeUser, tags)
		go func(evt *types.Event, wg *sync.WaitGroup) {
			defer wg.Done()
			hub.BroadcastEvents <- []*types.Event{evt}
		}(userEvent, wg)
		go func(evt *types.Event, wg *sync.WaitGroup) {
			defer wg.Done()
			c.PluginChan <- []*types.Event{evt}
		}(userEvent, wg)
	}
	wg.Add(1)
	go c.SendHistory(hub.GetHistory(), wg)
	// make sure those 3 are done before closing the send channel
	globals.AppLogger.Debug("wait for client wg chan")
	wg.Wait()
	globals.AppLogger.Debug("done waiting for client wg chan, waiting for doneChan")
	<-doneChan
	globals.AppLogger.Info("doneChan closed, exiting ws handler")
}
