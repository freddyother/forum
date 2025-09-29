package models

import "time"

type User struct {
	ID           int64
	Email        string
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Category struct {
	ID   int64
	Name string
}

type Post struct {
	ID        int64
	UserID    int64
	Title     string
	Content   string
	CreatedAt time.Time
	Cats      []Category
	Likes     int
	Dislikes  int
	Author    string
}

type Comment struct {
	ID        int64
	PostID    int64
	UserID    int64
	Content   string
	CreatedAt time.Time
	Author    string
	Likes     int
	Dislikes  int
}
