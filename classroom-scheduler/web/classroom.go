package web

import (
	"strconv"
	"net/http"
	"sort"
)

// Classroom detail page
func ClassroomHandler(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Path[len("/classroom/"):])
	if id < 1 {
		http.NotFound(w, r)
		return
	}
	mu.RLock()
	cl := classroomsCache[id]
	sess := sessionsCache[id]
	mu.RUnlock()
	if cl == nil {
		http.NotFound(w, r)
		return
	}
	sorted := append([]Session(nil), sess...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].StartTime < sorted[j].StartTime })

	RenderTemplate(w, "classroom.html", struct {
		ID       int
		Name     string
		Sessions []Session
	}{cl.ID, cl.Name, sorted})
}
