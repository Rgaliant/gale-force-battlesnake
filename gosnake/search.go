package main

import "time"

// Paranoid N-player minimax over a step chain: within each simulated turn,
// every live snake gets its own ply in slot order (we are always slot 0, so
// our tail correctly vacates before opponents move; opponents' tails are
// conservatively still in place when we move - same approximation the Python
// engine made, erring toward caution). We maximize; every opponent minimizes.
//
// depth counts plies, not turns, matching how the deadline actually limits
// work when snake count varies.

const (
	maxPlies    = 96
	h2hNudge    = 16 // probable head-to-head beats certain death by this much
	nodeCheck   = 511
)

type Searcher struct {
	b         *Board
	deadline  time.Time
	nodes     int
	aborted   bool
	movedMask uint32
	rootDepth int
	// Move-ordering cache: value each (ply, direction) got last iteration.
	// Shared across siblings at a ply - approximate but effective, and how
	// Sandworm does it.
	cache [maxPlies][4]int32
}

func (sr *Searcher) checkTime() bool {
	sr.nodes++
	if sr.nodes&nodeCheck == 0 && time.Now().After(sr.deadline) {
		sr.aborted = true
	}
	return sr.aborted
}

func (sr *Searcher) deathScore(depth int) int { return -(WinScore + depth) }

// legalDest reports whether snake s may move to cell `to`: on-board (caller
// guarantees), and not into any live body - with the exception of s's own
// tail cell when that tail will vacate this turn (no growth duplicate).
func (sr *Searcher) legalDest(s int, to int) bool {
	for i := 0; i < sr.b.N; i++ {
		sn := &sr.b.Snakes[i]
		if !sn.Alive || !sn.Body.Has(to) {
			continue
		}
		if i == s && to == sn.TailCell() &&
			sn.buf[sn.tailPos] != sn.buf[(sn.tailPos+1)&bufMask] {
			continue // own tail vacates as we arrive
		}
		return false
	}
	return true
}

// probableH2H reports whether moving snake s to `to` risks a losing (or
// mutual) head-to-head: some other snake at least as long, which hasn't moved
// yet this turn, could step onto the same cell.
func (sr *Searcher) probableH2H(s int, to int) bool {
	mover := &sr.b.Snakes[s]
	for j := 0; j < sr.b.N; j++ {
		if j == s || sr.movedMask&(1<<uint(j)) != 0 {
			continue
		}
		other := &sr.b.Snakes[j]
		if !other.Alive || other.Length < mover.Length {
			continue
		}
		for d := 0; d < 4; d++ {
			if n := sr.b.Geo.Next[to][d]; n >= 0 && int(n) == other.HeadCell {
				return true
			}
		}
	}
	return false
}

func (sr *Searcher) liveOpponents() int {
	n := 0
	for i := 1; i < sr.b.N; i++ {
		if sr.b.Snakes[i].Alive {
			n++
		}
	}
	return n
}

