package web

import (
	"net/http"
	"sort"
	"time"
)

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
    mu.RUnlock()

    data := struct {
        Classrooms []*Classroom
        Sessions   map[int][]Session

        // Layout fields
        Active     string
        PageTitle  string
        Year       int
        ExtraCSS   []string // optional per-page CSS
        Flash      string
    }{
        Classrooms: list,
        Sessions:   sessionsCache,

        Active:    "home",
        PageTitle: "Home",
        Year:      time.Now().Year(),
        ExtraCSS:  []string{"index.css"},
        Flash:     "",
    }

    RenderTemplate(w, "index.html", data)
}