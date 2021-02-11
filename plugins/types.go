package plugins

type PluginSpec struct {
	Name        string
	Plugin      EventHandler
	CronSpec    string
	EventFilter string
}
