package globals

import "github.com/hashicorp/go-hclog"

// AppLogger is the global hclog logger
var AppLogger = hclog.New(&hclog.LoggerOptions{
	Name:  "lightspeed-chat",
	Level: hclog.LevelFromString("DEBUG"),
})
