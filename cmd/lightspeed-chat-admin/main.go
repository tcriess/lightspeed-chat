package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/spf13/pflag"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/persistence"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
)

var (
	configPath          = pflag.StringP("config", "c", "", "path to config file or directory")
	eventHandlerPlugins = pflag.StringSliceP("plugin", "p", nil, "path(s) to event handler plugin(s)")

	globalPlugins map[string]plugins.PluginSpec = make(map[string]plugins.PluginSpec)
)

var Usage = func() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", filepath.Base(os.Args[0]))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "  %s [-c CONFIG] [[-p PLUGIN1] [-p PLUGIN2]...] COMMAND SUBCOMMAND\n", filepath.Base(os.Args[0]))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands/subcommands:")
	fmt.Fprintln(os.Stderr, "  show room ID")
	fmt.Fprintln(os.Stderr, "       print the JSON room definition of the room ID")
	fmt.Fprintln(os.Stderr, "  show user ID")
	fmt.Fprintln(os.Stderr, "       print the JSON user definition of the user ID")
	fmt.Fprintln(os.Stderr, "  set room")
	fmt.Fprintln(os.Stderr, "       read a JSON room definition from STDIN and update/create the room accordingly")
	fmt.Fprintln(os.Stderr, "  set user")
	fmt.Fprintln(os.Stderr, "       read a JSON user definition from STDIN and update/create the user accordingly")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	pflag.PrintDefaults()
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		<-c
		plugin.CleanupClients()
		log.Fatal("interrupted!")
	}()

	pflag.Usage = Usage
	pflag.Parse()

	if pflag.NArg() < 2 {
		pflag.Usage()
		return
	}

	log.SetFlags(0)

	var globalConfig *config.Config

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
	if persister == nil {
		panic("no persistence configured")
	}
	defer persister.Close()

	eventHandlers := make([]plugins.EventHandler, 0)
	for _, mhp := range *eventHandlerPlugins {
		pluginClient := plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig: plugins.Handshake,
			Plugins:         plugins.PluginMap,
			Cmd:             exec.Command("sh", "-c", mhp),
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Managed:          true,
		})

		// Connect via RPC
		rpcClient, err := pluginClient.Client()
		if err != nil {
			globals.AppLogger.Error("could not create rpc client", "error", err)
			panic("could not create rpc client")
		}

		// Request the plugin
		raw, err := rpcClient.Dispense("eventhandler")
		if err != nil {
			globals.AppLogger.Error("could not dispense rpc client", "error", err)
			panic("could not dispense rpc client")
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
		if pluginName == "main" {
			globals.AppLogger.Warn(`"main" is not a valid plugin name, skipping`)
			continue
		}
		pluginSpec := plugins.PluginSpec{
			Name:   pluginName,
			Plugin: eventHandler,
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

	switch pflag.Arg(0) {
	case "show":
		if pflag.NArg() < 3 {
			pflag.Usage()
			return
		}
		switch pflag.Arg(1) {
		case "room":
			room := types.Room{Id: pflag.Arg(2)}
			err := persister.GetRoom(&room)
			if err != nil {
				globals.AppLogger.Error("could not get room", "error", err)
				return
			}
			globals.AppLogger.Info("room", "room", room)

		case "user":
			user := types.User{Id: pflag.Arg(2)}
			err := persister.GetUser(&user)
			if err != nil {
				globals.AppLogger.Error("could not get user", "error", err)
				return
			}
			globals.AppLogger.Info("user", "user", user)
		}

	case "delete":
		if pflag.NArg() < 3 {
			pflag.Usage()
			return
		}
		switch pflag.Arg(1) {
		case "room":
			room := types.Room{Id: pflag.Arg(2)}
			err := persister.DeleteRoom(&room)
			if err != nil {
				globals.AppLogger.Error("could not delete room", "error", err)
				return
			}

		case "user":
			user := types.User{Id: pflag.Arg(2)}
			err := persister.DeleteUser(&user)
			if err != nil {
				globals.AppLogger.Error("could not delete user", "error", err)
				return
			}
		}

	case "set":
		dec := json.NewDecoder(os.Stdin)
		switch pflag.Arg(1) {
		case "room":
			room := types.Room{}
			err := dec.Decode(&room)
			if err != nil {
				globals.AppLogger.Error("could not decode room", "error", err)
				return
			}
			globals.AppLogger.Info("got room", "room", room)
			if room.Id == "" {
				globals.AppLogger.Error("no room id")
				return
			}
			err = persister.GetRoom(&room)
			if err != nil {
				globals.AppLogger.Info("room does not exist, creating")
			}
			if room.Owner.Id == "" {
				globals.AppLogger.Warn("no owner set")
			}
			if room.Owner.Id != "" && room.Owner.Nick == "" {
				globals.AppLogger.Info("user id set, but no nick, try to fetch user from db")
				err = persister.GetUser(room.Owner)
				if err != nil {
					globals.AppLogger.Error("could not get owner", "error", err)
					return
				}
			}
			err = persister.StoreRoom(room)
			if err != nil {
				globals.AppLogger.Error("could not store room", "error", err)
				return
			}

		case "user":
			// expect a json representation of a types.User in stdin
			user := types.User{}
			err := dec.Decode(&user)
			if err != nil {
				globals.AppLogger.Error("could not decode user", "error", err)
				return
			}
			globals.AppLogger.Info("got user", "user", user)
			if user.Id == "" {
				globals.AppLogger.Error("no user id")
				return
			}
			err = persister.GetUser(&user)
			if err != nil {
				globals.AppLogger.Info("user does not exist, creating")
			}
			err = persister.StoreUser(user)
			if err != nil {
				globals.AppLogger.Error("could not store user", "error", err)
				return
			}

		}
	}
}
