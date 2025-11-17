// blocks.go
package web

import (
	"log"
	"net/http"
	"strconv"
	"time"
)

func BlocksHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()

	// Classrooms count
	num := len(classroomsCache)
	if num == 0 {
		// default
		num = 3
	}	

	data := struct {
		Blocks               []Block
		BlockCount           int
		NumClassrooms        int
		DefaultSessionLength int
		BreakMinutes         int
	}{
		Blocks:               blocksCache,
		BlockCount:           len(blocksCache),
		NumClassrooms:        num,
		DefaultSessionLength: sessionLengthMinutes,
		BreakMinutes:         breakMinutes,
	}
	mu.RUnlock()
	Templates.ExecuteTemplate(w, "blocks.html", data)
}

func BlocksSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()

	// Classrooms count
	numClassrooms, _ := strconv.Atoi(r.FormValue("num_classrooms"))
	if numClassrooms < 1 {
		numClassrooms = 1
	}
	if numClassrooms > 30 {
		numClassrooms = 30
	}

	// Session length & break
	if v := r.FormValue("session_length"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 20 && n <= 300 {
			sessionLengthMinutes = n
			saveSetting("session_length_minutes", v)
		}
	}
	if v := r.FormValue("break_minutes"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 120 {
			breakMinutes = n
			saveSetting("break_minutes", v)
		}
	}

	// Blocks count
	count, _ := strconv.Atoi(r.FormValue("block_count"))
	if count < 1 {
		count = 1
	}
	if count > 20 {
		count = 20
	}

	mu.Lock()
	// Resize classrooms
	newClassrooms := make(map[int]*Classroom)
	for i := 1; i <= numClassrooms; i++ {
		if old, ok := classroomsCache[i]; ok {
			newClassrooms[i] = old
		} else {
			newClassrooms[i] = &Classroom{ID: i, Name: "Classroom " + strconv.Itoa(i)}
		}
	}
	classroomsCache = newClassrooms

	// Build new blocks
	blocksCache = make([]Block, count)
	var prevEnd time.Time
	for i := 0; i < count; i++ {
		idx := i + 1
		startStr := r.FormValue("start_" + strconv.Itoa(idx))
		endStr := r.FormValue("end_" + strconv.Itoa(idx))

		var startTime time.Time
		if startStr != "" {
			startTime, _ = time.Parse("15:04", startStr)
		} else if i == 0 {
			startTime = time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC)
		} else {
			startTime = prevEnd.Add(time.Minute * time.Duration(breakMinutes))
		}

		endTime := startTime.Add(time.Minute * time.Duration(sessionLengthMinutes))
		if endStr != "" {
			if t, err := time.Parse("15:04", endStr); err == nil {
				endTime = t
			}
		}

		blocksCache[i] = Block{
			ID:        idx,
			StartTime: startTime.Format("15:04"),
			EndTime:   endTime.Format("15:04"),
		}
		prevEnd = endTime
	}
	mu.Unlock()

	saveClassroomsToDB()
	saveBlocksToDB()
	log.Printf("Saved: %d classrooms, %d blocks", numClassrooms, count)
	http.Redirect(w, r, "/blocks", http.StatusSeeOther)
}