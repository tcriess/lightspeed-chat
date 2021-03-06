package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gofrs/flock"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tcriess/lightspeed-chat/config"
	"github.com/tcriess/lightspeed-chat/filter"
	"github.com/tcriess/lightspeed-chat/types"
)

type SQLitePersist struct {
	db *sql.DB
	sync.RWMutex
	*flock.Flock
}

func NewSQLitePersister(cfg *config.Config) (Persister, error) {
	db, err := setupSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, nil // no or wrong configuration, ignore the persister
	}
	p := SQLitePersist{db: db}
	if cfg.PersistenceConfig.FlockPath != "" {
		p.Flock = flock.New(cfg.PersistenceConfig.FlockPath)
	}
	return &p, nil
}

func setupSQLiteDB(cfg *config.Config) (*sql.DB, error) {
	if cfg.PersistenceConfig.SQLiteConfig.DSN == "" {
		return nil, nil
	}
	db, err := sql.Open("sqlite3", cfg.PersistenceConfig.SQLiteConfig.DSN)
	if err != nil {
		return nil, err
	}
	query := `CREATE TABLE IF NOT EXISTS users (
id TEXT PRIMARY KEY,
nick TEXT NOT NULL UNIQUE,
language TEXT DEFAULT "en" NOT NULL,
last_online INTEGER DEFAULT 0 NOT NULL,
tags TEXT DEFAULT "{}" NOT NULL
);
`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}
	query = `CREATE TABLE IF NOT EXISTS rooms (
id TEXT PRIMARY KEY,
owner_id TEXT NOT NULL,
tags TEXT DEFAULT "{}" NOT NULL,
FOREIGN KEY (owner_id) REFERENCES users (id) ON DELETE CASCADE ON UPDATE CASCADE
);`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}
	query = `CREATE TABLE IF NOT EXISTS events (
id TEXT PRIMARY KEY,
room_id TEXT NOT NULL,
user_id TEXT,
plugin_name TEXT DEFAULT "" NOT NULL,
name TEXT NOT NULL,
language TEXT DEFAULT "en" NOT NULL,
tags TEXT DEFAULT "{}" NOT NULL,
target_filter TEXT DEFAULT "" NOT NULL,
created INTEGER DEFAULT 0 NOT NULL,
created_sort INTEGER DEFAULT 0 NOT NULL,
sent INTEGER DEFAULT 0 NOT NULL,
FOREIGN KEY (room_id) REFERENCES rooms (id) ON DELETE CASCADE ON UPDATE CASCADE,
FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL ON UPDATE CASCADE
);`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}
	query = `CREATE INDEX IF NOT EXISTS events_created_idx ON events (created, created_sort);`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}
	return db, err
}

func (p *SQLitePersist) StoreUser(user types.User) error {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	if user.Tags == nil {
		user.Tags = make(map[string]string)
	}
	tags, err := json.Marshal(user.Tags)
	if err != nil {
		return err
	}
	query := `INSERT INTO users (id,nick,language,last_online,tags) VALUES (?,?,?,?,?) ON CONFLICT (id) DO UPDATE SET nick=EXCLUDED.nick,language=EXCLUDED.language,last_online=EXCLUDED.last_online,tags=EXCLUDED.tags;`
	_, err = p.db.Exec(query, user.Id, user.Nick, user.Language, user.LastOnline.Unix(), tags)
	return err
}

func (p *SQLitePersist) GetUser(user *types.User) error {
	p.RWMutex.RLock()
	defer p.RWMutex.RUnlock()
	if p.Flock != nil {
		p.Flock.RLock()
		defer p.Flock.Unlock()
	}
	var lastOnline int64
	var tagsRaw string
	query := `SELECT nick,language,last_online,tags FROM users WHERE id=?;`
	err := p.db.QueryRow(query, user.Id).Scan(&user.Nick, &user.Language, &lastOnline, &tagsRaw)
	if err != nil {
		return err
	}
	user.LastOnline = time.Unix(lastOnline, 0)
	tags := make(map[string]string)
	_ = json.Unmarshal([]byte(tagsRaw), &tags)
	user.Tags = tags
	return nil
}

func (p *SQLitePersist) GetUsers() ([]*types.User, error) {
	p.RWMutex.RLock()
	defer p.RWMutex.RUnlock()
	if p.Flock != nil {
		p.Flock.RLock()
		defer p.Flock.Unlock()
	}
	users := make([]*types.User, 0)
	query := `SELECT id,nick,language,last_online,tags FROM users;`
	rows, err := p.db.Query(query)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var user types.User
		var lastOnline int64
		var tagsRaw string
		err = rows.Scan(&user.Id, &user.Nick, &user.Language, &lastOnline, &tagsRaw)
		if err != nil {
			return nil, err
		}
		user.LastOnline = time.Unix(lastOnline, 0)
		tags := make(map[string]string)
		_ = json.Unmarshal([]byte(tagsRaw), &tags)
		user.Tags = tags
		users = append(users, &user)
	}
	return users, nil
}