// step runs the ply for snake index s (skipping dead ones); when s passes the
// last snake, the turn ends and a new one begins with the moved-mask cleared.
func (sr *Searcher) step(s int, depth, alpha, beta int) int {
	if sr.checkTime() {
		return 0
	}
	if !sr.b.Snakes[0].Alive {
		return sr.deathScore(depth)
	}

	for s < sr.b.N && !sr.b.Snakes[s].Alive {
		s++
	}
	if s == sr.b.N {
		// Turn boundary: everyone has moved.
		if sr.liveOpponents() == 0 {
			return WinScore + depth
		}
		oldMask := sr.movedMask
		sr.movedMask = 0
		v := sr.step(0, depth, alpha, beta)
		sr.movedMask = oldMask
		return v
	}

	if depth <= 0 {
		return Eval(sr.b, sr.movedMask)
	}
	if s == 0 && sr.liveOpponents() == 0 {
		return WinScore + depth
	}

	snake := &sr.b.Snakes[s]
	maximizing := s == 0
	ply := sr.rootDepth - depth
	if ply >= maxPlies {
		return Eval(sr.b, sr.movedMask)
	}

	// Order directions by last iteration's values at this ply.
	var dirs [4]int
	for i := range dirs {
		dirs[i] = i
	}
	cache := &sr.cache[ply]
	for i := 1; i < 4; i++ {
		for j := i; j > 0; j-- {
			better := cache[dirs[j]] > cache[dirs[j-1]]
			if !maximizing {
				better = cache[dirs[j]] < cache[dirs[j-1]]
			}
			if better {
				dirs[j], dirs[j-1] = dirs[j-1], dirs[j]
			} else {
				break
			}
		}
	}

	best := -WinScore * 2
	if !maximizing {
		best = WinScore * 2
	}
	moved := false

	for _, d := range dirs {
		if alpha >= beta {
			break
		}
		to16 := sr.b.Geo.Next[snake.HeadCell][d]
		if to16 < 0 {
			continue
		}
		to := int(to16)
		if !sr.legalDest(s, to) {
			continue
		}

		var value int
		if sr.probableH2H(s, to) {
			// Don't recurse: score it as death-for-the-mover, nudged toward
			// neutral so a probable head-to-head beats a certain death (and
			// an opponent prefers risking one over crashing into a wall).
			if maximizing {
				value = sr.deathScore(depth) + h2hNudge
			} else {
				value = WinScore + depth - h2hNudge
			}
			moved = true
		} else {
			u := sr.b.Make(snake, to)
			sr.movedMask |= 1 << uint(s)
			if !snake.Alive {
				// Starved mid-move.
				if maximizing {
					value = sr.deathScore(depth - 1)
				} else {
					value = sr.step(s+1, depth-1, alpha, beta)
				}
			} else {
				value = sr.step(s+1, depth-1, alpha, beta)
			}
			sr.movedMask &^= 1 << uint(s)
			sr.b.Unmake(snake, u)
			moved = true
		}

		if sr.aborted {
			return 0
		}
		cache[d] = int32(value)
		if maximizing {
			if value > best {
				best = value
			}
			if best > alpha {
				alpha = best
			}
		} else {
			if value < best {
				best = value
			}
			if best < beta {
				beta = best
			}
		}
	}

	if !moved {
		// No legal move at all: this snake dies where it stands. For us
		// that's a loss; for an opponent, mark them dead and keep searching
		// the rest of the turn.
		if maximizing {
			return sr.deathScore(depth)
		}
		snake.Alive = false
		v := sr.step(s+1, depth-1, alpha, beta)
		snake.Alive = true
		return v
	}
	return best
}

// BestMove runs iterative deepening until the deadline and returns the best
// direction from the deepest fully completed iteration.
func BestMove(b *Board, deadline time.Time) string {
	sr := &Searcher{b: b, deadline: deadline}
	me := &b.Snakes[0]

	type rootMove struct {
		dir   int
		to    int
		value int
	}
	var moves []rootMove
	for d := 0; d < 4; d++ {
		if to := b.Geo.Next[me.HeadCell][d]; to >= 0 && sr.legalDest(0, int(to)) {
			moves = append(moves, rootMove{dir: d, to: int(to)})
		}
	}
	if len(moves) == 0 {
		// Truly boxed in: any in-bounds direction; the engine kills us either
		// way but never answer with an off-board move.
		for d := 0; d < 4; d++ {
			if b.Geo.Next[me.HeadCell][d] >= 0 {
				return DirName[d]
			}
		}
		return "up"
	}
	if len(moves) == 1 {
		return DirName[moves[0].dir]
	}

	bestDir := moves[0].dir
	for depth := 2; depth <= maxPlies; depth++ {
		sr.rootDepth = depth
		alpha, beta := -WinScore*2, WinScore*2
		iterBest := -WinScore * 2
		iterDir := bestDir
		for i := range moves {
			m := &moves[i]
			var value int
			if sr.probableH2H(0, m.to) {
				value = sr.deathScore(depth) + h2hNudge
			} else {
				u := b.Make(me, m.to)
				sr.movedMask = 1
				if !me.Alive {
					value = sr.deathScore(depth - 1)
				} else {
					value = sr.step(1, depth-1, alpha, beta)
				}
				sr.movedMask = 0
				b.Unmake(me, u)
			}
			if sr.aborted {
				break
			}
			m.value = value
			if value > iterBest {
				iterBest = value
				iterDir = m.dir
			}
			if iterBest > alpha {
				alpha = iterBest
			}
		}
		if sr.aborted {
			break
		}
		bestDir = iterDir
		// Best-first ordering for the next, deeper iteration.
		for i := 1; i < len(moves); i++ {
			for j := i; j > 0 && moves[j].value > moves[j-1].value; j-- {
				moves[j], moves[j-1] = moves[j-1], moves[j]
			}
		}
	}
	return DirName[bestDir]
}
