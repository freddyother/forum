# 🗨️ Forum Project

## Fredy Gallegos

## Intro

A simple **web forum** written in **Go** with **SQLite**, featuring:

- User registration and login (with session cookies based on UUIDs).
- Creating posts and comments.
- Assigning categories to posts.
- Likes and dislikes on posts and comments.
- Filtering posts (by category, by author, by likes).
- Public read access (non-registered users can view content).
- Runs locally or inside a Docker container.

---

## 📦 Tech Stack

- **Go 1.22+**
- **SQLite3** (embedded database) migrate to **Postgres**
- **bcrypt** (password hashing)
- **UUID** (`github.com/google/uuid`)
- **HTML + CSS + JS** (no frontend frameworks)
- **Docker & Docker Compose**

---
## 🗄️ Project Structure
```text
forum/
├─ cmd/server/         # main.go (server entrypoint)
├─ internal/           # application logic by package
│  ├─ app/             # configuration
│  ├─ auth/            # registration, login, sessions
│  ├─ db/              # SQLite connection and migrations
│  ├─ http/            # handlers and middleware
│  ├─ models/          # data models
│  └─ util/            # helpers (templates, rendering)
├─ web/
│  ├─ templates/       # HTML views
│  └─ static/          # CSS and JS
├─ schema.sql          # SQLite schema (CREATE + seed data)
├─ Dockerfile          # build and runtime container
├─ docker-compose.yml  # stack with persistent volume
├─ Makefile            # shortcuts (run, build, test, docker-up…)
├─ .env.example        # configuration template
├─ .gitignore
└─ README.md


```
---

## 🧪 Tests

go test ./...

---

## 🔒 Security

Passwords hashed with bcrypt.

Session cookies include:

HttpOnly

SameSite=Lax

Expiry configurable (SESSION_LIFETIME_HOURS).

SQLite initialised with PRAGMA foreign_keys=ON.

---

## 🚀 Roadmap

✅ Pagination for post listings.

✅ Edit / delete posts and comments.

🔜 Improved error messages and form validation.

🔜 Internationalisation (English/Spanish).

🔜 CI/CD with automated testing.

---
## 📜 Licence

This project was built for educational purposes as part of 01Founders coursework.
You are welcome to adapt or extend it for your own use.

---
## ⚙️ Installation & Usage

### 1. Clone the repository

```bash
git clone https://learn.01founders.co/git/fgallego/forum.git
cd forum

##Copy .env.example to .env and adjust values as required:
cp .env.example .env

## Available variables:
ADDR=:8080
DATABASE_URL=./forum.db
SESSION_LIFETIME_HOURS=24

## 3. Run locally
go mod tidy
go run ./cmd/server

## 4. Run with Docker
docker compose up --build

Open in your browser: http://localhost:8080

```
---