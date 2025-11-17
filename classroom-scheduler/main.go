package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	port   = ":8080"
	dbFile = "scheduler.db"
)

type Block struct {
	ID        int
	StartTime string // "08:00"
	EndTime   string // "08:45"
}

type Session struct {
	ID          int
	ClassroomID int
	StartTime   string
	EndTime     string
	Title       string
	Presenter   string
	Description string
}

type Classroom struct {
	ID   int
	Name string
}

var (
	templates       *template.Template
	db              *sql.DB
	mu              sync.RWMutex
	classroomsCache = make(map[int]*Classroom)
	sessionsCache   = make(map[int][]Session)
	blocksCache   []Block          // ordered list of blocks
	sessionLengthMinutes = 45 // default fallback (45 min )
	breakMinutes         = 15   // default break between sessions
	defaultBlocks = 5              // fallback if nothing saved
)

// ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
// CRITICAL: This init() must come BEFORE main() and register functions
// ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
func init() {
funcMap := template.FuncMap{
        "add":  func(a, b int) int { return a + b },
        "sub":  func(a, b int) int { return a - b },
        "seq":  func(count int) []int {
            s := make([]int, count)
            for i := 0; i < count; i++ { s[i] = i + 1 }
            return s
        },
    }

    var err error
    templates = template.New("").Funcs(funcMap)
    templates, err = templates.ParseGlob("templates/*.html")
    if err != nil {
        log.Fatalf("Failed to parse templates: %v", err)
    }
}

func main() {
	var err error
	db, err = sql.Open("sqlite", dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	createTables()
	loadCacheFromDB()

	log.Printf("Classroom Scheduler (with SQLite persistence)")
	log.Printf("    http://localhost%s", port)
	log.Printf("    Config: http://localhost%s/config", port)
	log.Printf("    Database: %s", dbFile)
	log.Println("--------------------------------------------------")

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/classroom/", classroomHandler)
	http.HandleFunc("/config", configHandler)
	http.HandleFunc("/config/save", configSaveHandler) // new endpoint
	http.HandleFunc("/blocks", blocksHandler)
    http.HandleFunc("/blocks/save", blocksSaveHandler)

	log.Fatal(http.ListenAndServe(port, logRequest(http.DefaultServeMux)))
}

func blocksHandler(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	data := struct {
		Blocks              []Block
		BlockCount          int
		DefaultSessionLength int
		BreakMinutes        int
	}{
		Blocks:              blocksCache,
		BlockCount:          len(blocksCache),
		DefaultSessionLength: sessionLengthMinutes,
		BreakMinutes:        breakMinutes,
	}
	mu.RUnlock()
	templates.ExecuteTemplate(w, "blocks.html", data)
}

func blocksSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	r.ParseForm()

	// Save settings
	if v := r.FormValue("session_length"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil && mins >= 20 && mins <= 300 {
			sessionLengthMinutes = mins
			saveSetting("session_length_minutes", v)
		}
	}
	if v := r.FormValue("break_minutes"); v != "" {
		if mins, err := strconv.Atoi(v); err == nil && mins >= 0 && mins <= 120 {
			breakMinutes = mins
			saveSetting("break_minutes", v)
		}
	}

	count, _ := strconv.Atoi(r.FormValue("block_count"))
	if count < 1 { count = 1 }
	if count > 20 { count = 20 }

	mu.Lock()
	blocksCache = make([]Block, count)

	var prevEnd time.Time
	for i := 0; i < count; i++ {
		idx := i + 1
		startStr := r.FormValue(fmt.Sprintf("start_%d", idx))
		endStr := r.FormValue(fmt.Sprintf("end_%d", idx))

		var startTime, endTime time.Time
		var err error

		if startStr != "" {
			startTime, err = time.Parse("15:04", startStr)
			if err != nil { startTime = time.Date(0,1,1,8,0,0,0,time.UTC) }
		} else if i == 0 {
			startTime = time.Date(0,1,1,8,0,0,0,time.UTC)
		} else {
			startTime = prevEnd.Add(time.Minute * time.Duration(breakMinutes))
		}

		if endStr != "" {
			endTime, _ = time.Parse("15:04", endStr)
		} else {
			endTime = startTime.Add(time.Minute * time.Duration(sessionLengthMinutes))
		}

		blocksCache[i] = Block{
			ID:        idx,
			StartTime: startTime.Format("15:04"),
			EndTime:   endTime.Format("15:04"),
		}
		prevEnd = endTime
	}
	mu.Unlock()

	saveBlocksToDB()
	log.Printf("Saved schedule: %d sessions, %d min length, %d min break", len(blocksCache), sessionLengthMinutes, breakMinutes)
	http.Redirect(w, r, "/blocks", http.StatusSeeOther)
}

