// frc_jumpstart_2024_sessions_to_json.go
// Complete Go script that correctly parses the FULL CSV you provided (including kickoff, check-in, showcase, lunch, all 5 rounds, etc.)
// Outputs clean, structured JSON ready for SQLite import in your Go project

//To run:
//go run frc_jumpstart_2024_sessions_to_json.go -csv jumpstart2024.csv -json sessions.json
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"log"
	"os"
	"regexp"
	"strings"
)

type Session struct {
	TimeSlot    string   `json:"time_slot"`
	Round       string   `json:"round,omitempty"`
	Room        string   `json:"room"`
	Title       string   `json:"title"`
	Description   string `json:"description"`
	Speakers    []string `json:"speakers"`
	Presenter    string  `json:"presenter,omitempty"`
}

var rooms = []string{
	"Voyageurs South",
	"Voyageurs North",
	"Glacier South",
	"Glacier North",
	"Cascade",
	"Theater",
	"Gallery",
	"Alumni Room",
	"Mississippi",
	"Valhalla",
}

func main() {
	csvPath := flag.String("csv", "jumpstart2024.csv", "Input CSV file path")
	jsonPath := flag.String("json", "jumpstart2024.json", "Output JSON file path")
	flag.Parse()

	file, err := os.Open(*csvPath)
	if err != nil {
		log.Fatalf("Cannot open CSV: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true  // handles occasional bad quoting
	reader.Comment = '#'     // skip commented lines if any

	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("Failed to read CSV: %v", err)
	}

	var sessions []Session

	timeRegex := regexp.MustCompile(`\d{1,2}:\d{2}[AP]M|\d{1,2}[AP]M`)

	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 3 {
			continue
		}

		c0 := strings.TrimSpace(record[0])

		// === Special spanning events (Kickoff, Check-in, Robot Showcase, etc.) ===
		if timeRegex.MatchString(c0) && len(record) > 1 && record[1] != "" && !strings.Contains(strings.ToLower(record[1]), "round") {
			timeSlot := c0
			title := strings.TrimSpace(record[1])

			room := "Various / See description"
			if len(record) > 3 && strings.TrimSpace(record[3]) != "" {
				room = strings.TrimSpace(record[3]) // catches Ballroom, Upper Level, etc.
			}
			if strings.Contains(title, "Atwood") {
				room = "Atwood Memorial Center"
			}

			speakers := parseSpeakers("")
			presenter := "TBD"
			if len(speakers) > 0 {
				presenter = strings.Join(speakers, ", ")
			}

			sessions = append(sessions, Session{
				TimeSlot:    timeSlot,
				Room:        room,
				Title:       title,
				Description:   title, // the long text is the description
				Speakers:    speakers,
				Presenter:    presenter,
			})
			continue
		}

		// === Main round sessions (Round 1–5) ===
		if timeRegex.MatchString(c0) && len(record) > 1 && strings.Contains(strings.ToLower(record[1]), "round") {
			timeSlot := c0
			round := strings.TrimSpace(record[1])

			// Find description row (starts with ,, )
			descs := []string{}
			if i+1 < len(records) && len(records[i+1]) >= 12 && records[i+1][0] == "" && records[i+1][1] == "" {
				descs = records[i+1][2:]
				i++ // skip desc row
			}

			// Find speaker row (also starts with ,, )
			speakersRaw := []string{}
			if i+1 < len(records) && len(records[i+1]) >= 12 && records[i+1][0] == "" && records[i+1][1] == "" {
				speakersRaw = records[i+1][2:]
				i++ // skip speaker row
			}

			titles := record[2:]

			for j := 0; j < len(rooms); j++ {
				if j >= len(titles) || strings.TrimSpace(titles[j]) == "" {
					continue
				}

				title := strings.TrimSpace(titles[j])
				desc := ""
				if j < len(descs) {
					desc = strings.TrimSpace(descs[j])
				}

				speakerRaw := ""
				if j < len(speakersRaw) {
					speakerRaw = strings.TrimSpace(speakersRaw[j])
				}

				speakersList := parseSpeakers(speakerRaw)
				presenter := "TBD"
				if len(speakersList) > 0 {
					presenter = strings.Join(speakersList, ", ")
				}

				sessions = append(sessions, Session{
					TimeSlot:    timeSlot,
					Round:       round,
					Room:        rooms[j],
					Title:       title,
					Description:   desc,
					Speakers:    speakersList,
					Presenter:    presenter,
				})
			}
		}
	}

	// Manually add lunch (not in time column, but everyone needs to know!)
	sessions = append(sessions, Session{
		TimeSlot:    "12:05PM - 1:25PM",
		Room:        "Garvey Commons",
		Title:       "Lunch Break",
		Description:   "Lunch served in Garvey Commons",
		Speakers:    []string{},
		Presenter:    "",
	})

	// Write JSON
	out := map[string]interface{}{
		"event":     "2024 JUMPSTART Training Sessions",
		"location":  "St Cloud State University",
		"sessions":  sessions,
	}

	jsonData, _ := json.MarshalIndent(out, "", "  ")
	os.WriteFile(*jsonPath, jsonData, 0644)

	log.Printf("Parsed %d sessions → %s\n", len(sessions), *jsonPath)
}

func parseSpeakers(raw string) []string {
	if strings.TrimSpace(raw) == "" || strings.ToLower(raw) == "various" {
		return []string{"Various / Panel"}
	}

	// Split on , / & 
	parts := regexp.MustCompile(`[,&/]`).Split(raw, -1)
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// If it looks like a team number → "Team 4607"
		if len(part) >= 4 && regexp.MustCompile(`^\d+$`).MatchString(part) && part != "2025" && part != "2024" {
			result = append(result, "Team "+part)
			continue
		}

		// Try to detect "Name TeamXXXX" pattern (e.g. "Amy K 4728")
		if strings.Contains(part, " ") {
			fields := strings.Fields(part)
			last := fields[len(fields)-1]
			if len(last) >= 4 && regexp.MustCompile(`^\d+$`).MatchString(last) {
				team := "Team " + last
				name := strings.Join(fields[:len(fields)-1], " ")
				result = append(result, name+" ("+team+")")
				continue
			}
		}

		result = append(result, part)
	}

	if len(result) == 0 {
		result = []string{"Various / Panel"}
	}
	return result
}