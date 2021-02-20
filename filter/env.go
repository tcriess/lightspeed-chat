package filter

/*
Here the Env used in the event filters are defined.
Once this struct is fixed, it should not be changed, otherwise filters in history messages may not compile any more
(f.e. if properties are renamed etc.)
*/

type User struct {
	Id         string
	Nick       string
	Language   string
	Tags       map[string]string
	IntTags    map[string]int64
	LastOnline int64
}

type Room struct {
	Id    string
	Owner User
}

type Source struct {
	User
	PluginName string
}

type Client struct {
	ClientLanguage string
}

type Target struct {
	User
	Client
}

type Env struct {
	Room
	Source
	Target
	Created  int64
	Language string
	Name     string
	Tags     map[string]string
	IntTags  map[string]int64
}
