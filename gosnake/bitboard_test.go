package main

import "testing"

func TestShiftCrossesWordBoundary(t *testing.T) {
	b := BitAt(63)
	if got := b.Shl(1); !got.Has(64) || got.PopCount() != 1 {
		t.Fatalf("Shl across boundary: got %+v", got)
	}
	b = BitAt(64)
	if got := b.Shr(1); !got.Has(63) || got.PopCount() != 1 {
		t.Fatalf("Shr across boundary: got %+v", got)
	}
	b = BitAt(60)
	if got := b.Shl(11); !got.Has(71) || got.PopCount() != 1 {
		t.Fatalf("Shl(11) across boundary: got %+v", got)
	}
	b = BitAt(70)
	if got := b.Shr(11); !got.Has(59) || got.PopCount() != 1 {
		t.Fatalf("Shr(11) across boundary: got %+v", got)
	}
}

func TestAdjCenterEdgesCorners(t *testing.T) {
	g := NewGeometry(11, 11)
	idx := func(x, y int) int { return y*11 + x }

	// Center cell: 4 neighbors.
	adj := g.Adj(BitAt(idx(5, 5)))
	if adj.PopCount() != 4 {
		t.Fatalf("center adj count = %d, want 4", adj.PopCount())
	}
	for _, c := range [][2]int{{4, 5}, {6, 5}, {5, 4}, {5, 6}} {
		if !adj.Has(idx(c[0], c[1])) {
			t.Fatalf("center adj missing (%d,%d)", c[0], c[1])
		}
	}

	// Left edge: no wrap to the previous row's right edge.
	adj = g.Adj(BitAt(idx(0, 5)))
	if adj.PopCount() != 3 {
		t.Fatalf("left edge adj count = %d, want 3", adj.PopCount())
	}
	if adj.Has(idx(10, 4)) {
		t.Fatal("left edge wrapped around to right edge")
	}

	// Right edge: no wrap to the next row's left edge.
	adj = g.Adj(BitAt(idx(10, 5)))
	if adj.PopCount() != 3 {
		t.Fatalf("right edge adj count = %d, want 3", adj.PopCount())
	}
	if adj.Has(idx(0, 6)) {
		t.Fatal("right edge wrapped around to left edge")
	}

	// Corners: 2 neighbors, including the top-right cell crossing the word
	// boundary (cell (10,10) = bit 120).
	for _, c := range [][2]int{{0, 0}, {10, 0}, {0, 10}, {10, 10}} {
		adj = g.Adj(BitAt(idx(c[0], c[1])))
		if adj.PopCount() != 2 {
			t.Fatalf("corner (%d,%d) adj count = %d, want 2", c[0], c[1], adj.PopCount())
		}
	}

	// Top row: shifting up must not escape the board mask.
	adj = g.Adj(BitAt(idx(5, 10)))
	if adj.PopCount() != 3 {
		t.Fatalf("top edge adj count = %d, want 3", adj.PopCount())
	}
}

func TestFloodFillViaAdjCoversBoard(t *testing.T) {
	g := NewGeometry(11, 11)
	seen := BitAt(0)
	for {
		next := seen.Or(g.Adj(seen))
		if next == seen {
			break
		}
		seen = next
	}
	if seen.PopCount() != 121 {
		t.Fatalf("flood fill from corner reached %d cells, want 121", seen.PopCount())
	}
}
