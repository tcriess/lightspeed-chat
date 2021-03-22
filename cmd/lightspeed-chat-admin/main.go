package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/persistence"
	"github.com/tcriess/lightspeed-chat/plugins"
	"github.com/tcriess/lightspeed-chat/types"
)

// A very simple CLI tool for the administration of lightspeed-chat rooms and users.

var (
	configPath          = pflag.StringP("config", "c", "", "path to config file or directory")
	eventHandlerPlugins = pflag.StringSliceP("plugin", "p", nil, "path(s) to event handler plugin(s)")

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
	if persister == nil {
		panic("no persistence configured")
	}
	defer persister.Close()

	eventHandlers := make([]plugins.EventHandler, 0)
	for _, mhp := range *eventHandlerPlugins {
		pluginClient := plugin.NewClient(&plugin.ClientConfig{
			HandshakeConfig:  plugins.Handshake,
			Plugins:          plugins.PluginMap,
			Cmd:              exec.Command("sh", "-c", mhp),
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

	var cmdShow = &cobra.Command{
		Use:   "show",
		Short: "Show room or user",
		Long:  `show is for printing user or room information with a given user/room id.`,
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Show: " + strings.Join(args, " "))
		},
	}
	var cmdShowRooms = &cobra.Command{
		Use:   "rooms",
		Short: "Show rooms",
		Long:  `show is for listing all available rooms.`,
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			rooms, err := persister.GetRooms()
			if err != nil {
				globals.AppLogger.Error("could not get rooms", "error", err)
				return
			}
			r, err := json.Marshal(rooms)
			if err != nil {
				globals.AppLogger.Error("could not marshal rooms", "error", err)
				return
			}
			fmt.Println(string(r))
		},
	}
	var cmdShowRoom = &cobra.Command{
		Use:   "room [room id]",
		Short: "Show room",
		Long:  `show room prints detail information about the room with the given id.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			room := types.Room{Id: args[0]}
			err := persister.GetRoom(&room)
			if err != nil {
				globals.AppLogger.Error("could not get room", "error", err)
				return
			}
			r, err := json.Marshal(room)
			if err != nil {
				globals.AppLogger.Error("could not marshal room", "error", err)
				return
			}
			fmt.Println(string(r))
		},
	}
	var cmdShowUsers = &cobra.Command{
		Use:   "users",
		Short: "Show users",
		Long:  `shows a listing of all available users.`,
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			users, err := persister.GetUsers()
			if err != nil {
				globals.AppLogger.Error("could not get users", "error", err)
				return
			}
			u, err := json.Marshal(users)
			if err != nil {
				globals.AppLogger.Error("could not marshal users", "error", err)
				return
			}
			fmt.Println(string(u))
		},
	}
	var cmdShowUser = &cobra.Command{
		Use:   "user [user id]",
		Short: "Show user",
		Long:  `show user prints detail information about the user with the given id.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			user := types.User{Id: args[0]}
			err := persister.GetUser(&user)
			if err != nil {
				globals.AppLogger.Error("could not get user", "error", err)
				return
			}
			u, err := json.Marshal(user)
			if err != nil {
				globals.AppLogger.Error("could not marshal user", "error", err)
				return
			}
			fmt.Println(string(u))
		},
	}
	var cmdDelete = &cobra.Command{
		Use:   "delete",
		Short: "delete room or user",
		Long:  `delete removes the user or room with a given user/room id.`,
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Delete: " + strings.Join(args, " "))
		},
	}
	var cmdDeleteRoom = &cobra.Command{
		Use:   "room [room id]",
		Short: "Delete room",
		Long:  `delete room removes the room with the given id.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			room := types.Room{Id: pflag.Arg(2)}
			err := persister.DeleteRoom(&room)
			if err != nil {
				globals.AppLogger.Error("could not delete room", "error", err)
				return
			}
		},
	}
	var cmdDeleteUser = &cobra.Command{
		Use:   "user [user id]",
		Short: "Delete user",
		Long:  `delete user removes the user with the given id.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			user := types.User{Id: pflag.Arg(2)}
			err := persister.DeleteUser(&user)
			if err != nil {
				globals.AppLogger.Error("could not delete user", "error", err)
				return
			}
		},
	}
	var cmdSet = &cobra.Command{
		Use:   "set",
		Short: "create/update room or user",
		Long:  `set creates or updates a room or user.`,
		Args:  cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Set: " + strings.Join(args, " "))
		},
	}
	var cmdSetRoom = &cobra.Command{
		Use:   "room [room definition]",
		Short: "Set room",
		Long:  `set room creates or updates a room. If the room definition is "-", the definition is read from STDIN.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var r io.Reader
			if args[0] == "-" {
				r = os.Stdin
			} else {
				r = bytes.NewReader([]byte(args[0]))
			}
			dec := json.NewDecoder(r)
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
			oldRoom := types.Room{Id: room.Id}
			err = persister.GetRoom(&oldRoom)
			if err != nil {
				globals.AppLogger.Info("room does not exist, creating")
			}
			if room.Owner == nil {
				room.Owner = &types.User{}
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
		},
	}
	var cmdSetUser = &cobra.Command{
		Use:   "user [user definition]",
		Short: "Set user",
		Long:  `set user creates or updates a user with the given definition. If the user definition is "-", it is read from STDIN.`,
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			var r io.Reader
			if args[0] == "-" {
				r = os.Stdin
			} else {
				r = bytes.NewReader([]byte(args[0]))
			}
			dec := json.NewDecoder(r)
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
			err = persister.StoreUser(user)
			if err != nil {
				globals.AppLogger.Error("could not store user", "error", err)
				return
			}
		},
	}
	var rootCmd = &cobra.Command{Use: "lightspeed-chat-admin"}
	rootCmd.AddCommand(cmdShow)
	rootCmd.AddCommand(cmdDelete)
	rootCmd.AddCommand(cmdSet)
	cmdShow.AddCommand(cmdShowRooms, cmdShowRoom, cmdShowUsers, cmdShowUser)
	cmdDelete.AddCommand(cmdDeleteRoom, cmdDeleteUser)
	cmdSet.AddCommand(cmdSetRoom, cmdSetUser)
	rootCmd.Execute()
}