func (p *SQLitePersist) UpdateUserTags(user *types.User, updates []*types.TagUpdate) ([]bool, error) {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	ctx := context.Background()
	resOk := make([]bool, len(updates))
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	var tagsRaw string
	query := `SELECT tags FROM users WHERE id=?;`
	err = tx.QueryRowContext(ctx, query, user.Id).Scan(&tagsRaw)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	tags := make(map[string]string)
	_ = json.Unmarshal([]byte(tagsRaw), &tags)
	resOk = filter.UpdateTags(tags, updates)
	tagsRawBytes, err := json.Marshal(tags)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	query = `UPDATE users SET tags=? WHERE id=?;`
	_, err = tx.ExecContext(ctx, query, tagsRawBytes, user.Id)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	user.Tags = tags
	return resOk, nil
}

func (p *SQLitePersist) DeleteUser(user *types.User) error {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	query := `DELETE FROM users WHERE id=?;`
	_, err := p.db.Exec(query, user.Id)
	return err
}

func (p *SQLitePersist) StoreRoom(room types.Room) error {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	if room.Tags == nil {
		room.Tags = make(map[string]string)
	}
	tags, err := json.Marshal(room.Tags)
	if err != nil {
		return err
	}
	query := `INSERT INTO rooms (id,owner_id,tags) VALUES (?,?,?) ON CONFLICT (id) DO UPDATE SET owner_id=EXCLUDED.owner_id, tags=EXCLUDED.tags;`
	_, err = p.db.Exec(query, room.Id, room.Owner.Id, tags)
	return err
}

func (p *SQLitePersist) GetRoom(room *types.Room) error {
	p.RWMutex.RLock()
	defer p.RWMutex.RUnlock()
	if p.Flock != nil {
		p.Flock.RLock()
		defer p.Flock.Unlock()
	}
	user := types.User{}
	var lastOnline int64
	var roomTagsRaw, userTagsRaw string
	query := `SELECT r.tags,r.owner_id,u.nick,u.language,u.last_online,u.tags FROM rooms AS r INNER JOIN users AS u ON r.owner_id=u.id WHERE r.id=?;`
	err := p.db.QueryRow(query, room.Id).Scan(&roomTagsRaw, &user.Id, &user.Nick, &user.Language, &lastOnline, &userTagsRaw)
	if err != nil {
		return err
	}
	user.LastOnline = time.Unix(lastOnline, 0)
	roomTags := make(map[string]string)
	_ = json.Unmarshal([]byte(roomTagsRaw), &roomTags)
	room.Tags = roomTags
	userTags := make(map[string]string)
	_ = json.Unmarshal([]byte(userTagsRaw), &userTags)
	user.Tags = userTags
	room.Owner = &user
	return nil
}

func (p *SQLitePersist) DeleteRoom(room *types.Room) error {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	query := `DELETE FROM rooms WHERE id=?;`
	_, err := p.db.Exec(query, room.Id)
	return err
}

func (p *SQLitePersist) GetRooms() ([]*types.Room, error) {
	p.RWMutex.RLock()
	defer p.RWMutex.RUnlock()
	if p.Flock != nil {
		p.Flock.RLock()
		defer p.Flock.Unlock()
	}
	rooms := make([]*types.Room, 0)
	query := `SELECT r.id,r.tags,r.owner_id,u.nick,u.language,u.last_online,u.tags FROM rooms AS r INNER JOIN users AS u ON r.owner_id=u.id;`
	rows, err := p.db.Query(query)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var user types.User
		var room types.Room
		var lastOnline sql.NullInt64
		var roomTagsRaw string
		var uid, nick, language, userTagsRaw sql.NullString
		err = rows.Scan(&room.Id, &roomTagsRaw, &uid, &nick, &language, &lastOnline, &userTagsRaw)
		if err != nil {
			return nil, err
		}
		if uid.Valid {
			user.Id = uid.String
			user.Nick = nick.String
			user.Language = language.String
			user.LastOnline = time.Unix(lastOnline.Int64, 0)
		}
		roomTags := make(map[string]string)
		_ = json.Unmarshal([]byte(roomTagsRaw), &roomTags)
		room.Tags = roomTags
		userTags := make(map[string]string)
		_ = json.Unmarshal([]byte(userTagsRaw.String), &userTags)
		user.Tags = userTags
		room.Owner = &user
		rooms = append(rooms, &room)
	}
	return rooms, nil
}

func (p *SQLitePersist) UpdateRoomTags(room *types.Room, updates []*types.TagUpdate) ([]bool, error) {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	resOk := make([]bool, len(updates))
	if room.Id == "" {
		return nil, fmt.Errorf("no room id")
	}
	tx, err := p.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	var tagsRaw string
	query := `SELECT tags FROM rooms WHERE id=?;`
	err = tx.QueryRow(query, room.Id).Scan(&tagsRaw)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	tags := make(map[string]string)
	_ = json.Unmarshal([]byte(tagsRaw), &tags)
	resOk = filter.UpdateTags(tags, updates)
	tagsRawBytes, err := json.Marshal(tags)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	query = `UPDATE rooms SET tags=? WHERE id=?;`
	_, err = tx.Exec(query, tagsRawBytes, room.Id)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	room.Tags = tags
	return resOk, nil
}

