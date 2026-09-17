package main

import (
	"encoding/json"
	"testing"
)

func fixtureState(t *testing.T) *GameState {
	t.Helper()
	raw := `{
		"game": {"id": "g1", "timeout": 500},
		"turn": 10,
		"board": {
			"width": 11, "height": 11,
			"food": [{"x": 6, "y": 5}],
			"snakes": [
				{"id": "me", "health": 90, "body": [{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3}]},
				{"id": "opp", "health": 80, "body": [{"x":1,"y":1},{"x":1,"y":2},{"x":1,"y":3}]}
			]
		},
		"you": {"id": "me", "health": 90, "body": [{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3}]}
	}`
	var gs GameState
	if err := json.Unmarshal([]byte(raw), &gs); err != nil {
		t.Fatal(err)
	}
	return &gs
}

// snapshot captures observable snake state: stats plus live segments in
// tail-to-head order. Buffer slots outside the live window are dead storage
// and intentionally excluded - unmake restores the game, not the scratch RAM.
func snapshot(s *Snake) []int {
	out := []int{int(s.Body.lo), int(s.Body.lo >> 32), int(s.Body.hi), int(s.Body.hi >> 32),
		s.HeadCell, s.Length, s.Health}
	for p := s.tailPos; ; p = (p + 1) & bufMask {
		out = append(out, int(s.buf[p]))
		if p == s.headPos {
			break
		}
	}
	return out
}

func equalSnapshots(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLoadBoard(t *testing.T) {
	b, ok := LoadBoard(fixtureState(t))
	if !ok {
		t.Fatal("LoadBoard failed")
	}
	if b.N != 2 {
		t.Fatalf("N = %d, want 2", b.N)
	}
	me := &b.Snakes[0]
	if me.HeadCell != 5*11+5 || me.Length != 3 || me.Health != 90 {
		t.Fatalf("me loaded wrong: head=%d len=%d hp=%d", me.HeadCell, me.Length, me.Health)
	}
	if me.TailCell() != 3*11+5 {
		t.Fatalf("me tail = %d, want %d", me.TailCell(), 3*11+5)
	}
	if me.Body.PopCount() != 3 {
		t.Fatalf("me body popcount = %d, want 3", me.Body.PopCount())
	}
	if !b.Food.Has(5*11 + 6) {
		t.Fatal("food not loaded")
	}
}

func TestMakeUnmakePlainMove(t *testing.T) {
	b, _ := LoadBoard(fixtureState(t))
	me := &b.Snakes[0]
	before := snapshot(me)
	foodBefore := b.Food

	to := 5*11 + 4 // left
	u := b.Make(me, to)
	if me.HeadCell != to || me.Health != 89 || me.Length != 3 {
		t.Fatalf("after make: head=%d hp=%d len=%d", me.HeadCell, me.Health, me.Length)
	}
	if me.Body.Has(3*11 + 5) {
		t.Fatal("old tail bit should be cleared")
	}
	if !me.Body.Has(to) {
		t.Fatal("new head bit should be set")
	}

	b.Unmake(me, u)
	if !equalSnapshots(before, snapshot(me)) {
		t.Fatalf("unmake did not restore state:\n before %v\n after  %v", before, snapshot(me))
	}
	if b.Food != foodBefore {
		t.Fatal("food changed on a non-eating move")
	}
}

func TestMakeUnmakeEatingMove(t *testing.T) {
	b, _ := LoadBoard(fixtureState(t))
	me := &b.Snakes[0]
	before := snapshot(me)
	foodBefore := b.Food

	to := 5*11 + 6 // right, onto food
	u := b.Make(me, to)
	if me.Health != 100 || me.Length != 4 {
		t.Fatalf("after eat: hp=%d len=%d, want 100/4", me.Health, me.Length)
	}
	if b.Food.Has(to) {
		t.Fatal("food should be consumed")
	}
	// Official rules: the eating move still vacates the old tail; growth
	// duplicates the NEW last segment, which then holds for one extra turn.
	if me.Body.Has(3*11 + 5) {
		t.Fatal("old tail should vacate on the eating move itself")
	}
	if me.buf[me.tailPos] != me.buf[(me.tailPos+1)&bufMask] {
		t.Fatal("tail should be duplicated after eating")
	}
	// The duplicated tail means popcount is length-1.
	if me.Body.PopCount() != 3 {
		t.Fatalf("body popcount = %d, want 3 (4 segments, 1 duplicate)", me.Body.PopCount())
	}

	// Next move after eating: the duplicated tail cell (5,4) must NOT vacate -
	// the pop consumes the duplicate instead.
	u2 := b.Make(me, 5*11+7)
	if !me.Body.Has(4*11 + 5) {
		t.Fatal("duplicated tail cell should still be occupied one move after eating")
	}
	if me.Body.PopCount() != 4 {
		t.Fatalf("popcount after post-eat move = %d, want 4 (duplicate consumed)", me.Body.PopCount())
	}
	b.Unmake(me, u2)

	b.Unmake(me, u)
	if !equalSnapshots(before, snapshot(me)) {
		t.Fatalf("unmake(eat) did not restore state:\n before %v\n after  %v", before, snapshot(me))
	}
	if b.Food != foodBefore {
		t.Fatal("food not restored after unmake(eat)")
	}
}

func TestMakeUnmakeDeepSequenceRestores(t *testing.T) {
	b, _ := LoadBoard(fixtureState(t))
	me := &b.Snakes[0]
	opp := &b.Snakes[1]
	before0 := me.Body
	before1 := opp.Body
	foodBefore := b.Food

	// Walk both snakes several moves (me eats on the first), then unwind.
	type frame struct {
		s *Snake
		u undo
	}
	var stack []frame
	moves := []struct {
		s  *Snake
		to int
	}{
		{me, 5*11 + 6}, // eat
		{opp, 0*11 + 1},
		{me, 6*11 + 6},
		{opp, 0*11 + 2},
		{me, 7*11 + 6},
	}
	for _, m := range moves {
		stack = append(stack, frame{m.s, b.Make(m.s, m.to)})
	}
	for i := len(stack) - 1; i >= 0; i-- {
		b.Unmake(stack[i].s, stack[i].u)
	}
	if me.Body != before0 || opp.Body != before1 || b.Food != foodBefore {
		t.Fatal("deep make/unmake sequence did not restore the board")
	}
	if me.Health != 90 || me.Length != 3 || opp.Health != 80 {
		t.Fatalf("stats not restored: me hp=%d len=%d, opp hp=%d", me.Health, me.Length, opp.Health)
	}
}

func TestStarvationKillsAndUnmakeRevives(t *testing.T) {
	b, _ := LoadBoard(fixtureState(t))
	me := &b.Snakes[0]
	me.Health = 1
	u := b.Make(me, 5*11+4)
	if me.Alive {
		t.Fatal("snake at 1 hp moving without food should die")
	}
	b.Unmake(me, u)
	if !me.Alive || me.Health != 1 {
		t.Fatal("unmake should revive and restore health")
	}
}
