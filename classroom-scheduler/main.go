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
	defaultBlockMinutes = 45 // default fallback (90 min = 1h30)
	defaultBlocks = 5              // fallback if nothing saved
)

// ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
// CRITICAL: This init() must come BEFORE main() and register functions
// ←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←←
func init() {
    funcMap := template.FuncMap{
        "add": func(a, b int) int {
            return a + b
        },
        "repeat": func(count int) []struct{} {
            return make([]struct{}, count)
        },
        "seq": func(count int) []int {   // ← NEW
            s := make([]int, count)
            for i := 0; i < count; i++ {
                s[i] = i
            }
            return s
        },
    }

    var err error
    templates = template.New("").Funcs(funcMap)
    templates, err = templates.ParseGlob("templates/*.html")
    if err != nil {
        log.Fatalf("Failed to parse templates: %v", err)
    }
    log.Println("Templates loaded successfully (add/repeat/seq functions ready)")
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
		DefaultBlockMinutes int
	}{
		Blocks:              blocksCache,
		BlockCount:          len(blocksCache),
		DefaultBlockMinutes: defaultBlockMinutes,
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

	// Save default length
	if mins, err := strconv.Atoi(r.FormValue("default_minutes")); err == nil && mins >= 10 && mins <= 300 {
		defaultBlockMinutes = mins
		saveDefaultBlockMinutes()
	}

	count, _ := strconv.Atoi(r.FormValue("block_count"))
	if count < 1 { count = 1 }
	if count > 20 { count = 20 }

	mu.Lock()
	blocksCache = make([]Block, count)
	for i := 0; i < count; i++ {
		idx := i + 1
		start := r.FormValue(fmt.Sprintf("start_%d", idx))
		end := r.FormValue(fmt.Sprintf("end_%d", idx))
		if start == "" { start = "08:00" }
		if end == "" {
			// Auto-calculate if empty
			t, _ := time.Parse("15:04", start)
			end = t.Add(time.Minute * time.Duration(defaultBlockMinutes)).Format("15:04")
		}
		blocksCache[i] = Block{ID: idx, StartTime: start, EndTime: end}
	}
	mu.Unlock()

	saveBlocksToDB()
	log.Printf("Saved %d blocks with default length %d minutes", len(blocksCache), defaultBlockMinutes)
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

	// ... existing classrooms/sessions code ...

	// Load blocks
	blocksCache = blocksCache[:0] // clear
	rows, err := db.Query("SELECT id, start_time, end_time FROM blocks ORDER BY id")
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var b Block
		rows.Scan(&b.ID, &b.StartTime, &b.EndTime)
		blocksCache = append(blocksCache, b)
	}
	if len(blocksCache) == 0 {
		// Create default 6 blocks on first run
		blocksCache = []Block{
			{1, "08:00", "09:30"},
			{2, "09:40", "11:10"},
			{3, "11:20", "12:50"},
			{4, "13:30", "15:00"},
			{5, "15:10", "16:40"},
			{6, "16:50", "18:20"},
		}
		saveBlocksToDB() // persist defaults
	}
	log.Printf("Loaded %d time blocks", len(blocksCache))
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
	list := make([]*Classroom, 0, len(classroomsCache))
	for _, cl := range classroomsCache {
		list = append(list, cl)
	}
	mu.RUnlock()

	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	templates.ExecuteTemplate(w, "index.html", list)
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
		Classrooms []*Classroom
		Sessions   map[int][]Session
		NumRooms   int
	}{
		Classrooms: list,
		Sessions:   sessionsCache,
		NumRooms:   numRooms,
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

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad form", 400)
		return
	}

	numRooms, _ := strconv.Atoi(r.FormValue("num_classrooms"))
	if numRooms < 1 {
		numRooms = 1
	}
	if numRooms > 50 {
		numRooms = 50
	}

	tx, err := db.Begin()
	if err != nil {
		log.Println(err)
		http.Error(w, "DB error", 500)
		return
	}

	// Clear everything
	tx.Exec("DELETE FROM sessions")
	tx.Exec("DELETE FROM classrooms")

	// Create new classrooms
	stmtClass, _ := tx.Prepare("INSERT INTO classrooms (id, name) VALUES (?, ?)")
	defer stmtClass.Close()

	stmtSess, _ := tx.Prepare(`
		INSERT INTO sessions (classroom_id, start_time, end_time, title, presenter, description)
		VALUES (?, ?, ?, ?, ?, ?)`)
	defer stmtSess.Close()

	for i := 1; i <= numRooms; i++ {
		name := r.FormValue(fmt.Sprintf("roomname_%d", i))
		if name == "" {
			name = fmt.Sprintf("Classroom %d", i)
		}
		stmtClass.Exec(i, name)

		// Parse sessions for this room (up to 20 per room)
		for j := 0; j < 20; j++ {
			start := r.FormValue(fmt.Sprintf("start_%d_%d", i, j))
			if start == "" {
				continue
			}
			end := r.FormValue(fmt.Sprintf("end_%d_%d", i, j))
			title := r.FormValue(fmt.Sprintf("title_%d_%d", i, j))
			presenter := r.FormValue(fmt.Sprintf("presenter_%d_%d", i, j))
			desc := r.FormValue(fmt.Sprintf("desc_%d_%d", i, j))

			if title == "" || presenter == "" {
				continue
			}

			stmtSess.Exec(i, start, end, title, presenter, desc)
		}
	}

	tx.Commit()
	log.Printf("Saved configuration: %d classrooms", numRooms)

	// Reload cache
	loadCacheFromDB()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func loadDefaultBlockMinutes() {
	var val string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'default_block_minutes'").Scan(&val)
	if err == sql.ErrNoRows {
		defaultBlockMinutes = 90
		saveDefaultBlockMinutes()
	} else if err == nil {
		defaultBlockMinutes, _ = strconv.Atoi(val)
	}
	if defaultBlockMinutes < 10 {
		defaultBlockMinutes = 90
	}
}

func saveDefaultBlockMinutes() {
	db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('default_block_minutes', ?)", defaultBlockMinutes)
}