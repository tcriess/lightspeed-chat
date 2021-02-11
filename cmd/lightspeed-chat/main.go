package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/folkengine/goname"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/spf13/pflag"
	"github.com/tcriess/lightspeed-chat/auth"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/persistence"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/tcriess/lightspeed-chat/ws"
	"github.com/tidwall/buntdb"
)

var (
	configPath          = pflag.StringP("config", "c", "", "path to config file or directory")
	eventHandlerPlugins = pflag.StringSliceP("plugin", "p", nil, "path to event handler plugin")
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
	pflag.Parse()
	log.SetFlags(0)

	var globalConfig *config.Config

	globalConfig, pluginConfigs, err := config.ReadConfiguration(*configPath)
	if err != nil {
		panic(err)
	}

	persister, err := persistence.NewBuntPersister(globalConfig)
	if err != nil {
		panic(err)
	}
	if persister != nil {
		defer persister.Close()
	}

	eventHandlers := make([]plugins.EventHandler, 0)
	for _, mhp := range *eventHandlerPlugins {
		pluginClient := plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig: plugins.Handshake,
			Plugins:         plugins.PluginMap,
			Cmd:             exec.Command("sh", "-c", mhp),
			//AllowedProtocols: []plugin.Protocol{
			//	plugin.ProtocolNetRPC, plugin.ProtocolGRPC},
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
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

		pluginName := filepath.Base(mhp)
		if strings.HasPrefix(pluginName, "lightspeed-chat-") {
			pluginName = pluginName[len("lightspeed-chat-"):]
		}
		if strings.HasSuffix(pluginName, "-plugin") {
			pluginName = pluginName[:len(pluginName)-len("-plugin")]
		}
		pluginSpec := plugins.PluginSpec{
			Name:        pluginName,
			Plugin:      eventHandler,
		}
		log.Printf("pluginName: %s", pluginName)
		if cfg, ok := pluginConfigs[pluginName]; ok {
			log.Printf("found config: %+v", cfg)
			spec, err := eventHandler.GetSpec()
			if err != nil {
				panic(fmt.Sprintf("could not get plugin config spec: %s", err))
			}
			log.Printf("spec: %+v", spec)
			val, diag := hcldec.Decode(cfg, spec, nil)
			log.Printf("val: %+v diag: %+v", val, diag)
			log.Println(val.GoString())
			cronSpec, eventFilter, err := eventHandler.Configure(val)
			if err != nil {
				panic(fmt.Sprintf("could not configure plugin %s: %s", pluginName, err))
			}
			log.Printf("got cronspec from plugin: %s", cronSpec)
			log.Printf("got eventFilter from plugin: %s", eventFilter)
			pluginSpec.CronSpec = cronSpec
			pluginSpec.EventFilter = eventFilter
		}
		globalPlugins[pluginName] = pluginSpec
	}
	defer plugin.CleanupClients()

	hub := ws.NewHub("default", globalConfig, persister, globalPlugins)
	hubs["default"] = hub
	go hub.Run()
	setupRoutes()
	// start HTTP server
	if *sslCert != "" && *sslKey != "" {
		log.Fatal(http.ListenAndServeTLS(*addr, *sslCert, *sslKey, nil))
	} else {
		log.Fatal(http.ListenAndServe(*addr, nil))
	}
}

func setupRoutes() {
	router := mux.NewRouter()
	router.HandleFunc("/chat/{room:[a-z0-9_-]+}", websocketHandler).Methods(http.MethodGet)
	http.Handle("/", router)
}

// Handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("info: in websocketHandler")

	vars := mux.Vars(r)
	roomName := vars["room"]
	if roomName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
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

	userId := ""
	vals := r.URL.Query()
	if idToken := vals.Get("id_token"); idToken != "" {
		if provider := vals.Get("provider"); provider != "" {
			userId, _ = auth.Authenticate(idToken, provider, hub.Cfg)
		}
	}
	language := vals.Get("language")

	// Upgrade HTTP request to Websocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	// When this frame returns close the Websocket
	defer conn.Close() //nolint

	doneChan := make(chan struct{})

	nick := userId
	if nick == "" {
		nick = goname.New(goname.FantasyMap).FirstLast() + " (guest)"
	}
	user := types.User{
		Id:         userId,
		Nick:       nick,
		IdToken:    "",
		Language:   "",
		Tags:       make(map[string]string),
		IntTags:    make(map[string]int64),
		LastOnline: time.Time{},
	}
	if userId != "" {
		err = hub.Persister.GetUser(&user)
		if err == buntdb.ErrNotFound {
			user.Language = "en"
			user.LastOnline = time.Now()
			err := hub.Persister.StoreUser(user)
			if err != nil {
				log.Printf("error: could not store user: %s", err)
				return
			}
		} else {
			if err != nil {
				log.Printf("error: could not get user: %s", err)
				return
			}
			nick = user.Nick
		}
	}
	c := ws.NewClient(hub, conn, &user, language, doneChan)

	go c.PluginLoop()

	c.Add(2)
	go c.ReadLoop()
	go c.WriteLoop()

	// Add to the hub
	hub.Register <- c
	defer func() {
		hub.Unregister <- c
	}()

	msg := types.LoginMessage{
		Nick: user.Nick,
	}
	loginMsg, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	wsMsg := types.WebsocketMessage{
		Event: types.MessageTypeLogin,
		Data:  loginMsg,
	}
	wire, err := json.Marshal(wsMsg)
	if err != nil {
		panic(err)
	}
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		c.Send <- wire
	}(wg)
	go c.SendChatHistory(hub.GetChatHistory(), wg)
	go c.SendTranslationHistory(hub.GetTranslationHistory(), wg)
	// make sure those 3 are done before closing the send channel
	wg.Wait()
	<-doneChan
	log.Println("info: doneChan closed, exiting ws handler")
}
