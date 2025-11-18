//to run:
//go run import_to_sqlite.go -json sessions.json -db jumpstart2024.db

// import_to_sqlite.go
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "modernc.org/sqlite" // SQLite driver
)

type Session struct {
	TimeSlot     string   `json:"time_slot"`
	Round        string   `json:"round,omitempty"`
	Room         string   `json:"room"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Speakers     []string `json:"speakers"`
	Presenter    string   `json:"presenter,omitempty"`
}

type Data struct {
	Event    string     `json:"event"`
	Location string     `json:"location"`
	Sessions []Session  `json:"sessions"`
}

func main() {
	jsonFile := flag.String("json", "sessions.json", "Input JSON file (from csv_to_json)")
	dbFile   := flag.String("db", "jumpstart2024.db", "Output SQLite database")
	flag.Parse()

	// Read JSON
	data, err := os.ReadFile(*jsonFile)
	if err != nil {
		log.Fatalf("Cannot read JSON file: %v", err)
	}

	var payload Data
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Fatalf("Invalid JSON: %v", err)
	}

	// Open SQLite DB (creates if not exists)
	db, err := sql.Open("sqlite", *dbFile)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Enable WAL mode for better concurrency (optional but recommended)
	db.Exec("PRAGMA journal_mode=WAL;")

	// Drop table if exists (for clean re-import)
	db.Exec(`DROP TABLE IF EXISTS sessions`)

	// Create table
	createTable := `
	CREATE TABLE sessions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		time_slot   TEXT NOT NULL,
		round       TEXT,
		room        TEXT NOT NULL,
		title       TEXT NOT NULL,
		description TEXT,
		speakers    TEXT,               -- JSON array stored as TEXT
		presenter   TEXT,
		event       TEXT,
		location    TEXT,
		
		-- Full-text search virtual table will be added later if needed
		search_text TEXT GENERATED ALWAYS AS (
			title || ' ' || COALESCE(description, '') || ' ' || room || ' ' || COALESCE(round, '')
		) VIRTUAL
	);

	CREATE INDEX IF NOT EXISTS idx_time_slot ON sessions(time_slot);
	CREATE INDEX IF NOT EXISTS idx_room      ON sessions(room);
	CREATE INDEX IF NOT EXISTS idx_round     ON sessions(round);
	CREATE INDEX IF NOT EXISTS idx_search    ON sessions(search_text);
	`
	if _, err := db.Exec(createTable); err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	// Prepare insert statement
	stmt, err := db.Prepare(`
		INSERT INTO sessions 
		(time_slot, round, room, title, description, speakers, presenter, event, location)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Fatal(err)
	}
	defer stmt.Close()

	// Insert all sessions
	for _, s := range payload.Sessions {
		speakersJSON, _ := json.Marshal(s.Speakers)

		_, err := stmt.Exec(
			s.TimeSlot,
			nullString(s.Round),
			s.Room,
			s.Title,
			nullString(s.Description),
			string(speakersJSON),
			nullString(s.Presenter),
			payload.Event,
			payload.Location,
		)
		if err != nil {
			log.Printf("Failed to insert session '%s': %v", s.Title, err)
			continue
		}
	}

	// Final stats
	var count int
	db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)

	fmt.Printf(`
Import Complete!
Database: %s
Total Sessions Imported: %d
Event: %s
Location: %s

You can now query it with SQL, for example:

  SELECT time_slot, room, title, presenter FROM sessions 
  WHERE time_slot LIKE '9:25%' ORDER BY room;

  -- Search by keyword
  SELECT title, room, presenter FROM sessions 
  WHERE search_text LIKE '%%scouting%%';

  -- List all sessions in Glacier North
  SELECT time_slot, title, presenter FROM sessions 
  WHERE room = 'Glacier North';
`, *dbFile, count, payload.Event, payload.Location)
}

// Helper to convert empty string → NULL in SQLite
func nullString(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" || s == "TBD" {
		return nil
	}
	return s
}