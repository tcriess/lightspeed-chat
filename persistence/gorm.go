package persistence

import (
	"database/sql/driver"
	"fmt"
	"github.com/tcriess/lightspeed-chat/filter"
	"gorm.io/gorm/clause"
	"time"

	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/types"
	"gorm.io/datatypes"
	_ "gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var _ driver.Valuer = &datatypes.JSON{}

type GormPersist struct {
	db *gorm.DB
}

func NewGormPersister(cfg *config.Config) (Persister, error) {
	db, err := setupGormDB(cfg)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, nil // no or wrong configuration, ignore the persister
	}
	p := GormPersist{db: db}
	return &p, nil
}

func setupGormDB(cfg *config.Config) (*gorm.DB, error) {
	if cfg.PersistenceConfig.DSN == "" {
		return nil, nil
	}
	var dial gorm.Dialector
	switch cfg.PersistenceConfig.Type {
	case "postgres":
		dial = postgres.Open(cfg.PersistenceConfig.DSN)

	case "sqlite":
		dial = sqlite.Open(cfg.PersistenceConfig.DSN)

	default:
		return nil, fmt.Errorf("invalid gorm configuration")
	}
	db, err := gorm.Open(dial, &gorm.Config{})
	if err != nil {
		return nil, err
	}
	db.Migrator().AutoMigrate(&types.User{}, &types.Room{}, &types.Event{})
	return db, nil
}

func (p *GormPersist) StoreUser(user types.User) error {
	return p.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&user).Error
}

func (p *GormPersist) GetUser(user *types.User) error {
	return p.db.First(user).Error
}

func (p *GormPersist) GetUsers() ([]*types.User, error) {
	users := make([]*types.User, 0)
	err := p.db.Find(&users).Error
	return users, err
}

func (p *GormPersist) UpdateUserTags(user *types.User, updates []*types.TagUpdate) ([]bool, error) {
	res := make([]bool, len(updates))
	err := p.db.Transaction(func(tx *gorm.DB) error {
		err := tx.First(user).Error
		if err != nil {
			return err
		}
		tags := user.Tags
		res = filter.UpdateTags(tags, updates)
		return tx.Model(user).Update("tags", tags).Error
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (p *GormPersist) DeleteUser(user *types.User) error {
	return p.db.Delete(user).Error
}

func (p *GormPersist) StoreRoom(room types.Room) error {
	return p.db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&room).Error
}

func (p *GormPersist) GetRoom(room *types.Room) error {
	return p.db.First(room).Error
}

func (p *GormPersist) DeleteRoom(room *types.Room) error {
	return p.db.Delete(room).Error
}

func (p *GormPersist) GetRooms() ([]*types.Room, error) {
	rooms := make([]*types.Room, 0)
	err := p.db.Find(&rooms).Error
	return rooms, err
}

func (p *GormPersist) UpdateRoomTags(room *types.Room, updates []*types.TagUpdate) ([]bool, error) {
	res := make([]bool, len(updates))
	err := p.db.Transaction(func(tx *gorm.DB) error {
		err := tx.First(room).Error
		if err != nil {
			return err
		}
		tags := room.Tags
		res = filter.UpdateTags(tags, updates)
		return tx.Model(room).Update("tags", tags).Error
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (p *GormPersist) StoreEvents(_ *types.Room, events []*types.Event) error {
	return p.db.Create(&events).Error
}

func (p *GormPersist) GetEventHistory(room *types.Room, fromTs, toTs time.Time, fromIdx, maxCount int) ([]*types.Event, error) {
	events := make([]*types.Event, 0)
	err := p.db.Where("created BETWEEN ? AND ?", fromTs, toTs).Order("created DESC").Limit(maxCount).Offset(fromIdx).Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (p *GormPersist) Close() error {
	return nil
}