func createTables() {
	classroomsSQL := `
	CREATE TABLE IF NOT EXISTS classrooms (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL DEFAULT ''
	);`

	sessionsSQL := `
	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		classroom_id INTEGER,
		start_time TEXT NOT NULL,  -- HH:MM
		end_time TEXT NOT NULL,    -- HH:MM
		title TEXT NOT NULL,
		presenter TEXT NOT NULL,
		description TEXT,
		FOREIGN KEY(classroom_id) REFERENCES classrooms(id) ON DELETE CASCADE
	);`

	blocksSQL := `
	CREATE TABLE IF NOT EXISTS blocks (
		id INTEGER PRIMARY KEY,
		start_time TEXT NOT NULL,
		end_time TEXT NOT NULL
	);`

	_, err := db.Exec(classroomsSQL)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(sessionsSQL)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec(blocksSQL)
	if err != nil {
		log.Fatal(err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT
	);`)

	if err != nil {
		log.Fatal(err)
	}
}

func loadCacheFromDB() {
	mu.Lock()
	defer mu.Unlock()

	// ========= 1. Load Classrooms =========
	classroomsCache = make(map[int]*Classroom)
	rows, err := db.Query("SELECT id, name FROM classrooms ORDER BY id")
	if err != nil {
		log.Fatal("Failed to load classrooms:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cl Classroom
		if err := rows.Scan(&cl.ID, &cl.Name); err != nil {
			log.Fatal(err)
		}
		classroomsCache[cl.ID] = &cl
	}
	if len(classroomsCache) == 0 {
		log.Println("No classrooms found – will be created on first save")
	}

	// ========= 2. Load Sessions =========
	sessionsCache = make(map[int][]Session)
	rows, err = db.Query("SELECT classroom_id, start_time, end_time, title, presenter, description FROM sessions ORDER BY classroom_id")
	if err != nil {
		log.Fatal("Failed to load sessions:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s Session
		var classroomID int
		if err := rows.Scan(&classroomID, &s.StartTime, &s.EndTime, &s.Title, &s.Presenter, &s.Description); err != nil {
			log.Fatal(err)
		}
		s.ClassroomID = classroomID
		sessionsCache[classroomID] = append(sessionsCache[classroomID], s)
	}

	// ========= 3. Load Blocks (your existing code) =========
	blocksCache = blocksCache[:0] // clear slice
	rows, err = db.Query("SELECT id, start_time, end_time FROM blocks ORDER BY id")
	if err != nil {
		log.Fatal("Failed to load blocks:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.StartTime, &b.EndTime); err != nil {
			log.Fatal(err)
		}
		blocksCache = append(blocksCache, b)
	}
	if len(blocksCache) == 0 {
		// Default schedule
		blocksCache = []Block{
			{1, "08:00", "09:30"},
			{2, "09:40", "11:10"},
			{3, "11:20", "12:50"},
			{4, "13:30", "15:00"},
			{5, "15:10", "16:40"},
			{6, "16:50", "18:20"},
		}
		saveBlocksToDB()
		log.Println("Created default 6 time blocks")
	}

	// ========= 4. Load Settings (session length & break) =========
	loadSettings() // ← make sure this function exists (see below)

	log.Printf("Loaded %d classrooms, %d session entries, %d blocks", 
		len(classroomsCache), countTotalSessions(), len(blocksCache))
}

// Helper to count total sessions (optional, just for nice logging)
func countTotalSessions() int {
	total := 0
	for _, sess := range sessionsCache {
		total += len(sess)
	}
	return total
}

func saveBlocksToDB() {
	tx, _ := db.Begin()
	tx.Exec("DELETE FROM blocks")
	stmt, _ := tx.Prepare("INSERT INTO blocks (id, start_time, end_time) VALUES (?, ?, ?)")
	for _, b := range blocksCache {
		stmt.Exec(b.ID, b.StartTime, b.EndTime)
	}
	tx.Commit()
}

// Middleware: log every request
func logRequest(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		handler.ServeHTTP(w, r)
		log.Printf("%s | %4s | %-30s | %v",
			time.Now().Format("2006-01-02 15:04:05"),
			r.Method,
			r.URL.Path,
			time.Since(start),
		)
	})
}

// Handlers

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	mu.RLock()
	defer mu.RUnlock()

	list := make([]*Classroom, 0, len(classroomsCache))
	for _, cl := range classroomsCache {
		list = append(list, cl)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	data := struct {
		Classrooms []*Classroom
		Sessions   map[int][]Session
	}{
		Classrooms: list,
		Sessions:   sessionsCache,
	}

	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("Template error (index): %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

func classroomHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/classroom/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	mu.RLock()
	cl, exists := classroomsCache[id]
	sessions := sessionsCache[id]
	mu.RUnlock()

	if !exists {
		http.NotFound(w, r)
		return
	}

	// Sort sessions by start time
	sorted := append([]Session(nil), sessions...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartTime < sorted[j].StartTime
	})

	data := struct {
		ID       int
		Name     string
		Sessions []Session
	}{
		ID:       cl.ID,
		Name:     cl.Name,
		Sessions: sorted,
	}

	templates.ExecuteTemplate(w, "classroom.html", data)
}

func configHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	// Always determine how many rooms to show
	numRooms := len(classroomsCache)
	if numRooms == 0 {
		numRooms = 3 // default: show 3 rooms on first visit
	}

	// Build a complete list of classrooms (existing + placeholders)
	var list []*Classroom
	for i := 1; i <= numRooms; i++ {
		if cl, exists := classroomsCache[i]; exists {
			list = append(list, cl)
		} else {
			// Create placeholder for new rooms
			list = append(list, &Classroom{ID: i, Name: fmt.Sprintf("Classroom %d", i)})
		}
	}

	data := struct {
		Classrooms     []*Classroom
		Sessions       map[int][]Session
		NumRooms       int
		GlobalSessions []Block   // ← this line must exist
	}{
		Classrooms:     list,
		Sessions:       sessionsCache,
		NumRooms:       numRooms,
		GlobalSessions: blocksCache,
	}

	if err := templates.ExecuteTemplate(w, "config.html", data); err != nil {
		log.Printf("Template error (config): %v", err)
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

func configSaveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	r.ParseForm()
	numClassrooms, _ := strconv.Atoi(r.FormValue("num_classrooms"))
	if numClassrooms < 1 {
		numClassrooms = 1
	}
	if numClassrooms > 30 {
		numClassrooms = 30
	}

	mu.Lock()
	defer mu.Unlock()

	// 1. Save classrooms
	classroomsCache = make(map[int]*Classroom)
	for i := 1; i <= numClassrooms; i++ {
		name := r.FormValue(fmt.Sprintf("roomname_%d", i))
		if name == "" {
			name = fmt.Sprintf("Classroom %d", i)
		}
		classroomsCache[i] = &Classroom{ID: i, Name: name}
	}

	// 2. Save all sessions (one per global block)
	sessionsCache = make(map[int][]Session)
	for classroomID := 1; classroomID <= numClassrooms; classroomID++ {
		var roomSessions []Session
		for idx := 0; idx < len(blocksCache); idx++ {
			title := r.FormValue(fmt.Sprintf("title_%d_%d", classroomID, idx))
			presenter := r.FormValue(fmt.Sprintf("presenter_%d_%d", classroomID, idx))
			desc := r.FormValue(fmt.Sprintf("desc_%d_%d", classroomID, idx))

			// Use global block times (even if user changed them manually, we trust the global ones)
			start := blocksCache[idx].StartTime
			end := blocksCache[idx].EndTime

			roomSessions = append(roomSessions, Session{
				ClassroomID: classroomID,
				StartTime:   start,
				EndTime:     end,
				Title:       title,
				Presenter:   presenter,
				Description: desc,
			})
		}
		sessionsCache[classroomID] = roomSessions
	}

	// 3. Persist to DB
	saveClassroomsToDB()
	saveSessionsToDB()

	log.Printf("Saved %d classrooms and their sessions", numClassrooms)
	http.Redirect(w, r, "/config", http.StatusSeeOther)
}

func saveClassroomsToDB() {
	tx, _ := db.Begin()
	tx.Exec("DELETE FROM classrooms")
	stmt, _ := tx.Prepare("INSERT INTO classrooms (id, name) VALUES (?, ?)")
	for _, cl := range classroomsCache {
		stmt.Exec(cl.ID, cl.Name)
	}
	tx.Commit()
}

func saveSessionsToDB() {
	tx, _ := db.Begin()
	tx.Exec("DELETE FROM sessions")
	stmt, _ := tx.Prepare("INSERT INTO sessions (classroom_id, start_time, end_time, title, presenter, description) VALUES (?, ?, ?, ?, ?, ?)")
	for classroomID, sessions := range sessionsCache {
		for _, s := range sessions {
			stmt.Exec(classroomID, s.StartTime, s.EndTime, s.Title, s.Presenter, s.Description)
		}
	}
	tx.Commit()
}

func loadSettings() {
	// Session length
	var val string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'session_length_minutes'").Scan(&val)
	if err == sql.ErrNoRows {
		sessionLengthMinutes = 80
		saveSetting("session_length_minutes", "80")
	} else if err == nil {
		sessionLengthMinutes, _ = strconv.Atoi(val)
	}

	// Break time
	err = db.QueryRow("SELECT value FROM settings WHERE key = 'break_minutes'").Scan(&val)
	if err == sql.ErrNoRows {
		breakMinutes = 10
		saveSetting("break_minutes", "10")
	} else if err == nil {
		breakMinutes, _ = strconv.Atoi(val)
	}
}

func saveSetting(key string, value string) {
	db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)", key, value)
}