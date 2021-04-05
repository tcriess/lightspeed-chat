package types

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	Id         string         `json:"id" gorm:"primaryKey"`      // unique id
	Nick       string         `json:"nick" gorm:"index"`         // should also be unique
	Language   string         `json:"language"`                  // alpha-2 iso
	Tags       JSONStringMap  `json:"tags"`                      // tags
	LastOnline time.Time      `json:"last_online" hash:"ignore"` // last seen online
	CreatedAt  time.Time      `json:"-"`
	UpdatedAt  time.Time      `json:"-"`
	DeletedAt  gorm.DeletedAt `json:"-" gorm:"index"`
}
