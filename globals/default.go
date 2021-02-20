package globals

import "github.com/hashicorp/go-hclog"

var AppLogger = hclog.New(&hclog.LoggerOptions{
	Name:  "lightspeed-chat",
	Level: hclog.LevelFromString("DEBUG"),
})