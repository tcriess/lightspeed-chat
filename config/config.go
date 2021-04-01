package config

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/tcriess/lightspeed-chat/globals"
)

const (
	defaultAdminUser = "admin"
)

// Config is the global configuration object which is filled via the configuration file
// (TODO: possibly command-line options in the future, consider using viper)
type Config struct {
	HistoryConfig     HistoryConfig     `mapstructure:"history"`
	OIDCConfigs       []OIDCConfig      `mapstructure:"oidc"`
	PersistenceConfig PersistenceConfig `mapstructure:"persistence"`
	PluginConfigs     []PluginConfig    `mapstructure:"plugin"`
	LogLevel          string            `mapstructure:"log_level"`
	AdminUser         string            `mapstructure:"admin_user"`
}

// HistoryConfig configures the size of the immediate event history that is kept in memory in a ring buffer and
// sent to newly connected clients
type HistoryConfig struct {
	HistorySize int `mapstructure:"history_size"`
}

// An OIDCConfig  object configures an OpenID Connect provider that is used to authenticate users. Users provide
// an ID token and the name of the provider, the authentication is then performed via verification of the token.
type OIDCConfig struct {
	Name        string `mapstructure:"name"`
	ClientId    string `mapstructure:"client_id"`
	ProviderUrl string `mapstructure:"provider_url"` // f.e. "https://accounts.google.com", this is used to construct the discovery url and subsequently discover the openid endpoints
}

// BundDBConfig configures the BuntDB file storage backed database.
type BuntDBConfig struct {
	GlobalName       string `mapstructure:"global_name"`
	RoomNameTemplate string `mapstructure:"room_name_template"`
}

type SQLiteConfig struct {
	DSN string `mapstructure:"dsn"`
}

type PostgresConfig struct {
	DSN string `mapstructure:"dsn"`
}

// PersistenceConfig configures the persistence backends. Currently only BuntDB via BuntDBConfig and SQLite via
// SQLiteConfig are supported. If more than one persister is defined, sqlite > buntdb.
type PersistenceConfig struct {
	Type string `mapstructure:"type"`
	DSN  string `mapstructure:"dsn"`

	FlockPath      string         `mapstructure:"flock_path"` // deprecated
	BuntDBConfig   BuntDBConfig   `mapstructure:"buntdb"`     // deprecated
	SQLiteConfig   SQLiteConfig   `mapstructure:"sqlite"`     // deprecated
	PostgresConfig PostgresConfig `mapstructure:"postgres"`   // deprecated
}

// Each named PluginConfig block configures a plugin. The raw configuration RawPluginConfig is passed on to the plugin which
// parses its own configuration.
type PluginConfig struct {
	Name            string                 `mapstructure:"name"`
	RawPluginConfig map[string]interface{} `mapstructure:",remain"`
}

func GetFlagSet() *pflag.FlagSet {
	flagSet := pflag.NewFlagSet("configuration", pflag.ContinueOnError)
	flagSet.StringP("admin-user", "a", "", "id of the admin user")
	return flagSet
}

// wordSepNormalizeFunc allows for normalization of the flag names (which use - as a separator)
func wordSepNormalizeFunc(f *pflag.FlagSet, name string) pflag.NormalizedName {
	from := "-"
	to := "_"
	name = strings.Replace(name, from, to, -1)
	return pflag.NormalizedName(name)
}

// ReadConfiguration reads and parses the configuration located at configPath, which can either point to a single TOML
// file or to a directory, in which case all *.toml files in this directory are concatenated. It returns a Config
// object.
func ReadConfiguration(configPath string, flagSet *pflag.FlagSet) (*Config, error) {
	cfg := Config{}
	flagSet.SetNormalizeFunc(wordSepNormalizeFunc)
	viper.SetDefault("admin_user", defaultAdminUser)
	err := viper.BindPFlags(flagSet)
	if err != nil {
		globals.AppLogger.Error("could not bind flags (ignored)", "error", err)
	}
	viper.SetEnvPrefix("LSCHAT")
	viper.AutomaticEnv()
	if configPath != "" {
		fi, err := os.Stat(configPath)
		if err != nil {
			return nil, err
		}
		contents := make([]byte, 0)
		//fName := configPath
		files := []string{configPath}
		if fi.IsDir() {
			files, err = filepath.Glob(filepath.Join(configPath, "*.toml"))
			if err != nil {
				return nil, err
			}
		}
		for _, configFile := range files {
			fileContents, err := ioutil.ReadFile(configFile)
			if err != nil {
				return nil, err
			}
			contents = append(contents, fileContents...)
			contents = append(contents, '\n')
		}
		viper.SetConfigType("toml")
		err = viper.ReadConfig(bytes.NewBuffer(contents))
		if err != nil {
			globals.AppLogger.Error("could not read config:", "error", err)
		}
	}
	err = viper.Unmarshal(&cfg)
	if err != nil {
		globals.AppLogger.Error("could not unmarshal config:", "error", err)
	}

	globals.AppLogger.Info("config", "cfg", cfg, "all", viper.AllSettings())
	return &cfg, nil
}
