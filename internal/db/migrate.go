package db

import (
	"database/sql"
	"os"
)

func Migrate(db *sql.DB, schemaPath string) error {
	b, err := os.ReadFile(schemaPath)
	if err != nil { return err }
	_, err = db.Exec(string(b))
	return err
}
