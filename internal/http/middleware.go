package httpx

import (
	"log"
	"net/http"
	"time"

	"forum/internal/auth"
)

const CookieName = "session_id"

func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Lee la cookie
		if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
			// Valida la sesión en BD
			if uid, exp, err2 := auth.UserFromSession(s.DB, c.Value); err2 == nil && exp.After(time.Now()) {
				// OK: inyecta uid en el contexto y loguea
				r = r.WithContext(auth.WithUserID(r.Context(), uid))
				log.Printf("session OK sid=%s uid=%d exp=%s", c.Value, uid, exp.Format(time.RFC3339))
			} else {
				// Sesión inválida/expirada
				log.Printf("session FAIL sid=%s err=%v", c.Value, err2)
			}
		} else {
			// No hay cookie de sesión (navegación anónima)
			// log.Printf("no session cookie: %v", err) // opcional
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.UserIDFrom(r.Context()); !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ——— access log ———

type statusRW struct {
	http.ResponseWriter
	status int
}

func (w *statusRW) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// WithAccessLog envuelve un handler y loguea METHOD PATH -> STATUS (duración)
func WithAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRW{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start).Truncate(time.Millisecond))
	})
}

// WithTimeout aplica un timeout de 5s a la request completa
func WithTimeout(next http.Handler) http.Handler {
	return http.TimeoutHandler(next, 5*time.Second, "request timeout")
}
