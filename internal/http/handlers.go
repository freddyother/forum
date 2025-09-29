package httpx

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"forum/internal/app"
	"forum/internal/auth"
	"forum/internal/util"
)

type Server struct {
	DB  *sql.DB
	Cfg app.Config
	Mux *http.ServeMux
}

func NewServer(db *sql.DB, cfg app.Config) *Server {
	s := &Server{DB: db, Cfg: cfg, Mux: http.NewServeMux()}
	fs := http.FileServer(http.Dir("web/static"))
	s.Mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// routes
	s.Mux.Handle("/", s.withSession(http.HandlerFunc(s.handleIndex)))
	s.Mux.Handle("/register", s.withSession(http.HandlerFunc(s.handleRegister)))
	s.Mux.Handle("/login", s.withSession(http.HandlerFunc(s.handleLogin)))
	s.Mux.Handle("/logout", s.withSession(http.HandlerFunc(s.handleLogout)))

	s.Mux.Handle("/post/new", s.withSession(s.requireAuth(http.HandlerFunc(s.handlePostNew))))
	s.Mux.Handle("/post/create", s.withSession(s.requireAuth(http.HandlerFunc(s.handlePostCreate))))
	s.Mux.Handle("/comment/create", s.withSession(s.requireAuth(http.HandlerFunc(s.handleCommentCreate))))
	s.Mux.Handle("/react", s.withSession(s.requireAuth(http.HandlerFunc(s.handleReact))))

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.Mux.ServeHTTP(w, r) }

type pageData struct {
	Title      string
	Flash      string
	UserID     int64
	Username   string
	Categories []catVM
	Posts      []postVM
	Filters    struct{
		Category string
		Mine     bool
		Liked    bool
	}
}

type catVM struct{ ID int64; Name string }
type postVM struct{
	ID int64; Title, Content, Author string; Created string
	Likes, Dislikes int
	Cats []string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	var uid int64
	if id, ok := auth.UserIDFrom(r.Context()); ok { uid = id }
	qCat := r.URL.Query().Get("cat")
	qMine := r.URL.Query().Has("mine")
	qLiked := r.URL.Query().Has("liked")

	// categories
	rows, _ := s.DB.Query(`SELECT id, name FROM categories ORDER BY name`)
	var cats []catVM
	for rows.Next() {
		var c catVM
		_ = rows.Scan(&c.ID, &c.Name)
		cats = append(cats, c)
	}
	_ = rows.Close()

	// posts with filters
	var args []any
	sb := strings.Builder{}
	sb.WriteString(`
SELECT p.id, p.title, p.content, u.username,
       IFNULL(SUM(CASE WHEN r.value=1 THEN 1 END),0) as likes,
       IFNULL(SUM(CASE WHEN r.value=-1 THEN 1 END),0) as dislikes,
       p.created_at
FROM posts p
JOIN users u ON u.id=p.user_id
LEFT JOIN reactions r ON r.target_type='post' AND r.target_id=p.id
`)
	if qCat != "" || qMine || qLiked {
		sb.WriteString("WHERE 1=1 ")
	}
	if qCat != "" {
		sb.WriteString(`
 AND EXISTS (SELECT 1 FROM post_categories pc
             JOIN categories c ON c.id=pc.category_id
             WHERE pc.post_id=p.id AND c.name=?)
`)
		args = append(args, qCat)
	}
	if qMine && uid != 0 {
		sb.WriteString(" AND p.user_id=? ")
		args = append(args, uid)
	}
	if qLiked && uid != 0 {
		sb.WriteString(`
 AND EXISTS (SELECT 1 FROM reactions rx
             WHERE rx.user_id=? AND rx.target_type='post' AND rx.target_id=p.id AND rx.value=1)
`)
		args = append(args, uid)
	}
	sb.WriteString(" GROUP BY p.id ORDER BY p.created_at DESC LIMIT 100 ")

	rows2, err := s.DB.Query(sb.String(), args...)
	if err != nil { http.Error(w, err.Error(), 500); return }
	var posts []postVM
	for rows2.Next() {
		var p postVM
		var created time.Time
		if err := rows2.Scan(&p.ID,&p.Title,&p.Content,&p.Author,&p.Likes,&p.Dislikes,&created); err==nil {
			p.Created = created.Format("2006-01-02 15:04")
			// categories for post
			rc, _ := s.DB.Query(`SELECT c.name FROM post_categories pc JOIN categories c ON c.id=pc.category_id WHERE pc.post_id=? ORDER BY c.name`, p.ID)
			for rc.Next() { var n string; _=rc.Scan(&n); p.Cats = append(p.Cats, n) }
			_ = rc.Close()
			posts = append(posts, p)
		}
	}
	_ = rows2.Close()

	var data pageData
	data.Title = "Forum"
	data.UserID = uid
	data.Categories = cats
	data.Posts = posts
	data.Filters.Category = qCat
	data.Filters.Mine = qMine
	data.Filters.Liked = qLiked

	util.Render(w, "index.html", data)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		util.Render(w, "auth_register.html", nil); return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if email=="" || username=="" || password=="" {
		http.Error(w, "Missing fields", http.StatusBadRequest); return
	}
	if err := auth.Register(s.DB, email, username, password); err != nil {
		if errors.Is(err, auth.ErrEmailTaken) || errors.Is(err, auth.ErrUsernameTaken) {
			http.Error(w, err.Error(), http.StatusConflict); return
		}
		http.Error(w, err.Error(), 500); return
	}
	http.Redirect(w, r, "/login?ok=1", http.StatusSeeOther)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		util.Render(w, "auth_login.html", map[string]any{"OK": r.URL.Query().Get("ok")=="1"}); return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	sid, uid, err := auth.Login(s.DB, email, password, s.Cfg.SessionLifetime)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized); return
	}
	c := &http.Cookie{
		Name: CookieName, Value: sid, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(s.Cfg.SessionLifetime),
	}
	http.SetCookie(w, c)
	_ = uid // reserved if you want to redirect by user
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = auth.Logout(s.DB, c.Value)
		c.MaxAge = -1
		http.SetCookie(w, c)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handlePostNew(w http.ResponseWriter, r *http.Request) {
	// list categories
	rows, _ := s.DB.Query(`SELECT id,name FROM categories ORDER BY name`)
	var cats []catVM
	for rows.Next() { var c catVM; _=rows.Scan(&c.ID,&c.Name); cats=append(cats,c) }
	_ = rows.Close()
	util.Render(w, "post_new.html", map[string]any{"Cats": cats})
}

