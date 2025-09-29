package db

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_fk=1")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite
	return db, db.Ping()
}
