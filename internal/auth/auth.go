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

/* =========================
   Context helpers
   ========================= */

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

/* =========================
   Register (Postgres)
   ========================= */

func Register(db *sql.DB, email, username, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	username = strings.TrimSpace(username)

	if email == "" || username == "" || password == "" {
		return errors.New("email, username and password are required")
	}
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters")
	}

	// Comprobación rápida para mensaje amable
	var exists int
	if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE email = $1`, email).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return ErrEmailTaken
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE username = $1`, username).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return ErrUsernameTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO users (email, username, password_hash, created_at)
		VALUES ($1, $2, $3, NOW())
	`, email, username, string(hash))

	// Por carrera con UNIQUE, mapea al error amigable
	if isPgUniqueErr(err, "users_email_key") {
		return ErrEmailTaken
	}
	if isPgUniqueErr(err, "users_username_key") {
		return ErrUsernameTaken
	}
	return err
}

/* =========================
   Login (crea sesión UUID)
   ========================= */

func Login(db *sql.DB, email, password string, lifetime time.Duration) (string, int64, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	var uid int64
	var passwdHash string

	// 1) Busca el usuario
	err := db.QueryRow(`SELECT id, password_hash FROM users WHERE email = $1`, email).Scan(&uid, &passwdHash)
	if err == sql.ErrNoRows {
		log.Printf("auth.Login: no user for email=%s", email)
		return "", 0, ErrInvalidLogin
	}
	if err != nil {
		log.Printf("auth.Login: query user err: %v", err)
		return "", 0, err
	}

	// 2) Verifica contraseña
	if err := bcrypt.CompareHashAndPassword([]byte(passwdHash), []byte(password)); err != nil {
		log.Printf("auth.Login: bad password for email=%s", email)
		return "", 0, ErrInvalidLogin
	}

	// 3) Crea sesión
	tx, err := db.Begin()
	if err != nil {
		log.Printf("auth.Login: begin tx err: %v", err)
		return "", 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = $1`, uid); err != nil {
		log.Printf("auth.Login: delete old sessions err: %v", err)
		return "", 0, err
	}

	sid := uuid.New().String()
	exp := time.Now().Add(lifetime)

	if _, err := tx.Exec(`
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES ($1, $2, $3, NOW())
	`, sid, uid, exp); err != nil { // ⬅️ pasa time.Time, no string
		log.Printf("auth.Login: insert session err: %v", err)
		return "", 0, err
	}

	if err := tx.Commit(); err != nil {
		log.Printf("auth.Login: commit err: %v", err)
		return "", 0, err
	}

	log.Printf("auth.Login: OK email=%s uid=%d sid=%s", email, uid, sid)
	return sid, uid, nil
}

/* =========================
   Logout
   ========================= */

func Logout(db *sql.DB, sid string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = $1`, sid)
	return err
}

/* =========================
   UserFromSession
   ========================= */

func UserFromSession(db *sql.DB, sid string) (int64, time.Time, error) {
	var uid int64
	var exp time.Time

	err := db.QueryRow(
		`SELECT user_id, expires_at FROM sessions WHERE id = $1`,
		sid,
	).Scan(&uid, &exp)

	if err == sql.ErrNoRows {
		return 0, time.Time{}, ErrNoSession
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	return uid, exp, nil
}

/* =========================
   Helpers
   ========================= */

// Detecta UNIQUE de Postgres por nombre de constraint en el mensaje.
// Con UNIQUE implícitos, PG usa nombres tipo: users_email_key, users_username_key.
func isPgUniqueErr(err error, constraint string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key value violates unique constraint") &&
		strings.Contains(msg, strings.ToLower(constraint))
}
