package model

import (
	"github.com/glebarez/sqlite"
	"github.com/sqids/sqids-go"
	"gorm.io/gorm"
)

var sqidGen *sqids.Sqids

var DB *gorm.DB

func init() {
	var err error
	sqidGen, err = sqids.New(sqids.Options{Alphabet: "abcdefghikmnpqrstuvwxyz1234567890", MinLength: 5})
	if err != nil {
		panic(err)
	}
}

func InitDB(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(
		&Infraction{},
		&GuildSettings{},
	)
	if err != nil {
		return nil, err
	}

	DB = db
	return db, nil
}
