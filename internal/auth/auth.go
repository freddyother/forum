// internal/auth/auth.go
package auth

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailTaken    = errors.New("email already taken")
	ErrUsernameTaken = errors.New("username already taken")
	ErrInvalidLogin  = errors.New("invalid email or password")
	ErrNoSession     = errors.New("session not found")
)

// ----------------------------
// Context helpers (para middleware y handlers)
// ----------------------------

type ctxKeyUserID struct{}

func WithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, ctxKeyUserID{}, uid)
}

func UserIDFrom(ctx context.Context) (int64, bool) {
	v := ctx.Value(ctxKeyUserID{})
	if v == nil {
		return 0, false
	}
	id, _ := v.(int64)
	return id, id != 0
}

// ----------------------------
// Register
// ----------------------------

func Register(db *sql.DB, email, username, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	username = strings.TrimSpace(username)

	if email == "" || username == "" || password == "" {
		return errors.New("email, username and password are required")
	}
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters")
	}

	// Comprobar duplicados (rápido y con error claro)
	var exists int
	if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE email = ?`, email).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return ErrEmailTaken
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE username = ?`, username).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost) // cost ~10
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO users (email, username, password_hash, created_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		email, username, string(hash),
	)
	// Por si hay condición de carrera con UNIQUE:
	if isUniqueErr(err, "users.email") {
		return ErrEmailTaken
	}
	if isUniqueErr(err, "users.username") {
		return ErrUsernameTaken
	}
	return err
}

// ----------------------------
// Login (crea sesión con UUID y expiración)
// ----------------------------

func Login(db *sql.DB, email, password string, lifetime time.Duration) (string, int64, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	var uid int64
	var passwdHash string

	// 1) Busca el usuario
	err := db.QueryRow(`SELECT id, password_hash FROM users WHERE email = ?`, email).Scan(&uid, &passwdHash)
	if err == sql.ErrNoRows {
		log.Printf("auth.Login: no user for email=%s", email) // ⬅️ log
		return "", 0, ErrInvalidLogin
	}
	if err != nil {
		log.Printf("auth.Login: query user err: %v", err) // ⬅️ log
		return "", 0, err
	}

	// 2) Verifica contraseña
	if err := bcrypt.CompareHashAndPassword([]byte(passwdHash), []byte(password)); err != nil {
		log.Printf("auth.Login: bad password for email=%s", email) // ⬅️ log (no revela hash)
		return "", 0, ErrInvalidLogin
	}

	// 3) Crea sesión dentro de transacción
	tx, err := db.Begin()
	if err != nil {
		log.Printf("auth.Login: begin tx err: %v", err) // ⬅️ log
		return "", 0, err
	}
	defer tx.Rollback()

	// (opcional) Limpia sesiones antiguas del usuario
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, uid); err != nil {
		log.Printf("auth.Login: delete old sessions err: %v", err) // ⬅️ log
		return "", 0, err
	}

	sid := uuid.New().String()
	exp := time.Now().Add(lifetime)

	if _, err := tx.Exec(`
        INSERT INTO sessions (id, user_id, expires_at, created_at)
        VALUES (?, ?, ?, CURRENT_TIMESTAMP)
    `, sid, uid, exp.Format("2006-01-02 15:04:05.999999999-07:00")); err != nil {
		log.Printf("auth.Login: insert session err: %v", err) // ⬅️ log
		return "", 0, err
	}

	if err := tx.Commit(); err != nil {
		log.Printf("auth.Login: commit err: %v", err) // ⬅️ log
		return "", 0, err
	}

	log.Printf("auth.Login: OK email=%s uid=%d sid=%s", email, uid, sid) // ⬅️ opcional
	return sid, uid, nil
}

// ----------------------------
// Logout (borra la sesión por ID)
// ----------------------------

func Logout(db *sql.DB, sid string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, sid)
	return err
}

// ----------------------------
// UserFromSession: valida cookie y devuelve (uid, expires)
// ----------------------------

func UserFromSession(db *sql.DB, sid string) (int64, time.Time, error) {
	var uid int64
	var expRaw string

	err := db.QueryRow(
		`SELECT user_id, expires_at FROM sessions WHERE id = ?`,
		sid,
	).Scan(&uid, &expRaw)

	if err == sql.ErrNoRows {
		return 0, time.Time{}, ErrNoSession
	}
	if err != nil {
		return 0, time.Time{}, err
	}

	// SQLite guarda como TEXT; parseamos formatos habituales
	exp, err := parseSQLiteTime(expRaw)
	if err != nil {
		// intenta como UTC sin nanos: 2006-01-02 15:04:05-07:00
		return uid, time.Time{}, err
	}
	return uid, exp, nil
}

// ----------------------------
// Helpers
// ----------------------------

func isUniqueErr(err error, col string) bool {
	// SQLite: "UNIQUE constraint failed: table.column"
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, strings.ToLower(col))
}

func parseSQLiteTime(s string) (time.Time, error) {
	// ejemplos vistos:
	// "2025-10-01 13:15:02.258078456+01:00"
	// "2025-10-01 13:15:02+01:00"
	// "2025-10-01T13:15:02Z"
	layouts := []string{
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}
	var last error
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		} else {
			last = err
		}
	}
	return time.Time{}, last
}
