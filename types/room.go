package types

// this is basically identified with one hub, it is just a logical separation
type Room struct {
	Id    string `json:"id"`
	Owner *User  `json:"owner"`
	// TODO: define the final structure
	users      []*User
	userById   map[string]*User
	userByNick map[string]*User
	moderators map[string]*User
}
