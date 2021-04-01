package types

import (
	"time"
)

type User struct {
	Id         string        `json:"id"`                        // e-mail, unique!
	Nick       string        `json:"nick"`                      // should also be unique
	Language   string        `json:"language"`                  // alpha-2 iso
	Tags       JSONStringMap `json:"tags"`                      // tags
	LastOnline time.Time     `json:"last_online" hash:"ignore"` // last seen online
}
