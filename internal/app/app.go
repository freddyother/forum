package app

import (
	"database/sql"
	"log"
	"os"
	"time"
)

type Config struct {
	Addr                string
	DatabaseURL         string
	SessionLifetime     time.Duration
}

func LoadConfig() Config {
	addr := getenv("ADDR", ":8080")
	dbURL := getenv("DATABASE_URL", "./forum.db")
	lifeHours := getenv("SESSION_LIFETIME_HOURS", "24")
	dur, err := time.ParseDuration(lifeHours + "h")
	if err != nil { dur = 24 * time.Hour }
	return Config{
		Addr:            addr,
		DatabaseURL:     dbURL,
		SessionLifetime: dur,
	}
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" { return def }
	return v
}

type App struct {
	DB  *sql.DB
	Cfg Config
}

func Must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
