package web

import (
	"net/http"
	"strconv"
	"strings"

)

// Config page
func ConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	num := len(classroomsCache)
	if num == 0 {
		num = 3
	}
	list := make([]*Classroom, num)
	for i := 1; i <= num; i++ {
		if c, ok := classroomsCache[i]; ok {
			list[i-1] = c
		} else {
			list[i-1] = &Classroom{ID: i, Name: "Classroom " + strconv.Itoa(i)}
		}
	}
	data := struct {
		Classrooms     []*Classroom
		Sessions       map[int][]Session
		GlobalSessions []Block
	}{list, sessionsCache, blocksCache}
	Templates.ExecuteTemplate(w, "config.html", data)
}

func ConfigSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()
	mu.Lock()
	defer mu.Unlock()

	num := len(classroomsCache)
	if num == 0 {
		num = 3
	}

	// Update names
	for i := 1; i <= num; i++ {
		if _, ok := classroomsCache[i]; !ok {
			classroomsCache[i] = &Classroom{ID: i, Name: "Classroom " + strconv.Itoa(i)}
		}
		if n := r.FormValue("roomname_" + strconv.Itoa(i)); n != "" {
			classroomsCache[i].Name = n
		}
	}

	// Rebuild sessions from global blocks
	sessionsCache = make(map[int][]Session)
	for cid := 1; cid <= num; cid++ {
		var list []Session
		for idx, b := range blocksCache {
			list = append(list, Session{
				ClassroomID: cid,
				StartTime:   b.StartTime,
				EndTime:     b.EndTime,
				Title:       strings.TrimSpace(r.FormValue("title_" + strconv.Itoa(cid) + "_" + strconv.Itoa(idx))),
				Presenter:   strings.TrimSpace(r.FormValue("presenter_" + strconv.Itoa(cid) + "_" + strconv.Itoa(idx))),
				Description: strings.TrimSpace(r.FormValue("desc_" + strconv.Itoa(cid) + "_" + strconv.Itoa(idx))),
			})
		}
		sessionsCache[cid] = list
	}

	saveClassroomsToDB()
	saveSessionsToDB()
	http.Redirect(w, r, "/config", http.StatusSeeOther)
}