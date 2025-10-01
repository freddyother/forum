package main

import (
	"log"
	"net/http"
	"time"

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

	// ⚙️ Encadena middlewares a nivel de servidor
	var handler http.Handler = srv
	handler = httpx.WithTimeout(handler)   // ✅
	handler = httpx.WithAccessLog(handler) // ✅

	server := &http.Server{
		Addr:              cfg.Addr, // p.ej. ":8080"
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("listening on %s", cfg.Addr)
	app.Must(server.ListenAndServe())
}
