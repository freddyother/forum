package db

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

// Open abre la base de datos SQLite con las PRAGMA recomendadas.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	// Importante: reducir contención y evitar bloqueos largos
	_, _ = db.Exec(`PRAGMA journal_mode=WAL;`)  // lectores no bloquean al escritor
	_, _ = db.Exec(`PRAGMA busy_timeout=3000;`) // espera hasta 3s antes de “database is locked”
	_, _ = db.Exec(`PRAGMA foreign_keys=ON;`)   // forzar integridad referencial

	return db, nil
}
