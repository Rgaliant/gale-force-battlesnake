package main

import (
	"testing"
	"time"
)

func bestMoveFor(t *testing.T, raw string) string {
	t.Helper()
	b := boardFromJSON(t, raw)
	return BestMove(b, time.Now().Add(100*time.Millisecond))
}

func TestSearchAvoidsWall(t *testing.T) {
	// Head in the bottom-left corner moving along the wall; only up or right
	// stay on the board.
	move := bestMoveFor(t, `{
		"game": {"id":"g","timeout":500}, "turn": 1,
		"board": {"width":11,"height":11,"food":[],
			"snakes":[{"id":"me","health":90,"body":[{"x":0,"y":0},{"x":1,"y":0},{"x":2,"y":0}]}]},
		"you": {"id":"me","health":90,"body":[{"x":0,"y":0},{"x":1,"y":0},{"x":2,"y":0}]}
	}`)
	if move != "up" {
		t.Fatalf("corner snake should go up, got %q", move)
	}
}

func TestSearchAvoidsSmallPocket(t *testing.T) {
	// A wall of opponent body splits the board; left pocket is 1 cell at
	// (0,1)+... going left is a dead end, going up/right is open space.
	//
	// Opponent occupies the full column x=1 except y=0; our head at (0,0)
	// with body trailing right along y=0. Moving up enters a 10-cell strip
	// (column 0); the strip is walled by the opponent. Moving is forced: only
	// "up" is in bounds and legal; sanity: search returns it.
	move := bestMoveFor(t, `{
		"game": {"id":"g","timeout":500}, "turn": 1,
		"board": {"width":11,"height":11,"food":[],
			"snakes":[
				{"id":"me","health":90,"body":[{"x":0,"y":0},{"x":1,"y":0},{"x":2,"y":0}]},
				{"id":"o","health":90,"body":[
					{"x":1,"y":10},{"x":1,"y":9},{"x":1,"y":8},{"x":1,"y":7},{"x":1,"y":6},
					{"x":1,"y":5},{"x":1,"y":4},{"x":1,"y":3},{"x":1,"y":2},{"x":1,"y":1}]}]},
		"you": {"id":"me","health":90,"body":[{"x":0,"y":0},{"x":1,"y":0},{"x":2,"y":0}]}
	}`)
	if move != "up" {
		t.Fatalf("only legal move is up, got %q", move)
	}
}

func TestSearchTakesWinningHeadToHead(t *testing.T) {
	// We are longer; a head-to-head kills the opponent, not us. The search
	// should not be scared off the food at (5,6) by the shorter snake at
	// (5,7) - probableH2H only fires for equal-or-longer threats.
	move := bestMoveFor(t, `{
		"game": {"id":"g","timeout":500}, "turn": 1,
		"board": {"width":11,"height":11,"food":[{"x":5,"y":6}],
			"snakes":[
				{"id":"me","health":50,"body":[{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3},{"x":4,"y":3},{"x":3,"y":3}]},
				{"id":"o","health":90,"body":[{"x":5,"y":7},{"x":5,"y":8},{"x":5,"y":9}]}]},
		"you": {"id":"me","health":50,"body":[{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3},{"x":4,"y":3},{"x":3,"y":3}]}
	}`)
	if move != "up" {
		t.Fatalf("longer snake should contest the food, got %q", move)
	}
}

func TestSearchAvoidsLosingHeadToHead(t *testing.T) {
	// Mirror case: the opponent is longer; stepping to (5,6) risks a losing
	// head-to-head. Any other move should be preferred.
	move := bestMoveFor(t, `{
		"game": {"id":"g","timeout":500}, "turn": 1,
		"board": {"width":11,"height":11,"food":[{"x":5,"y":6}],
			"snakes":[
				{"id":"me","health":90,"body":[{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3}]},
				{"id":"o","health":90,"body":[{"x":5,"y":7},{"x":5,"y":8},{"x":5,"y":9},{"x":6,"y":9},{"x":7,"y":9}]}]},
		"you": {"id":"me","health":90,"body":[{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3}]}
	}`)
	if move == "up" {
		t.Fatal("shorter snake walked into a losing head-to-head")
	}
}

func TestSearchChasesOwnTailWhenTight(t *testing.T) {
	// Classic tail-chase: snake coiled in a ring with one gap - the cell its
	// own tail is about to vacate. Entering it is legal and safe.
	//
	//  . o o .
	//  . o H .      body ring around (1..2, 1..2), head H=(2,2), tail T=(2,1)
	//  . o T .
	move := bestMoveFor(t, `{
		"game": {"id":"g","timeout":500}, "turn": 1,
		"board": {"width":11,"height":11,"food":[],
			"snakes":[{"id":"me","health":90,"body":[
				{"x":2,"y":2},{"x":1,"y":2},{"x":1,"y":1},{"x":2,"y":1}]}]},
		"you": {"id":"me","health":90,"body":[
			{"x":2,"y":2},{"x":1,"y":2},{"x":1,"y":1},{"x":2,"y":1}]}
	}`)
	// Moving down = onto our own vacating tail; it must at minimum be legal
	// (not crash the search) and any returned move must be a legal one.
	if move == "left" {
		t.Fatal("moved into own neck")
	}
}

func TestSearchRespectsDeadline(t *testing.T) {
	b := boardFromJSON(t, `{
		"game": {"id":"g","timeout":500}, "turn": 1,
		"board": {"width":11,"height":11,"food":[{"x":8,"y":8}],
			"snakes":[
				{"id":"me","health":90,"body":[{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3}]},
				{"id":"o","health":90,"body":[{"x":2,"y":2},{"x":2,"y":1},{"x":2,"y":0}]},
				{"id":"p","health":90,"body":[{"x":8,"y":2},{"x":8,"y":1},{"x":8,"y":0}]},
				{"id":"q","health":90,"body":[{"x":2,"y":8},{"x":1,"y":8},{"x":0,"y":8}]}]},
		"you": {"id":"me","health":90,"body":[{"x":5,"y":5},{"x":5,"y":4},{"x":5,"y":3}]}
	}`)
	start := time.Now()
	move := BestMove(b, start.Add(150*time.Millisecond))
	elapsed := time.Since(start)
	if elapsed > 250*time.Millisecond {
		t.Fatalf("search overran deadline: %v", elapsed)
	}
	found := false
	for _, name := range DirName {
		if move == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("invalid move %q", move)
	}
}
