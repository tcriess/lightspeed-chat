package types

// Make sure those constants are the same as defined in the proto enum!
const (
	TagValueTypeString      = 0
	TagValueTypeInt         = 1
	TagValueTypeFloat       = 2
	TagValueTypeStringSlice = 3
	TagValueTypeIntSlice    = 4
	TagValueTypeFloatSlice  = 5
)

// a slice of TagUpdate objects is used for atomically updating Tags values (users/rooms/...)
type TagUpdate struct {
	Name       string `json:"name"`       // tag name
	Type       int    `json:"type"`       // type - TagValueType* const
	Index      int    `json:"index"`      // used for slice types
	Expression string `json:"expression"` // expr expression to apply to the Tags, the tag "Name" is set to the resulting value
}
