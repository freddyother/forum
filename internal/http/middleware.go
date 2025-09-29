package httpx

import (
	"net/http"
	"time"

	"forum/internal/auth"
)

const CookieName = "session_id"

func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err == nil && c.Value != "" {
			if uid, exp, err := auth.UserFromSession(s.DB, c.Value); err == nil && exp.After(time.Now()) {
				r = r.WithContext(auth.WithUserID(r.Context(), uid))
			}
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
