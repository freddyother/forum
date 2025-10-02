package db

import (
	"database/sql"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx"
)

func Open(dsn string) (*sql.DB, error) {
	// Para esta rama, asumimos Postgres siempre:
	// dsn: postgres://user:pass@host:5432/dbname?sslmode=require
	return sql.Open("pgx", dsn)
}

// Si quisieras autodetectar driver (por si reutilizas el archivo):
func IsPostgres(dsn string) bool {
	ds := strings.ToLower(dsn)
	return strings.HasPrefix(ds, "postgres://") || strings.HasPrefix(ds, "postgresql://")
}
