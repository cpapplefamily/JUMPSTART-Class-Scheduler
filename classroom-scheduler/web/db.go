// web/db.go
package web

import (
	"database/sql"
	"log"
	"sync"
	"strconv"
	_ "modernc.org/sqlite"
)

var (
	DB                     *sql.DB
	mu                     sync.RWMutex
	classroomsCache        = make(map[int]*Classroom)
	sessionsCache          = make(map[int][]Session)
	blocksCache            []Block
	sessionLengthMinutes   = 45
	breakMinutes           = 15
)

type Block struct{
	ID 			int; 
	StartTime	string;
	EndTime 	string 
}

type Session struct{
	ClassroomID int; 
	StartTime 	string;
	EndTime 	string;
	Title 		string;
	Presenter 	string;
	Description string
}

type Classroom struct{
	ID int; 
	Name string 
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

	_, err := DB.Exec(classroomsSQL)
	if err != nil {
		log.Fatal(err)
	}
	_, err = DB.Exec(sessionsSQL)
	if err != nil {
		log.Fatal(err)
	}
	_, err = DB.Exec(blocksSQL)
	if err != nil {
		log.Fatal(err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT
	);`)

	if err != nil {
		log.Fatal(err)
	}
}

func saveClassroomsToDB() { 
	tx, _ := DB.Begin()
	tx.Exec("DELETE FROM classrooms")
	stmt, _ := tx.Prepare("INSERT INTO classrooms (id, name) VALUES (?, ?)")
	for _, cl := range classroomsCache {
		stmt.Exec(cl.ID, cl.Name)
	}
	tx.Commit()
 }
func saveSessionsToDB()   { 
	tx, _ := DB.Begin()
	tx.Exec("DELETE FROM sessions")
	stmt, _ := tx.Prepare("INSERT INTO sessions (classroom_id, start_time, end_time, title, presenter, description) VALUES (?, ?, ?, ?, ?, ?)")
	for classroomID, sessions := range sessionsCache {
		for _, s := range sessions {
			stmt.Exec(classroomID, s.StartTime, s.EndTime, s.Title, s.Presenter, s.Description)
		}
	}
	tx.Commit()
 }
func saveBlocksToDB()     { 
	tx, _ := DB.Begin()
	tx.Exec("DELETE FROM blocks")
	stmt, _ := tx.Prepare("INSERT INTO blocks (id, start_time, end_time) VALUES (?, ?, ?)")
	for _, b := range blocksCache {
		stmt.Exec(b.ID, b.StartTime, b.EndTime)
	}
	tx.Commit()
}
func saveSetting(k, v string) {
	DB.Exec("INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)", k, v)
}

// Public function called from main.go
func OpenDBAndLoadCaches() {
	var err error
	DB, err = sql.Open("sqlite", "scheduler.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	createTables()
	loadCaches() // This loads everything including defaults
}

// Main cache loader — called once at startup
func loadCaches() {
	mu.Lock()
	defer mu.Unlock()

	loadClassroomsFromDB()
	loadSessionsFromDB()
	loadBlocksFromDB()
	ensureDefaultBlocks()     // ← creates default blocks if none exist
	loadSettingsFromDB()      // ← session length & break time

	log.Printf("Cache loaded: %d classrooms, %d blocks, %d total sessions",
		len(classroomsCache), len(blocksCache), countTotalSessions())
}

// Individual loaders
func loadClassroomsFromDB() {
	classroomsCache = make(map[int]*Classroom)
	rows, err := DB.Query("SELECT id, name FROM classrooms ORDER BY id")
	if err != nil {
		log.Fatal("Failed to load classrooms:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c Classroom
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			log.Fatal(err)
		}
		classroomsCache[c.ID] = &c
	}
	if len(classroomsCache) == 0 {
		log.Println("No classrooms in DB – will be created when you save in /blocks")
	}
}

func loadSessionsFromDB() {
	sessionsCache = make(map[int][]Session)
	rows, err := DB.Query("SELECT classroom_id, start_time, end_time, title, presenter, description FROM sessions")
	if err != nil {
		log.Fatal("Failed to load sessions:", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s Session
		var cid int
		if err := rows.Scan(&cid, &s.StartTime, &s.EndTime, &s.Title, &s.Presenter, &s.Description); err != nil {
			log.Fatal(err)
		}
		s.ClassroomID = cid
		sessionsCache[cid] = append(sessionsCache[cid], s)
	}
}

func loadBlocksFromDB() {
	blocksCache = blocksCache[:0] // clear
	rows, err := DB.Query("SELECT id, start_time, end_time FROM blocks ORDER BY id")
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
}

func ensureDefaultBlocks() {
	if len(blocksCache) > 0 {
		return // already have blocks
	}
	log.Println("No schedule found → creating default 5 blocks")
	blocksCache = []Block{
		{ID: 1, StartTime: "08:00", EndTime: "09:30"},
		{ID: 2, StartTime: "09:40", EndTime: "11:10"},
		{ID: 3, StartTime: "11:20", EndTime: "12:50"},
		{ID: 4, StartTime: "13:30", EndTime: "15:00"},
		{ID: 5, StartTime: "15:10", EndTime: "16:40"},

	}
	saveBlocksToDB()
}

func loadSettingsFromDB() {
	var val string
	if err := DB.QueryRow("SELECT value FROM settings WHERE key = 'session_length_minutes'").Scan(&val); err == sql.ErrNoRows {
		sessionLengthMinutes = 45
		saveSetting("session_length_minutes", "45")
	} else if err == nil {
		if n, _ := strconv.Atoi(val); n >= 20 && n <= 300 {
			sessionLengthMinutes = n
		}
	}

	if err := DB.QueryRow("SELECT value FROM settings WHERE key = 'break_minutes'").Scan(&val); err == sql.ErrNoRows {
		breakMinutes = 15
		saveSetting("break_minutes", "15")
	} else if err == nil {
		if n, _ := strconv.Atoi(val); n >= 0 && n <= 120 {
			breakMinutes = n
		}
	}
}

func countTotalSessions() int {
	total := 0
	for _, list := range sessionsCache {
		total += len(list)
	}
	return total
}