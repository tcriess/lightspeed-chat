package persistence

import (
	"fmt"
	"testing"

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
	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{})
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
}
