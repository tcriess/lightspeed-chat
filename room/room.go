package room

import "github.com/tcriess/lightspeed-chat/types"

// this is basically identified with one hub, it is just a logical separation
type Room struct {
	Users      []*types.User
	UserById   map[string]*types.User
	UserByNick map[string]*types.User
	Moderators map[string]*types.User
}
