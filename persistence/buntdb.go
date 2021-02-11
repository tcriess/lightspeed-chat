package persistence

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/tcriess/lightspeed-chat/config"
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
		err = db.CreateIndex("chatbyts", "chat:*", buntdb.IndexJSON("timestamp"))
		if err != nil {
			db.Close()
			return nil, err
		}
		err = db.CreateIndex("translationbyts", "translation:*", buntdb.IndexJSON("timestamp"))
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
		u, err := tx.Get(user.Id)
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

func (p *BuntDBPersist) StoreChatMessage(chatMessage types.ChatMessage) error {
	chatMessage.Timestamp = chatMessage.Timestamp.In(time.UTC)
	msg, err := json.Marshal(chatMessage)
	if err != nil {
		return err
	}
	return p.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set("chat:"+chatMessage.Id, string(msg), nil)
		return err
	})
}

func (p *BuntDBPersist) StoreTranslationMessage(translationMessage types.TranslationMessage) error {
	translationMessage.Timestamp = translationMessage.Timestamp.In(time.UTC)
	msg, err := json.Marshal(translationMessage)
	if err != nil {
		return err
	}
	return p.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set("translation:"+translationMessage.SourceId+":"+translationMessage.Language, string(msg), nil)
		return err
	})
}

func (p *BuntDBPersist) GetChatHistory(fromTs, toTs time.Time, fromIdx, maxCount int) ([]types.ChatMessage, error) {
	log.Println("info: in GetChatHistory")
	messages := make([]types.ChatMessage, 0)

	fromCond := fmt.Sprintf(`{"timestamp":"%s"}`, fromTs.In(time.UTC).Format(time.RFC3339))
	toCond := fmt.Sprintf(`{"timestamp":"%s"}`, toTs.In(time.UTC).Format(time.RFC3339))

	err := p.db.View(func(tx *buntdb.Tx) error {
		currentNo := -1
		count := 0
		log.Printf("from: %s, to: %s", fromTs.In(time.UTC).Format(time.RFC3339), toTs.In(time.UTC).Format(time.RFC3339))
		return tx.DescendRange("chatbyts", toCond, fromCond, func(key, val string) bool {
			log.Printf("got chat history entry: %s %s", key, val)
			currentNo++
			if currentNo < fromIdx {
				return true
			}
			cm := types.ChatMessage{}
			if err := json.Unmarshal([]byte(val), &cm); err == nil {
				messages = append(messages, cm)
			}
			count++
			return maxCount <= 0 || count < maxCount
		})
	})
	return messages, err
}

func (p *BuntDBPersist) GetTranslationHistory(fromTs, toTs time.Time, fromIdx, maxCount int) ([]types.TranslationMessage, error) {
	messages := make([]types.TranslationMessage, 0)

	fromCond := fmt.Sprintf(`{"timestamp":"%s"}`, fromTs.In(time.UTC).Format(time.RFC3339))
	toCond := fmt.Sprintf(`{"timestamp":"%s"}`, toTs.In(time.UTC).Format(time.RFC3339))

	err := p.db.View(func(tx *buntdb.Tx) error {
		currentNo := -1
		count := 0
		return tx.DescendRange("translationbyts", toCond, fromCond, func(key, val string) bool {
			currentNo++
			if currentNo < fromIdx {
				return true
			}
			tm := types.TranslationMessage{}
			if err := json.Unmarshal([]byte(val), &tm); err == nil {
				messages = append(messages, tm)
			}
			count++
			return maxCount <= 0 || count < maxCount
		})
	})
	return messages, err
}

func (p *BuntDBPersist) Close() error {
	return p.db.Close()
}
