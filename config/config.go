package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Config struct {
	HistoryConfig     *HistoryConfig     `hcl:"history,block"`
	OIDCConfigs       []OIDCConfig       `hcl:"oidc,block"`
	PersistenceConfig *PersistenceConfig `hcl:"persistence,block"`
	PluginConfigs     []PluginConfig     `hcl:"plugin,block"`
	LogLevel          *string            `hcl:"log_level"`
}

type HistoryConfig struct {
	HistorySize int `hcl:"history_size"`
}

type OIDCConfig struct {
	Name        string `hcl:"name,label"`
	ClientId    string `hcl:"client_id"`
	ProviderUrl string `hcl:"provider_url"` // f.e. "https://accounts.google.com", this is used to construct the discovery url and subsequently discover the openid endpoints
}

type BundDBConfig struct {
	GlobalName string `hcl:"global_name"`
	RoomNameTemplate string `hcl:"room_name_template"`
}

type PersistenceConfig struct {
	BuntDBConfig *BundDBConfig `hcl:"buntdb,block"`
}

type PluginConfig struct {
	Name            string   `hcl:"name,label"`
	RawPluginConfig hcl.Body `hcl:",remain"`
}

/*
ReadConfiguration reads and parses the configuration located at configPath, which can either point to a single HCL file
or to a directory, in which case all *.hcl files in this directory are concatenated. It returns a Config object and
a map of unparsed plugin configurations.
*/
func ReadConfiguration(configPath string) (*Config, map[string]hcl.Body, error) {
	cfg := &Config{}
	pluginConfigs := make(map[string]hcl.Body)
	if configPath != "" {
		fi, err := os.Stat(configPath)
		if err != nil {
			return nil, nil, err
		}
		contents := make([]byte, 0)
		fName := configPath
		files := []string{configPath}
		if fi.IsDir() {
			files, err = filepath.Glob(filepath.Join(configPath, "*.hcl"))
			if err != nil {
				return nil, nil, err
			}
		}
		for _, configFile := range files {
			fileContents, err := ioutil.ReadFile(configFile)
			if err != nil {
				return nil, nil, err
			}
			contents = append(contents, fileContents...)
		}
		if !strings.HasSuffix(fName, ".hcl") {
			fName = "config.hcl" // dummy name with .hcl suffix
		}
		err = hclsimple.Decode(fName, contents, nil, cfg)
		if err != nil {
			return nil, nil, err
		}
		for _, pc := range cfg.PluginConfigs {
			if pc.Name != "" && pc.RawPluginConfig != nil {
				pluginConfigs[pc.Name] = pc.RawPluginConfig
			}
		}
	}
	return cfg, pluginConfigs, nil
}
