package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// searchMargin is reserved out of the game's advertised timeout for JSON
// handling, network round-trip, and the final iteration's overrun.
const searchMargin = 120 * time.Millisecond

func handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{
		"apiversion": "1",
		"author":     "Gale",
		"color":      "#4F9153",
		"head":       "ski",
		"tail":       "replit-notmark",
	})
}

func handleMove(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var gs GameState
	if err := json.NewDecoder(r.Body).Decode(&gs); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	move := "up"
	if b, ok := LoadBoard(&gs); ok {
		timeout := time.Duration(gs.Game.Timeout) * time.Millisecond
		if timeout == 0 {
			timeout = 500 * time.Millisecond
		}
		budget := timeout - searchMargin
		if budget < 50*time.Millisecond {
			budget = 50 * time.Millisecond
		}
		move = BestMove(b, start.Add(budget))
	} else {
		// Board too big for bitboards (or too many snakes): any in-bounds,
		// non-body move beats a fixed answer.
		move = fallbackMove(&gs)
	}

	writeJSON(w, map[string]string{"move": move})
}

// fallbackMove is the no-bitboard escape hatch: first in-bounds direction
// whose destination isn't inside any snake body.
func fallbackMove(gs *GameState) string {
	w, h := gs.Board.Width, gs.Board.Height
	head := gs.You.Body[0]
	occupied := map[[2]int]bool{}
	for _, s := range gs.Board.Snakes {
		for _, c := range s.Body {
			occupied[[2]int{c.X, c.Y}] = true
		}
	}
	deltas := map[string][2]int{
		"up": {0, 1}, "down": {0, -1}, "left": {-1, 0}, "right": {1, 0},
	}
	for name, d := range deltas {
		x, y := head.X+d[0], head.Y+d[1]
		if x >= 0 && x < w && y >= 0 && y < h && !occupied[[2]int{x, y}] {
			return name
		}
	}
	return "up"
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("response write: %v", err)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			handleInfo(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/move":
			handleMove(w, r)
		case r.Method == http.MethodPost && (r.URL.Path == "/start" || r.URL.Path == "/end"):
			writeJSON(w, struct{}{})
		default:
			http.NotFound(w, r)
		}
	})
	log.Printf("gosnake listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
