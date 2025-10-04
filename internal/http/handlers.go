package httpx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	s.Mux.Handle("/forgot", s.withSession(http.HandlerFunc(s.handleForgot)))

	s.Mux.Handle("/post/new", s.withSession(s.requireAuth(http.HandlerFunc(s.handlePostNew))))
	s.Mux.Handle("/post/create", s.withSession(s.requireAuth(http.HandlerFunc(s.handlePostCreate))))
	s.Mux.Handle("/comment/create", s.withSession(s.requireAuth(http.HandlerFunc(s.handleCommentCreate))))
	s.Mux.Handle("/react", s.withSession(s.requireAuth(http.HandlerFunc(s.handleReact))))

	s.Mux.Handle("/debug/me", s.withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if uid, ok := auth.UserIDFrom(r.Context()); ok {
			w.Write([]byte(fmt.Sprintf("logged uid=%d", uid)))
			return
		}
		w.Write([]byte("anon"))
	})))

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.Mux.ServeHTTP(w, r) }

type pageData struct {
	Title      string
	Flash      string
	UserID     int64
	Username   string
	UserInitial string
	Categories []catVM
	Posts      []postVM
	Filters    struct {
		Category string
		Mine     bool
		Liked    bool
	}
    FlashOK bool //  true = éxito, false = error
}

type catVM struct {
	ID   int64
	Name string
}
type commentVM struct {
	ID      int64
	Author  string
	Content string
	Created string
}
type postVM struct {
	ID                     int64
	Title, Content, Author string
	Created                string
	Likes, Dislikes        int
	Cats                   []string
	Comments               []commentVM // ⬅️ nuevo
}

// ------------------------------------------------------------------------------
// ------------HandlerIndex Function---------------------------------------------
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Contexto con timeout para TODA la carga de la página
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var uid int64
	if id, ok := auth.UserIDFrom(r.Context()); ok {
		uid = id
	}

	qCat := r.URL.Query().Get("cat")
	qMine := r.URL.Query().Has("mine")
	qLiked := r.URL.Query().Has("liked")

	// ---------------------------
	// Cargar categorías
	// ---------------------------
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name FROM categories ORDER BY name`)
	if err != nil {
		http.Error(w, "categories query: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var cats []catVM
	for rows.Next() {
		var c catVM
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			_ = rows.Close()
			http.Error(w, "categories scan: "+err.Error(), http.StatusInternalServerError)
			return
		}
		cats = append(cats, c)
	}
	if err := rows.Close(); err != nil {
		http.Error(w, "categories close: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "categories err: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// ---------------------------
	// Query de posts + filtros (PostgreSQL)
	// ---------------------------
	var (
		args []any
		sb   strings.Builder
	)
	// helper para numerar $1, $2, ...
	nextArg := func() string { return fmt.Sprintf("$%d", len(args)+1) }

	sb.WriteString(`
SELECT
  p.id, p.title, p.content, u.username,
  COUNT(*) FILTER (WHERE r.value = 1)  AS likes,
  COUNT(*) FILTER (WHERE r.value = -1) AS dislikes,
  p.created_at
FROM posts p
JOIN users u ON u.id = p.user_id
LEFT JOIN reactions r
  ON r.target_type = 'post'
 AND r.target_id  = p.id
`)

	if qCat != "" || qMine || qLiked {
		sb.WriteString("WHERE 1=1 ")
	}
	if qCat != "" {
		sb.WriteString(`
  AND EXISTS (
        SELECT 1
          FROM post_categories pc
          JOIN categories c ON c.id = pc.category_id
         WHERE pc.post_id = p.id
           AND c.name = ` + nextArg() + `
      )
`)
		args = append(args, qCat)
	}
	if qMine && uid != 0 {
		sb.WriteString("  AND p.user_id = " + nextArg() + " ")
		args = append(args, uid)
	}
	if qLiked && uid != 0 {
		sb.WriteString(`
  AND EXISTS (
        SELECT 1
          FROM reactions rx
         WHERE rx.user_id     = ` + nextArg() + `
           AND rx.target_type = 'post'
           AND rx.target_id   = p.id
           AND rx.value       = 1
      )