func (s *Server) handlePostCreate(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	cats := r.Form["cats"] // category names

	if title=="" || content=="" {
		http.Error(w, "Title and content required", http.StatusBadRequest); return
	}
	res, err := s.DB.Exec(`INSERT INTO posts (user_id,title,content) VALUES (?,?,?)`, uid, title, content)
	if err != nil { http.Error(w, err.Error(), 500); return }
	pid, _ := res.LastInsertId()

	// attach categories by name; create if missing
	for _, name := range cats {
		name = strings.TrimSpace(name)
		if name == "" { continue }
		var id int64
		err = s.DB.QueryRow(`SELECT id FROM categories WHERE name=?`, name).Scan(&id)
		if err == sql.ErrNoRows {
			rx, e2 := s.DB.Exec(`INSERT INTO categories (name) VALUES (?)`, name)
			if e2==nil { id, _ = rx.LastInsertId() }
		} else if err != nil { continue }
		if id != 0 {
			_, _ = s.DB.Exec(`INSERT OR IGNORE INTO post_categories (post_id,category_id) VALUES (?,?)`, pid, id)
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleCommentCreate(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	pid, _ := strconv.ParseInt(r.FormValue("post_id"), 10, 64)
	content := strings.TrimSpace(r.FormValue("content"))
	if pid==0 || content=="" {
		http.Error(w, "Bad request", http.StatusBadRequest); return
	}
	_, err := s.DB.Exec(`INSERT INTO comments (post_id,user_id,content) VALUES (?,?,?)`, pid, uid, content)
	if err != nil { http.Error(w, err.Error(), 500); return }
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	target := r.FormValue("target") // "post" or "comment"
	id, _ := strconv.ParseInt(r.FormValue("id"),10,64)
	val, _ := strconv.Atoi(r.FormValue("value")) // 1 or -1
	if (target!="post" && target!="comment") || (val!=1 && val!=-1) || id==0 {
		http.Error(w, "Bad request", http.StatusBadRequest); return
	}
	_, err := s.DB.Exec(`
INSERT INTO reactions (user_id,target_type,target_id,value) VALUES (?,?,?,?)
ON CONFLICT(user_id,target_type,target_id) DO UPDATE SET value=excluded.value
`, uid, target, id, val)
	if err != nil { http.Error(w, err.Error(), 500); return }
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// helper used in templates
func joinCats(cs []string) string {
	return strings.Join(cs, ", ")
}
var _ = fmt.Sprintf
