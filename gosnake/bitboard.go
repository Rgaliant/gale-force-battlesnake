package main

import "math/bits"

// BB is a bitboard over at most 128 cells. Cell (x, y) maps to bit y*width+x.
// lo holds bits 0-63, hi holds bits 64-127.
type BB struct {
	lo, hi uint64
}

func (b BB) IsZero() bool     { return b.lo == 0 && b.hi == 0 }
func (b BB) And(o BB) BB      { return BB{b.lo & o.lo, b.hi & o.hi} }
func (b BB) Or(o BB) BB       { return BB{b.lo | o.lo, b.hi | o.hi} }
func (b BB) AndNot(o BB) BB   { return BB{b.lo &^ o.lo, b.hi &^ o.hi} }
func (b BB) Intersects(o BB) bool {
	return b.lo&o.lo != 0 || b.hi&o.hi != 0
}
func (b BB) PopCount() int {
	return bits.OnesCount64(b.lo) + bits.OnesCount64(b.hi)
}

// Shl shifts the whole board left by n bits (n must be in [1, 63]).
func (b BB) Shl(n uint) BB {
	return BB{b.lo << n, b.hi<<n | b.lo>>(64-n)}
}

// Shr shifts the whole board right by n bits (n must be in [1, 63]).
func (b BB) Shr(n uint) BB {
	return BB{b.lo>>n | b.hi<<(64-n), b.hi >> n}
}

func BitAt(idx int) BB {
	if idx < 64 {
		return BB{1 << uint(idx), 0}
	}
	return BB{0, 1 << uint(idx-64)}
}

func (b BB) Has(idx int) bool {
	if idx < 64 {
		return b.lo&(1<<uint(idx)) != 0
	}
	return b.hi&(1<<uint(idx-64)) != 0
}

// Compass directions, stable across the whole engine: index into Next tables
// and the move-ordering cache.
const (
	DirUp = iota
	DirDown
	DirLeft
	DirRight
)

var DirName = [4]string{"up", "down", "left", "right"}

// Geometry holds the immutable per-board masks used by every board operation.
// `all` masks off bits beyond width*height; `notRight` excludes the rightmost
// column (cells that would wrap when shifted by 1); `notLeft` mirrors it.
// Next[cell][dir] is the destination cell for a move, or -1 if off-board.
type Geometry struct {
	W, H              int
	all               BB
	notRight, notLeft BB
	Next              [128][4]int16
}

func NewGeometry(w, h int) Geometry {
	g := Geometry{W: w, H: h}
	for i := 0; i < w*h; i++ {
		g.all = g.all.Or(BitAt(i))
	}
	for y := 0; y < h; y++ {
		g.notRight = g.notRight.Or(BitAt(y*w + w - 1))
		g.notLeft = g.notLeft.Or(BitAt(y * w))
	}
	g.notRight = g.all.AndNot(g.notRight)
	g.notLeft = g.all.AndNot(g.notLeft)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := y*w + x
			for d := 0; d < 4; d++ {
				g.Next[c][d] = -1
			}
			if y+1 < h {
				g.Next[c][DirUp] = int16(c + w)
			}
			if y > 0 {
				g.Next[c][DirDown] = int16(c - w)
			}
			if x > 0 {
				g.Next[c][DirLeft] = int16(c - 1)
			}
			if x+1 < w {
				g.Next[c][DirRight] = int16(c + 1)
			}
		}
	}
	return g
}

// Adj returns the union of b shifted one step in each cardinal direction,
// clipped to the board - the whole-board neighbor expansion at the heart of
// every flood-fill/Voronoi wavefront.
func (g *Geometry) Adj(b BB) BB {
	right := b.And(g.notRight).Shl(1)
	left := b.And(g.notLeft).Shr(1)
	up := b.Shl(uint(g.W)).And(g.all)
	down := b.Shr(uint(g.W))
	return right.Or(left).Or(up).Or(down)
}
