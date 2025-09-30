package util

import (
	"html/template"
	"net/http"
	"path/filepath"
)

func Render(w http.ResponseWriter, name string, data any) {
	layout := filepath.Join("web", "templates", "layout.html")
	flash := filepath.Join("web", "templates", "_flash.html")
	view := filepath.Join("web", "templates", name)

	t, err := template.ParseFiles(layout, flash, view)
	if err != nil {
		http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template exec error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
