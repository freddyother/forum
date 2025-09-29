package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrEmailTaken = errors.New("email already taken")
var ErrUsernameTaken = errors.New("username already taken")
var ErrBadCredentials = errors.New("invalid credentials")

func Register(db *sql.DB, email, username, password string) error {
	// unique email/username
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE email=?`, email).Scan(&n); err != nil { return err }
	if n > 0 { return ErrEmailTaken }
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, username).Scan(&n); err != nil { return err }
	if n > 0 { return ErrUsernameTaken }

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil { return err }
	_, err = db.Exec(`INSERT INTO users (email, username, password_hash) VALUES (?,?,?)`, email, username, string(hash))
	return err
}

func Login(db *sql.DB, email, password string, lifetime time.Duration) (string, int64, error) {
	var id int64
	var hash string
	err := db.QueryRow(`SELECT id, password_hash FROM users WHERE email=?`, email).Scan(&id, &hash)
	if err != nil { 
		if err == sql.ErrNoRows { return "", 0, ErrBadCredentials }
		return "", 0, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", 0, ErrBadCredentials
	}
	sid := uuid.NewString()
	expires := time.Now().Add(lifetime)
	_, err = db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?,?,?)`, sid, id, expires)
	return sid, id, err
}

func Logout(db *sql.DB, sessionID string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id=?`, sessionID)
	return err
}

type userKey struct{}
func WithUserID(ctx context.Context, uid int64) context.Context { return context.WithValue(ctx, userKey{}, uid) }
func UserIDFrom(ctx context.Context) (int64, bool) {
	v := ctx.Value(userKey{})
	if v==nil { return 0, false }
	id, ok := v.(int64)
	return id, ok
}

func UserFromSession(db *sql.DB, sessionID string) (int64, time.Time, error) {
	var uid int64
	var exp time.Time
	err := db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE id=?`, sessionID).Scan(&uid, &exp)
	if err != nil { return 0, time.Time{}, err }
	return uid, exp, nil
}
