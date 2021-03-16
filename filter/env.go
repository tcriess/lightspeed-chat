package filter

/*
Here the Env used in the event filters are defined.
Once this struct is fixed, it should not be changed, otherwise target filters in history messages may not compile any
more (f.e. if properties are renamed etc.)
The following structs represent all the information that is required inside the filter expression for events.
Note that currently there are two types of filters:
- target filters are part of every event and determine to which clients an event is sent
- plugin filters are part of every plugin and determine which events are sent to the plugin
*/

// User is the representation of types.User inside the Env
type User struct {
	Id         string
	Nick       string
	Language   string
	Tags       map[string]string
	LastOnline int64
}

// Room is the representation of types.Room inside the Env
type Room struct {
	Id    string
	Owner User
	Tags  map[string]string
}

// Source is the representation of types.Event.Source inside the Env
type Source struct {
	User
	PluginName string
}

// Client is the representation of the connected client ws.Client inside the Env
type Client struct {
	ClientLanguage string
}

// Target represents the User and Client that the event is about to be sent to
type Target struct {
	User
	Client
}

// Env is the complete environment of input data for target or plugin filters
type Env struct {
	Room
	Source
	Target
	Created       int64
	Language      string
	Name          string
	Tags          map[string]string
	AsInt         func(string) int64
	AsFloat       func(string) float64
	AsStringSlice func(string) []string
	AsIntSlice    func(string) []int64
	AsFloatSlice  func(string) []float64
}

// TagsEnv is the environment for tag updates (see UpdateTags)
type TagsEnv struct {
	Tags map[string]string
	AsInt         func(string) int64
	AsFloat       func(string) float64
	AsStringSlice func(string) []string
	AsIntSlice    func(string) []int64
	AsFloatSlice  func(string) []float64
}
