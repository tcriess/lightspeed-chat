package filter

type User struct {
	Id         string
	Tags       map[string]string
	IntTags    map[string]int64
	LastOnline int64
}

type Client struct {
	ClientLanguage string
}

type Message struct {
	MessageLanguage string
}

type Command struct {
	Command string
	Nick    string
}

type Env struct {
	User
	Client
	Message
	Command
}
