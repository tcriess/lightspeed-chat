package persistence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"text/template"
	"time"

	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/filter"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/tidwall/buntdb"
)

// BuntDBPersist is a Persister for BuntDB (file-backed in-memory key-value store)
type BuntDBPersist struct {
	db                     *buntdb.DB
	roomDbs                map[string]*buntdb.DB
	roomDbFileNameTemplate *template.Template
	sync.RWMutex
}

func NewBuntPersister(cfg *config.Config) (Persister, error) {
	db, roomDbs, t, err := setupBuntDB(cfg)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, nil // no or wrong configuration, ignore the persister
	}
	return &BuntDBPersist{db: db, roomDbs: roomDbs, roomDbFileNameTemplate: t}, nil
}

func getRoomDbName(room *types.Room, t *template.Template) (string, error) {
	def := struct {
		RoomId string
	}{
		RoomId: room.Id,
	}
	buf := &bytes.Buffer{}
	err := t.Execute(buf, def)
	if err != nil {
		return "", err
	}
	fileName := buf.String()
	if fileName == "" {
		return "", fmt.Errorf("room file name empty")
	}
	return fileName, nil
}

func setupBuntDB(cfg *config.Config) (*buntdb.DB, map[string]*buntdb.DB, *template.Template, error) {
	var db *buntdb.DB
	roomDbs := make(map[string]*buntdb.DB)
	var t *template.Template
	if cfg.PersistenceConfig.BuntDBConfig.GlobalName != "" && cfg.PersistenceConfig.BuntDBConfig.RoomNameTemplate != "" {
		fileName := cfg.PersistenceConfig.BuntDBConfig.GlobalName
		var err error
		db, err = buntdb.Open(fileName)
		if err != nil {
			return nil, nil, nil, err
		}

		err = db.CreateIndex("rooms", "room:*", buntdb.IndexJSON("id"))
		if err != nil {
			db.Close()
			return nil, nil, nil, err
		}
		err = db.CreateIndex("users", "user:*", buntdb.IndexJSON("id"))
		if err != nil {
			db.Close()
			return nil, nil, nil, err
		}
		rooms := make([]*types.Room, 0)
		db.View(func(tx *buntdb.Tx) error {
			tx.Descend("rooms", func(key, val string) bool {
				room := &types.Room{}
				if err := json.Unmarshal([]byte(val), room); err == nil {
					rooms = append(rooms, room)
				}
				return true
			})
			return nil
		})
		t = template.Must(template.New("room_db").Parse(cfg.PersistenceConfig.BuntDBConfig.RoomNameTemplate))
		for _, room := range rooms {
			if room.Id == "" {
				continue
			}
			fileName, err := getRoomDbName(room, t)
			if err != nil {
				return nil, nil, nil, err
			}
			roomDb, err := buntdb.Open(fileName)
			if err != nil {
				return nil, nil, nil, err
			}
			err = roomDb.CreateIndex("eventsts", "event:*", buntdb.IndexJSON("created"))
			if err != nil {
				db.Close()
				for _, rDb := range roomDbs {
					rDb.Close()
				}
				return nil, nil, nil, err
			}
			roomDbs[room.Id] = roomDb
		}
	}
	return db, roomDbs, t, nil
}

func (p *BuntDBPersist) StoreUser(user types.User) error {
	u, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return p.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set("user:"+user.Id, string(u), nil)
		return err
	})
}

