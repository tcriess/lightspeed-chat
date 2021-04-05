package types

import (
	"gorm.io/gorm"
	"time"
)

// this is basically identified with one hub, it is just a logical separation
type Room struct {
	Id        string         `json:"id" gorm:"primaryKey"`
	OwnerId   string         `json:"-"`
	Owner     *User          `json:"owner"`
	Tags      JSONStringMap  `json:"tags"` // tags
	CreatedAt time.Time      `json:"-"`
	UpdatedAt time.Time      `json:"-"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
	// TODO: define the final structure
	users      []*User
	userById   map[string]*User
	userByNick map[string]*User
	moderators map[string]*User
}
