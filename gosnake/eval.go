package main

// Score scale: integer, roughly "centi-cells". Terminal scores live far
// outside the heuristic range.
const (
	WinScore = 20000

	kOwned     = 2  // per cell we reach strictly before every opponent
	kFood      = 2  // per food cell inside our owned region
	kLength    = 8  // per segment of length differential vs each opponent
	kTight     = 4  // ramp: per cell of credited-reach shortfall below 2x length
	kHungerDiv = 16 // hunger * food-distance / this
	maxVoronoi = 32 // wavefront propagation rounds
)

// Eval scores the board from snake 0's perspective. movedMask marks snakes
// that have already moved this turn (bit i = snake i) - unmoved opponents get
// an extra Voronoi expansion step since they effectively move first against
// any cell we're racing them to, as do longer-or-equal snakes since they win
// the head-to-head at any contested frontier.
func Eval(b *Board, movedMask uint32) int {
	g := &b.Geo
	me := &b.Snakes[0]
	bodies := b.AllBodies()

	// Voronoi seeding.
	owned := BitAt(me.HeadCell)
	var lost BB
	for i := 1; i < b.N; i++ {
		s := &b.Snakes[i]
		if !s.Alive {
			continue
		}
		seed := BitAt(s.HeadCell)
		if movedMask&(1<<uint(i)) == 0 {
			seed = seed.Or(g.Adj(seed).AndNot(bodies))
		}
		if s.Length >= me.Length {
			seed = seed.Or(g.Adj(seed).AndNot(bodies))
		}
		lost = lost.Or(seed)
	}

	// Simultaneous wavefront propagation; early-exit once both fronts stall.
	for i := 0; i < maxVoronoi; i++ {
		nextOwned := owned.Or(g.Adj(owned).AndNot(bodies).AndNot(lost))
		nextLost := lost.Or(g.Adj(lost).AndNot(bodies).AndNot(nextOwned))
		if nextOwned == owned && nextLost == lost {
			break
		}
		owned, lost = nextOwned, nextLost
	}

	// Tail-following credit: a region whose edge touches a snake's tail opens
	// up as that tail vacates, so it's roomier than the raw count says.
	// (Sandworm computes the same credit for `lost` and then never uses it -
	// owned-only is what actually plays at rank 4 of 500.)
	nOwned := owned.PopCount()
	ownedEdge := owned.Or(g.Adj(owned))
	for i := 0; i < b.N; i++ {
		s := &b.Snakes[i]
		if s.Alive && ownedEdge.Has(s.TailCell()) {
			nOwned += s.Length / 2
		}
	}

	score := nOwned * kOwned
	score += owned.And(b.Food).PopCount() * kFood

	for i := 1; i < b.N; i++ {
		if b.Snakes[i].Alive {
			score += (me.Length - b.Snakes[i].Length) * kLength
		}
	}

	// Reach: cells we can get to at all, ignoring races - this is the "am I
	// coiling myself into a pocket" signal, distinct from contested territory.
	// The ramp starts penalizing well before the pocket is fatal so the search
	// has a gradient to climb away from (the cliff-vs-ramp lesson from the
	// Python version, validated 3-7 -> 5-5 against Abraham).
	reach := BitAt(me.HeadCell)
	foodDist := -1
	for i := 0; ; i++ {
		if foodDist < 0 && reach.Intersects(b.Food) {
			foodDist = i
		}
		next := reach.Or(g.Adj(reach).AndNot(bodies))
		if next == reach {
			break
		}
		reach = next
	}
	credited := reach.PopCount()
	reachEdge := reach.Or(g.Adj(reach))
	for i := 0; i < b.N; i++ {
		s := &b.Snakes[i]
		if s.Alive && reachEdge.Has(s.TailCell()) {
			credited += s.Length / 2
		}
	}
	if shortfall := 2*me.Length - credited; shortfall > 0 {
		score -= shortfall * kTight
	}

	// Hunger: pull toward reachable food, scaled by how low health is. If no
	// food is reachable at all, treat it as a far distance rather than a wall.
	if foodDist < 0 {
		foodDist = g.W + g.H
	}
	if hunger := 100 - me.Health; hunger > 0 {
		score -= hunger * foodDist / kHungerDiv
	}

	return score
}
