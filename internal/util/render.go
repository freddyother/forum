package util

import (
	"html/template"
	"net/http"
	"path/filepath"
)

func Render(w http.ResponseWriter, name string, data any) {
	files := []string{
		filepath.Join("web", "templates", "layout.html"),
		filepath.Join("web", "templates", name),
	}
	t := template.Must(template.ParseFiles(files...))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = t.ExecuteTemplate(w, "base", data)
}
