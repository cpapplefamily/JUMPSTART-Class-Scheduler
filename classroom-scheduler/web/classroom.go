package web

import (
	"strconv"
	"net/http"
	"sort"
	"time"
)

// Classroom detail page
func ClassroomHandler(w http.ResponseWriter, r *http.Request) {
    idStr := r.URL.Path[len("/classroom/"):]
    id, err := strconv.Atoi(idStr)
    if err != nil || id < 1 {
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

    // Sort sessions by start time
    sorted := append([]Session(nil), sess...)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].StartTime < sorted[j].StartTime
    })

    data := struct {
        ID        int
        Name      string
        Sessions  []Session

        Active    string
        PageTitle string
        Year      int
        ExtraCSS  []string
        Flash     string
    }{
        ID:        cl.ID,
        Name:      cl.Name,
        Sessions:  sorted,

        Active:    "",
        PageTitle: cl.Name,
        Year:      time.Now().Year(),
        ExtraCSS:  []string{"classroom.css"},
        Flash:     "",
    }

    RenderTemplate(w, "classroom.html", data)
}
