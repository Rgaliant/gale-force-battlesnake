package main

import (
	"encoding/json"
	"testing"
	"time"
)

const midgame4p = `{
	"game": {"id":"g","timeout":500}, "turn": 60,
	"board": {"width":11,"height":11,
		"food":[{"x":0,"y":4},{"x":9,"y":9},{"x":5,"y":0}],
		"snakes":[
			{"id":"me","health":80,"body":[
				{"x":5,"y":5},{"x":5,"y":4},{"x":4,"y":4},{"x":3,"y":4},{"x":3,"y":5},
				{"x":3,"y":6},{"x":4,"y":6}]},
			{"id":"a","health":75,"body":[
				{"x":8,"y":7},{"x":8,"y":8},{"x":7,"y":8},{"x":6,"y":8},{"x":6,"y":9},
				{"x":5,"y":9}]},
			{"id":"b","health":90,"body":[
				{"x":2,"y":2},{"x":2,"y":1},{"x":3,"y":1},{"x":4,"y":1},{"x":5,"y":1}]},
			{"id":"c","health":60,"body":[
				{"x":9,"y":2},{"x":9,"y":3},{"x":8,"y":3},{"x":7,"y":3}]}]},
	"you": {"id":"me","health":80,"body":[
		{"x":5,"y":5},{"x":5,"y":4},{"x":4,"y":4},{"x":3,"y":4},{"x":3,"y":5},
		{"x":3,"y":6},{"x":4,"y":6}]}
}`

// TestThroughputReport isn't a correctness test - it prints the numbers the
// migration was done for: nodes/sec and plies reached in a 400ms budget on a
// 4-snake midgame board. Run with -v to see them.
func TestThroughputReport(t *testing.T) {
	b := boardFromJSON(t, midgame4p)
	sr := &Searcher{b: b, deadline: time.Now().Add(400 * time.Millisecond)}

	me := &b.Snakes[0]
	start := time.Now()
	deepest := 0
	for depth := 2; depth <= maxPlies; depth++ {
		sr.rootDepth = depth
		alpha, beta := -WinScore*2, WinScore*2
		for d := 0; d < 4; d++ {
			to := b.Geo.Next[me.HeadCell][d]
			if to < 0 || !sr.legalDest(0, int(to)) {
				continue
			}
			if sr.probableH2H(0, int(to)) {
				continue
			}
			u := b.Make(me, int(to))
			sr.movedMask = 1
			v := sr.step(1, depth-1, alpha, beta)
			sr.movedMask = 0
			b.Unmake(me, u)
			if !sr.aborted && v > alpha {
				alpha = v
			}
		}
		if sr.aborted {
			break
		}
		deepest = depth
	}
	elapsed := time.Since(start)

	perSec := float64(sr.nodes) / elapsed.Seconds()
	t.Logf("4-snake midgame: %d nodes in %v = %.0f nodes/sec, completed depth %d plies (~%d turns with 4 snakes)",
		sr.nodes, elapsed.Round(time.Millisecond), perSec, deepest, deepest/4)
	if deepest < 8 {
		t.Fatalf("expected to complete at least 8 plies in 400ms, got %d", deepest)
	}
}

func BenchmarkEval(b *testing.B) {
	var gs GameState
	if err := jsonUnmarshal(midgame4p, &gs); err != nil {
		b.Fatal(err)
	}
	board, _ := LoadBoard(&gs)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(board, 0)
	}
}

func jsonUnmarshal(raw string, v interface{}) error {
	return json.Unmarshal([]byte(raw), v)
}
