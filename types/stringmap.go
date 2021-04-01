package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// JSONStringMap defined JSON data type, need to implements driver.Valuer, sql.Scanner interface
type JSONStringMap map[string]string

// Value return json value, implement driver.Valuer interface
func (m JSONStringMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	ba, err := m.MarshalJSON()
	return string(ba), err
}

// Scan scan value into Jsonb, implements sql.Scanner interface
func (m *JSONStringMap) Scan(val interface{}) error {
	var ba []byte
	switch v := val.(type) {
	case []byte:
		ba = v
	case string:
		ba = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", val))
	}
	t := map[string]string{}
	err := json.Unmarshal(ba, &t)
	*m = JSONStringMap(t)
	return err
}

// MarshalJSON to output non base64 encoded []byte
func (m JSONStringMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	t := (map[string]string)(m)
	return json.Marshal(t)
}

// UnmarshalJSON to deserialize []byte
func (m *JSONStringMap) UnmarshalJSON(b []byte) error {
	t := map[string]string{}
	err := json.Unmarshal(b, &t)
	*m = JSONStringMap(t)
	return err
}

// GormDataType gorm common data type
func (m JSONStringMap) GormDataType() string {
	return "jsonstringmap"
}

// GormDBDataType gorm db data type
func (JSONStringMap) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "JSON"
	case "mysql":
		return "JSON"
	case "postgres":
		return "JSONB"
	}
	return ""
}