package types

import "time"

type User struct {
	Id         string            `json:"id"`          // e-mail, unique!
	Nick       string            `json:"nick"`        // should also be unique
	Language   string            `json:"language"`    // alpha-2 iso
	Tags       map[string]string `json:"tags"`        // tags
	IntTags    map[string]int64  `json:"int_tags"`    // integer tags (we use int64 here to avoid casting back and forth when transmitting via grpc)
	LastOnline time.Time         `json:"last_online"` // last seen online
}
