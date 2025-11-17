// web/templates.go
package web

import (
	"html/template"
	"log"
)

var Templates *template.Template

func InitTemplates() {
	funcMap := template.FuncMap{
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

	var err error
	Templates = template.New("").Funcs(funcMap)
	Templates, err = Templates.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatal("Template parse error:", err)
	}
}