func (p *BuntDBPersist) GetUser(user *types.User) error {
	if user.Id == "" {
		return fmt.Errorf("no user id")
	}
	err := p.db.View(func(tx *buntdb.Tx) error {
		u, err := tx.Get("user:" + user.Id)
		if err != nil {
			return err
		}
		err = json.Unmarshal([]byte(u), user)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *BuntDBPersist) GetUsers() ([]*types.User, error) {
	users := make([]*types.User, 0)
	err := p.db.View(func(tx *buntdb.Tx) error {
		tx.Descend("users", func(key, val string) bool {
			log.Printf("got user: %s %s", key, val)
			user := &types.User{}
			if err := json.Unmarshal([]byte(val), user); err == nil {
				users = append(users, user)
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (p *BuntDBPersist) UpdateUserTags(user *types.User, updates []*types.TagUpdate) ([]bool, error) {
	resOk := make([]bool, len(updates))
	if user.Id == "" {
		return nil, fmt.Errorf("no user id")
	}
	err := p.db.Update(func(tx *buntdb.Tx) error {
		u, err := tx.Get("user:" + user.Id)
		if err != nil {
			return err
		}
		err = json.Unmarshal([]byte(u), user)
		if err != nil {
			return err
		}
		if user.Tags == nil {
			user.Tags = make(map[string]string)
		}
		resOk = filter.UpdateTags(user.Tags, updates)
		newUser, err := json.Marshal(user)
		if err != nil {
			return err
		}
		_, _, err = tx.Set("user:" + user.Id, string(newUser), nil)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resOk, nil
}

func (p *BuntDBPersist) DeleteUser(user *types.User) error {
	if user.Id == "" {
		return fmt.Errorf("no user id")
	}
	err := p.db.View(func(tx *buntdb.Tx) error {
		_, err := tx.Delete("user:" + user.Id)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *BuntDBPersist) StoreRoom(room types.Room) error {
	u, err := json.Marshal(room)
	if err != nil {
		return err
	}

	p.Lock()
	defer p.Unlock()

	replaced := true
	oldVal := ""
	err = p.db.Update(func(tx *buntdb.Tx) error {
		var err error
		oldVal, replaced, err = tx.Set("room:"+room.Id, string(u), nil)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, ok := p.roomDbs[room.Id]; !replaced || !ok {
		// created new room, also create the db and add it to the map of dbs here
		fileName, err := getRoomDbName(&room, p.roomDbFileNameTemplate)
		if err != nil {
			if replaced {
				p.db.Update(func(tx *buntdb.Tx) error {
					_, _, err := tx.Set("room:"+room.Id, oldVal, nil)
					return err
				})
			} else {
				p.db.Update(func(tx *buntdb.Tx) error {
					_, err := tx.Delete("room:" + room.Id)
					return err
				})
			}
			return err
		}
		roomDb, err := buntdb.Open(fileName)
		if err != nil {
			if replaced {
				p.db.Update(func(tx *buntdb.Tx) error {
					_, _, err := tx.Set("room:"+room.Id, oldVal, nil)
					return err
				})
			} else {
				p.db.Update(func(tx *buntdb.Tx) error {
					_, err := tx.Delete("room:" + room.Id)
					return err
				})
			}
			return err
		}
		err = roomDb.CreateIndex("eventsts", "event:*", buntdb.IndexJSON("created"))
		if err != nil {
			roomDb.Close()
			if replaced {
				p.db.Update(func(tx *buntdb.Tx) error {
					_, _, err := tx.Set("room:"+room.Id, oldVal, nil)
					return err
				})
			} else {
				p.db.Update(func(tx *buntdb.Tx) error {
					_, err := tx.Delete("room:" + room.Id)
					return err
				})
			}
			return err
		}
		p.roomDbs[room.Id] = roomDb
	}
	return nil
}

func (p *BuntDBPersist) GetRoom(room *types.Room) error {
	if room.Id == "" {
		return fmt.Errorf("no room id")
	}
	err := p.db.View(func(tx *buntdb.Tx) error {
		u, err := tx.Get("room:" + room.Id)
		if err != nil {
			return err
		}
		err = json.Unmarshal([]byte(u), room)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *BuntDBPersist) DeleteRoom(room *types.Room) error {
	if room.Id == "" {
		return fmt.Errorf("no room id")
	}

	p.Lock()
	defer p.Unlock()

	oldVal := ""
	err := p.db.Update(func(tx *buntdb.Tx) error {
		var err error
		oldVal, err = tx.Delete("room:" + room.Id)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		p.db.Update(func(tx *buntdb.Tx) error {
			_, _, err := tx.Set("room:"+room.Id, oldVal, nil)
			return err
		})
		return err
	}
	delete(p.roomDbs, "room:"+room.Id)
	return nil
}

func (p *BuntDBPersist) GetRooms() ([]*types.Room, error) {
	rooms := make([]*types.Room, 0)
	err := p.db.View(func(tx *buntdb.Tx) error {
		tx.Descend("rooms", func(key, val string) bool {
			log.Printf("got room: %s %s", key, val)
			room := &types.Room{}
			if err := json.Unmarshal([]byte(val), room); err == nil {
				rooms = append(rooms, room)
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rooms, nil
}

func (p *BuntDBPersist) UpdateRoomTags(room *types.Room, updates []*types.TagUpdate) ([]bool, error) {
	resOk := make([]bool, len(updates))
	if room.Id == "" {
		return nil, fmt.Errorf("no room id")
	}
	err := p.db.Update(func(tx *buntdb.Tx) error {
		r, err := tx.Get("room:" + room.Id)
		if err != nil {
			return err
		}
		err = json.Unmarshal([]byte(r), room)
		if err != nil {
			return err
		}
		if room.Tags == nil {
			room.Tags = make(map[string]string)
		}
		resOk = filter.UpdateTags(room.Tags, updates)
		newRoom, err := json.Marshal(room)
		if err != nil {
			return err
		}
		_, _, err = tx.Set("room:" + room.Id, string(newRoom), nil)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resOk, nil
}

func (p *BuntDBPersist) StoreEvents(room *types.Room, events []*types.Event) error {
	if len(events) == 0 {
		return nil
	}
	if room == nil {
		return fmt.Errorf("no room")
	}

	p.RLock()
	defer p.RUnlock()

	if roomDb, ok := p.roomDbs[room.Id]; ok {
		return roomDb.Update(func(tx *buntdb.Tx) error {
			for _, event := range events {
				msg, err := json.Marshal(event)
				if err != nil {
					globals.AppLogger.Error("could not marshal event", "error", err)
					return err
				}
				_, _, err = tx.Set("event:"+event.Id, string(msg), nil)
				if err != nil {
					globals.AppLogger.Error("could not store event", "error", err)
					return err
				}
			}
			return nil
		})
	} else {
		return fmt.Errorf("no room db")
	}
}

// GetEventHistory returns a slice of events from db.
//
// Use fromTs/toTs to restrict the time range, and fromIdx/maxCount for pagination.
// Important: the resulting events are expected to have the "History" flag set!
func (p *BuntDBPersist) GetEventHistory(room *types.Room, fromTs, toTs time.Time, fromIdx, maxCount int) ([]*types.Event, error) {
	log.Println("info: in GetEventHistory")
	if room == nil {
		return nil, fmt.Errorf("no room")
	}
	events := make([]*types.Event, 0)

	fromCond := fmt.Sprintf(`{"created":"%s"}`, fromTs.In(time.UTC).Format(time.RFC3339))
	toCond := fmt.Sprintf(`{"created":"%s"}`, toTs.In(time.UTC).Format(time.RFC3339))

	p.RLock()
	defer p.RUnlock()

	if roomDb, ok := p.roomDbs[room.Id]; ok {
		err := roomDb.View(func(tx *buntdb.Tx) error {
			currentNo := -1
			count := 0
			log.Printf("from: %s, to: %s", fromTs.In(time.UTC).Format(time.RFC3339), toTs.In(time.UTC).Format(time.RFC3339))
			return tx.DescendRange("eventsts", toCond, fromCond, func(key, val string) bool {
				log.Printf("got chat history entry: %s %s", key, val)
				currentNo++
				if currentNo < fromIdx {
					return true
				}
				event := &types.Event{}
				if err := json.Unmarshal([]byte(val), event); err == nil {
					event.History = true
					events = append(events, event)
				}
				count++
				return maxCount <= 0 || count < maxCount
			})
		})
		return events, err
	} else {
		return nil, fmt.Errorf("no room db")
	}
}

func (p *BuntDBPersist) Close() error {
	var err error
	err = p.db.Close()
	for _, roomDb := range p.roomDbs {
		rErr := roomDb.Close()
		if err == nil && rErr != nil {
			err = rErr
		}
	}
	return err
}
