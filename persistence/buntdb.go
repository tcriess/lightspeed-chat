package persistence

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/globals"
	"github.com/tcriess/lightspeed-chat/types"
	"github.com/tidwall/buntdb"
)

type BuntDBPersist struct {
	db *buntdb.DB
}

func NewBuntPersister(cfg *config.Config) (Persister, error) {
	db, err := setupBuntDB(cfg)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, nil // no or wrong configuration, ignore the persister
	}
	return &BuntDBPersist{db}, nil
}

func setupBuntDB(cfg *config.Config) (*buntdb.DB, error) {
	var db *buntdb.DB
	if cfg.PersistenceConfig != nil && cfg.PersistenceConfig.BuntDBConfig != nil && cfg.PersistenceConfig.BuntDBConfig.Name != "" {
		fileName := cfg.PersistenceConfig.BuntDBConfig.Name
		var err error
		db, err = buntdb.Open(fileName)
		if err != nil {
			return nil, err
		}
		err = db.CreateIndex("eventsts", "event:*", buntdb.IndexJSON("created"))
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
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

func (p *BuntDBPersist) StoreRoom(room types.Room) error {
	u, err := json.Marshal(room)
	if err != nil {
		return err
	}
	return p.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set("room:"+room.Id, string(u), nil)
		return err
	})
}

func (p *BuntDBPersist) GetRoom(room *types.Room) error {
	if room.Id == "" {
		return fmt.Errorf("no user id")
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

func (p *BuntDBPersist) StoreEvents(events []*types.Event) error {
	return p.db.Update(func(tx *buntdb.Tx) error {
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
}

// GetEventHistory returns a slice of events from db.
//
// Use fromTs/toTs to restrict the time range, and fromIdx/maxCount for pagination.
// Important: the resulting events are expected to have the "History" flag set!
func (p *BuntDBPersist) GetEventHistory(fromTs, toTs time.Time, fromIdx, maxCount int) ([]*types.Event, error) {
	log.Println("info: in GetEventHistory")
	events := make([]*types.Event, 0)

	fromCond := fmt.Sprintf(`{"created":"%s"}`, fromTs.In(time.UTC).Format(time.RFC3339))
	toCond := fmt.Sprintf(`{"created":"%s"}`, toTs.In(time.UTC).Format(time.RFC3339))

	err := p.db.View(func(tx *buntdb.Tx) error {
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
}

func (p *BuntDBPersist) Close() error {
	return p.db.Close()
}