func (p *SQLitePersist) StoreEvents(_ *types.Room, events []*types.Event) error {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	tx, err := p.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	query := `INSERT INTO events (id,room_id,user_id,plugin_name,name,language,tags,target_filter,created,created_sort,sent) VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT (id) DO NOTHING;`
	for _, event := range events {
		if event.Tags == nil {
			event.Tags = make(map[string]string)
		}
		tags, err := json.Marshal(event.Tags)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		uid := sql.NullString{}
		if event.Source.User != nil && event.Source.User.Id != "" {
			uid.Valid = true
			uid.String = event.Source.User.Id
		}
		sort := event.Created.Nanosecond()
		_, err = tx.Exec(query, event.Id, event.Room.Id, uid, event.Source.PluginName, event.Name, event.Language, tags, event.TargetFilter, event.Created.Unix(), sort, event.Sent.Unix())
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	err = tx.Commit()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return nil
}

func (p *SQLitePersist) GetEventHistory(room *types.Room, fromTs, toTs time.Time, fromIdx, maxCount int) ([]*types.Event, error) {
	p.RWMutex.RLock()
	defer p.RWMutex.RUnlock()
	if p.Flock != nil {
		p.Flock.RLock()
		defer p.Flock.Unlock()
	}
	if room == nil {
		return nil, fmt.Errorf("no room")
	}
	events := make([]*types.Event, 0)
	from := fromTs.Unix()
	to := toTs.Unix()
	query := `SELECT e.id,e.room_id,e.user_id,e.plugin_name,e.name,e.language,e.tags,e.target_filter,e.created,e.sent,r.owner_id,r.tags,
       u.nick,u.language,u.last_online,u.tags,o.nick,o.language,o.last_online,o.tags
FROM events AS e INNER JOIN (rooms AS r INNER JOIN users AS o ON o.id=r.owner_id) ON r.id=e.room_id LEFT JOIN users AS u ON u.id=e.user_id
WHERE r.id=? AND e.created >= ? AND e.created < ? ORDER BY e.created, e.created_sort DESC LIMIT ? OFFSET ?;`
	rows, err := p.db.Query(query, room.Id, from, to, maxCount, fromIdx)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var sourceUser, owner types.User
		var sourceUserId, sourceUserNick, sourceUserLanguage sql.NullString
		var newRoom types.Room
		var rawSourceUserTags sql.NullString
		var rawRoomOwnerTags, rawRoomTags, rawEventTags string
		var created, sent int64
		var sourceUserLastOnline sql.NullInt64
		var ownerLastOnline int64
		var event types.Event
		event.Source = &types.Source{}
		err = rows.Scan(&event.Id, &newRoom.Id, &sourceUserId, &event.Source.PluginName, &event.Name, &event.Language, &rawEventTags, &event.TargetFilter, &created, &sent, &owner.Id, &rawRoomTags, &sourceUserNick, &sourceUserLanguage, &sourceUserLastOnline, &rawSourceUserTags, &owner.Nick, &owner.Language, &ownerLastOnline, &rawRoomOwnerTags)
		if err != nil {
			return nil, err
		}
		if sourceUserId.Valid {
			sourceUser.Id = sourceUserId.String
			sourceUser.Nick = sourceUserNick.String
			sourceUser.Language = sourceUserLanguage.String
		}
		sourceUserTags := make(map[string]string)
		_ = json.Unmarshal([]byte(rawSourceUserTags.String), &sourceUserTags)
		sourceUser.Tags = sourceUserTags
		ownerTags := make(map[string]string)
		_ = json.Unmarshal([]byte(rawRoomOwnerTags), &ownerTags)
		owner.Tags = ownerTags
		roomTags := make(map[string]string)
		_ = json.Unmarshal([]byte(rawRoomTags), &roomTags)
		newRoom.Tags = roomTags
		eventTags := make(map[string]string)
		_ = json.Unmarshal([]byte(rawEventTags), &eventTags)
		event.Tags = eventTags
		sourceUser.LastOnline = time.Unix(sourceUserLastOnline.Int64, 0)
		owner.LastOnline = time.Unix(ownerLastOnline, 0)
		newRoom.Owner = &owner
		event.Created = time.Unix(created, 0)
		event.Sent = time.Unix(sent, 0)
		event.Room = &newRoom
		event.Source.User = &sourceUser
		event.History = true
		events = append(events, &event)
	}
	return events, nil
}

func (p *SQLitePersist) Close() error {
	p.RWMutex.Lock()
	defer p.RWMutex.Unlock()
	if p.Flock != nil {
		p.Flock.Lock()
		defer p.Flock.Unlock()
	}
	return p.db.Close()
}