`)
		args = append(args, uid)
	}

	// En Postgres deben agruparse TODOS los no agregados
	sb.WriteString(`
GROUP BY p.id, p.title, p.content, u.username, p.created_at
ORDER BY p.created_at DESC
LIMIT 100
`)

	rows2, err := s.DB.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		http.Error(w, "posts query: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var posts []postVM
	for rows2.Next() {
		var p postVM
		var created time.Time
		if err := rows2.Scan(&p.ID, &p.Title, &p.Content, &p.Author, &p.Likes, &p.Dislikes, &created); err != nil {
			_ = rows2.Close()
			http.Error(w, "posts scan: "+err.Error(), http.StatusInternalServerError)
			return
		}
		p.Created = created.Format("2006-01-02 15:04")

		// Categorías del post
		rc, err := s.DB.QueryContext(ctx, `
SELECT c.name
  FROM post_categories pc
  JOIN categories c ON c.id = pc.category_id
 WHERE pc.post_id = $1
 ORDER BY c.name
`, p.ID)
		if err != nil {
			_ = rows2.Close()
			http.Error(w, "post categories query: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for rc.Next() {
			var n string
			if err := rc.Scan(&n); err != nil {
				_ = rc.Close()
				_ = rows2.Close()
				http.Error(w, "post categories scan: "+err.Error(), http.StatusInternalServerError)
				return
			}
			p.Cats = append(p.Cats, n)
		}
		if err := rc.Close(); err != nil {
			_ = rows2.Close()
			http.Error(w, "post categories close: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := rc.Err(); err != nil {
			_ = rows2.Close()
			http.Error(w, "post categories err: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Comentarios del post
		rcm, err := s.DB.QueryContext(ctx, `
SELECT c.id, u.username, c.content, c.created_at
  FROM comments c
  JOIN users u ON u.id = c.user_id
 WHERE c.post_id = $1
 ORDER BY c.created_at ASC
