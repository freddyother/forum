package main

import (
	"log"
	"net/http"

	"forum/internal/app"
	"forum/internal/db"
	httpx "forum/internal/http"
)

func main() {
	cfg := app.LoadConfig()
	d, err := db.Open(cfg.DatabaseURL)
	app.Must(err)
	app.Must(db.Migrate(d, "schema.sql"))

	srv := httpx.NewServer(d, cfg)
	log.Printf("listening on %s", cfg.Addr)
	app.Must(http.ListenAndServe(cfg.Addr, srv))
}
