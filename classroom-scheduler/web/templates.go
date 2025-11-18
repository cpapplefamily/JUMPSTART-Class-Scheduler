// web/templates.go
package web

import (
	"html/template"
	"log"
	"net/http"

)


var funcMap = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"seq": func(n int) []int {
		s := make([]int, n)
		for i := range s {
			s[i] = i + 1
		}
		return s
	},
}

// This function parses templates fresh every request → instant live reload!
func RenderTemplate(w http.ResponseWriter, name string, data any) {
	// Always parse from disk → no caching
	tmpl := template.New("").Funcs(funcMap)
	tmpl, err := tmpl.ParseGlob("templates/*.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), 500)
		log.Println("Template parse error:", err)
		return
	}
	err = tmpl.ExecuteTemplate(w, name, data)
	if err != nil {
		http.Error(w, "Template exec error: "+err.Error(), 500)
		log.Println("Template exec error:", err)
	}
}