`, p.ID)
		if err != nil {
			_ = rows2.Close()
			http.Error(w, "comments query: "+err.Error(), http.StatusInternalServerError)
			return
		}
		for rcm.Next() {
			var cm commentVM
			var ctime time.Time
			if err := rcm.Scan(&cm.ID, &cm.Author, &cm.Content, &ctime); err != nil {
				_ = rcm.Close()
				_ = rows2.Close()
				http.Error(w, "comments scan: "+err.Error(), http.StatusInternalServerError)
				return
			}
			cm.Created = ctime.Format("2006-01-02 15:04")
			p.Comments = append(p.Comments, cm)
		}
		if err := rcm.Close(); err != nil {
			_ = rows2.Close()
			http.Error(w, "comments close: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := rcm.Err(); err != nil {
			_ = rows2.Close()
			http.Error(w, "comments err: "+err.Error(), http.StatusInternalServerError)
			return
		}

		posts = append(posts, p)
	}
	if err := rows2.Close(); err != nil {
		http.Error(w, "posts close: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := rows2.Err(); err != nil {
		http.Error(w, "posts err: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// ---------------------------
	// Render
	// ---------------------------
	var data pageData
	data.Title = "Forum"
	data.UserID = uid
	data.Categories = cats
	data.Posts = posts
	data.Filters.Category = qCat
	data.Filters.Mine = qMine
	data.Filters.Liked = qLiked

	if r.URL.Query().Get("ok") == "1" {
		data.Flash = "Post created successfully"
		data.FlashOK = true
	}
	if r.URL.Query().Get("err") != "" {
		data.Flash = r.URL.Query().Get("err")
		data.FlashOK = false
	}

	s.fillUserMeta(r.Context(), &data) // 👈 añade Username e inicial si hay sesión
	util.Render(w, "index.html", data)
}

//---------------------------------------------------------------------------------
//------------HandleRegistre Function-----------------------------------------------

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		util.Render(w, "auth_register.html", map[string]any{
			"Error":    r.URL.Query().Get("err"),
			"Email":    r.URL.Query().Get("email"),
			"Username": r.URL.Query().Get("username"),
		})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if email == "" || username == "" || password == "" {
		http.Redirect(w, r, "/register?err=Missing+fields&email="+url.QueryEscape(email)+"&username="+url.QueryEscape(username), http.StatusSeeOther)
		return
	}

	if err := auth.Register(s.DB, email, username, password); err != nil {
		msg := "Internal+error"
		if errors.Is(err, auth.ErrEmailTaken) {
			msg = "Email+already+taken"
		} else if errors.Is(err, auth.ErrUsernameTaken) {
			msg = "Username+already+taken"
		}
		http.Redirect(w, r, "/register?err="+msg+"&email="+url.QueryEscape(email)+"&username="+url.QueryEscape(username), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/login?ok=1", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------------
// ------------HandleForgot Function-----------------------------------------------
func (s *Server) handleForgot(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		util.Render(w, "auth_forgot.html", nil)
		return
	}
	// POST: no revelar si el email existe (buena práctica)
	_ = strings.TrimSpace(r.FormValue("email"))
	// Aquí en el futuro: generar token, guardar y enviar email.
	http.Redirect(w, r, "/login?reset=1", http.StatusSeeOther)
}

//---------------------------------------------------------------------------------
//------------HandleLogin Function-----------------------------------------------

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		util.Render(w, "auth_login.html", map[string]any{
			"OK":    r.URL.Query().Get("ok") == "1",
			"Reset": r.URL.Query().Get("reset") == "1",
			"Error": r.URL.Query().Get("err"),
			"Email": r.URL.Query().Get("email"),
		})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	sid, uid, err := auth.Login(s.DB, email, password, s.Cfg.SessionLifetime)
	if err != nil {
		// registra el fallo para saber por qué
		log.Printf("login FAIL email=%s err=%v", email, err)
		http.Redirect(w, r, "/login?err=Invalid+email+or+password&email="+url.QueryEscape(email), http.StatusSeeOther)
		return
	}

	// solo aquí el login es OK
	log.Printf("login OK email=%s uid=%d sid=%s", email, uid, sid)

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(s.Cfg.SessionLifetime),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------------
// ------------HandleLogout Function-----------------------------------------------
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		_ = auth.Logout(s.DB, c.Value)
		c.MaxAge = -1
		http.SetCookie(w, c)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------------
// ------------HandlePostNew Function-----------------------------------------------
func (s *Server) handlePostNew(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1) Cargar categorías (con manejo de errores)
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name FROM categories ORDER BY name`)
	if err != nil {
		http.Error(w, "categories query: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var cats []catVM
	for rows.Next() {
		var c catVM
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			http.Error(w, "categories scan: "+err.Error(), http.StatusInternalServerError)
			return
		}
		cats = append(cats, c)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "categories err: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 2) Preparar datos de la página
	var data pageData
	data.Title       = "New Post"
	data.Categories  = cats

	// 3) Completar metadatos de usuario para el layout (UserID/Username/Initial)
	s.fillUserMeta(ctx, &data)

	if r.URL.Query().Get("ok") == "1" {
		data.Flash = "Post created successfully"
		data.FlashOK = true
	}
	if r.URL.Query().Get("err") != "" {
		data.Flash = r.URL.Query().Get("err")
		data.FlashOK = false
	}

	util.Render(w, "post_new.html", data)
}

// ---------------------------------------------------------------------------------
// ------------HandlePost Create Function-----------------------------------------------
// handlers.go
func (s *Server) handlePostCreate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	// Asegura parseo del form (seguro incluso si ya se parseó)
    if err := r.ParseForm(); err != nil {
        http.Redirect(w, r, "/post/new?err="+url.QueryEscape("bad form: "+err.Error()), http.StatusSeeOther)
        return
    }

	uid, _ := auth.UserIDFrom(r.Context())
	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	cats := r.Form["cats"] // <select multiple name="cats"> o checkboxes name="cats"
	newCat := strings.TrimSpace(r.FormValue("newcat")) // input de “nueva categoría”

	if title == "" || content == "" {
		http.Error(w, "Title and content required", http.StatusBadRequest)
		return
	}
	if len(cats) == 0 && newCat == "" {
        http.Redirect(w, r, "/post/new?err=Please pick at least one category", http.StatusSeeOther)
        return
    }

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// 1) Crear post y obtener id (PG: RETURNING)
	var pid int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO posts (user_id, title, content)
         VALUES ($1,$2,$3)
         RETURNING id`,
		uid, title, content,
	).Scan(&pid); err != nil {
		http.Error(w, "db error: "+err.Error(), http.StatusInternalServerError)
		return
	}
   // 2) Normalizamos el conjunto de categorías (existentes + la nueva si aplica)
   if newCat != "" {
	cats = append(cats, newCat)
}
	// 3) Asegurar categorías y vincular (PG: ON CONFLICT DO NOTHING)
	for _, name := range cats {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		var cid int64
		// Intentamos insertar; si ya existe, no devuelve fila (ErrNoRows).
		err := tx.QueryRowContext(ctx, `
			INSERT INTO categories (name)
			VALUES ($1)
			ON CONFLICT (name) DO NOTHING
			RETURNING id
		`, name).Scan(&cid)

		if err == sql.ErrNoRows {
			// Ya existía: recupera su id
			if e2 := tx.QueryRowContext(ctx, `SELECT id FROM categories WHERE name=$1`, name).Scan(&cid); e2 != nil {
				// si no podemos obtenerla, saltamos esta categoría
				log.Printf("skip category %q: %v", name, e2)
				continue
			}
		} else if err != nil {
			// error real al intentar crear categoría
			log.Printf("insert category %q err: %v", name, err)
			continue
		}

		// Vincular post-categoría; PK (post_id,category_id) evita duplicados
		if _, e3 := tx.ExecContext(ctx, `
            INSERT INTO post_categories (post_id, category_id)
            VALUES ($1,$2)
            ON CONFLICT DO NOTHING
        `, pid, cid); e3 != nil {
            log.Printf("link post-category err: %v", e3)
        }
	}

	if err := tx.Commit(); err != nil {
        http.Redirect(w, r, "/post/new?err="+url.QueryEscape(err.Error()), http.StatusSeeOther)
        return
    }

	log.Printf("create post uid=%d title=%q cats=%v", uid, title, cats)
	http.Redirect(w, r, "/?ok=1", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------------
// ------------HandleComment create Function-----------------------------------------------
func (s *Server) handleCommentCreate(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	pid, _ := strconv.ParseInt(r.FormValue("post_id"), 10, 64)
	content := strings.TrimSpace(r.FormValue("content"))
	if pid == 0 || content == "" {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	_, err := s.DB.Exec(`INSERT INTO comments (post_id,user_id,content) VALUES ($1,$2,$3)`, pid, uid, content)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ---------------------------------------------------------------------------------
// ------------HandleReact Function-----------------------------------------------
func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	target := r.FormValue("target") // "post" or "comment"
	id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
	val, _ := strconv.Atoi(r.FormValue("value")) // 1 or -1
	if (target != "post" && target != "comment") || (val != 1 && val != -1) || id == 0 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	_, err := s.DB.Exec(`
INSERT INTO reactions (user_id,target_type,target_id,value) VALUES ($1,$2,$3,$4)
ON CONFLICT(user_id,target_type,target_id) DO UPDATE SET value=excluded.value
`, uid, target, id, val)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
//--------------------------------------------------------------------------------------
//--------------fillUserMeta Function helper-------------------------------------------

func (s *Server) fillUserMeta(ctx context.Context, data *pageData) {
    if uid, ok := auth.UserIDFrom(ctx); ok && uid != 0 {
        data.UserID = uid

        var name string
        // Postgres
        _ = s.DB.QueryRowContext(ctx, `SELECT username FROM users WHERE id = $1`, uid).Scan(&name)

        if name != "" {
            data.Username = name
            r := []rune(name)
            data.UserInitial = strings.ToUpper(string(r[0]))
        }
    }
}

// helper used in templates
func joinCats(cs []string) string {
	return strings.Join(cs, ", ")
}

var _ = fmt.Sprintf
