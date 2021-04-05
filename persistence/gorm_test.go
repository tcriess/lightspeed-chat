package persistence

import (
	"encoding/json"
	"fmt"
	"github.com/tcriess/lightspeed-chat/config"
	"testing"
	"time"

	"github.com/tcriess/lightspeed-chat/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//var _ driver.Valuer = &JSONStringMap{}

type TestModel struct {
	gorm.Model
	Tags types.JSONStringMap
}

func TestNewGormPersister(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%v", db)
	db.Migrator().AutoMigrate(&TestModel{})
	tags := make(map[string]string)
	tags["hello"] = "123"
	m := TestModel{Tags: tags}
	err = db.Create(&m).Error
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{}
	cfg.PersistenceConfig.Type = "sqlite"
	cfg.PersistenceConfig.DSN = "test.db" // "file::memory:?cache=shared"
	p, err := NewGormPersister(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	userTags := make(map[string]string)
	user := types.User{
		Id:         "testuser",
		Nick:       "testuser",
		Language:   "en",
		Tags:       userTags,
		LastOnline: time.Time{},
	}
	p.StoreUser(user)
	roomTags := make(map[string]string)
	room := types.Room{
		Id:    "test",
		Owner: &user,
		Tags:  roomTags,
	}
	p.StoreRoom(room)

	users, err := p.GetUsers()
	if err != nil {
		t.Fatal(err)
	}
	usersJson, _ := json.Marshal(users)
	fmt.Printf("%+v", string(usersJson))

	updates := make([]*types.TagUpdate, 1)
	updates[0] = &types.TagUpdate{
		Name:       "testtag",
		Type:       types.TagValueTypeInt,
		Index:      0,
		Expression: "17",
	}
	res, err := p.UpdateUserTags(&user, updates)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%+v", res)
	userJson, _ := json.Marshal(user)
	fmt.Printf("%+v", string(userJson))

	events := make([]*types.Event, 2)
	events[0] = &types.Event{
		Id:           "HASH1",
		Room:         &room,
		Source:       &types.Source{
			User:       &user,
			PluginName: "",
		},
		Language:     "en",
		Name:         "chat",
		Tags:         make(types.JSONStringMap),
		History:      false,
		Sent:         time.Time{},
		TargetFilter: "",
	}
	events[1] = &types.Event{
		Id:           "HASH2",
		Room:         &room,
		Source:       &types.Source{
			User:       &user,
			PluginName: "",
		},
		Language:     "en",
		Name:         "chat",
		Tags:         nil,
		History:      false,
		Sent:         time.Time{},
		TargetFilter: "",
	}
	p.StoreEvents(&room, events)
}
