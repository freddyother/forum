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
- **SQLite3** (embedded database)
- **bcrypt** (password hashing)
- **UUID** (`github.com/google/uuid`)
- **HTML + CSS + JS** (no frontend frameworks)
- **Docker & Docker Compose**

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