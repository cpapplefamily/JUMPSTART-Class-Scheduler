package web

import (
	"net/http"
	"sort"
)

// Index – home page with classroom cards
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	mu.RLock()
	list := make([]*Classroom, 0, len(classroomsCache))
	for _, c := range classroomsCache {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	data := struct {
		Classrooms []*Classroom
		Sessions   map[int][]Session
	}{list, sessionsCache}
	mu.RUnlock()

	Templates.ExecuteTemplate(w, "index.html", data)
}