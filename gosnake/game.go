package main

const MaxSnakes = 8

// Snake state during search. Body cells live in a circular buffer (tail..head
// order); duplicates only ever occur at the tail end, because the official
// rules implement growth as "drop last segment, then duplicate the new last".
type Snake struct {
	buf        [128]uint8
	headPos    int // buffer slot of the head
	tailPos    int // buffer slot of the tail (oldest segment)
	Length     int
	Health     int
	Body       BB // occupancy of every segment
	HeadCell   int
	Alive      bool
}

const bufMask = 127

func (s *Snake) TailCell() int { return int(s.buf[s.tailPos]) }

// undo holds what make() changed, so unmake() can restore it exactly.
type undo struct {
	prevHealth   int
	poppedTail   uint8
	clearedTail  bool
	ate          bool
	prevHeadCell int
}

type Board struct {
	Geo    Geometry
	Snakes [MaxSnakes]Snake
	N      int // number of snakes loaded (dead ones keep their slot)
	Food   BB
}

// Make advances one snake by one step onto cell `to`, which the caller has
// already verified is in bounds and not a body collision. Returns the undo
// record. Handles tail pop (with duplicate awareness), food, growth, health.
func (b *Board) Make(s *Snake, to int) undo {
	u := undo{prevHealth: s.Health, prevHeadCell: s.HeadCell}

	// Pop the tail. Only clear its occupancy bit if the segment before it
	// isn't the same cell (i.e. it wasn't a growth duplicate).
	u.poppedTail = s.buf[s.tailPos]
	s.tailPos = (s.tailPos + 1) & bufMask
	if s.buf[s.tailPos] != u.poppedTail {
		s.Body = s.Body.AndNot(BitAt(int(u.poppedTail)))
		u.clearedTail = true
	}

	// Push the new head.
	s.headPos = (s.headPos + 1) & bufMask
	s.buf[s.headPos] = uint8(to)
	s.Body = s.Body.Or(BitAt(to))
	s.HeadCell = to
	s.Health--

	// Feed: health to full, grow by duplicating the current last segment.
	if b.Food.Has(to) {
		u.ate = true
		b.Food = b.Food.AndNot(BitAt(to))
		s.Health = 100
		s.Length++
		s.tailPos = (s.tailPos - 1) & bufMask
		s.buf[s.tailPos] = s.buf[(s.tailPos+1)&bufMask]
	}

	if s.Health <= 0 {
		s.Alive = false
	}
	return u
}

func (b *Board) Unmake(s *Snake, u undo) {
	if u.ate {
		s.Length--
		s.tailPos = (s.tailPos + 1) & bufMask
		b.Food = b.Food.Or(BitAt(s.HeadCell))
	}

	// Pop the head we pushed.
	s.Body = s.Body.AndNot(BitAt(s.HeadCell))
	s.headPos = (s.headPos - 1) & bufMask
	s.HeadCell = u.prevHeadCell
	// The previous head cell is still a body segment, so its bit must be set
	// (it never went away - only the head marker moved).
	s.Body = s.Body.Or(BitAt(u.prevHeadCell))

	// Restore the tail we popped.
	s.tailPos = (s.tailPos - 1) & bufMask
	s.buf[s.tailPos] = u.poppedTail
	if u.clearedTail {
		s.Body = s.Body.Or(BitAt(int(u.poppedTail)))
	}

	s.Health = u.prevHealth
	s.Alive = true
}

// AllBodies is the union of every live snake's occupancy.
func (b *Board) AllBodies() BB {
	var out BB
	for i := 0; i < b.N; i++ {
		if b.Snakes[i].Alive {
			out = out.Or(b.Snakes[i].Body)
		}
	}
	return out
}

// --- Battlesnake API request parsing ---

type apiCoord struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type apiSnake struct {
	ID     string     `json:"id"`
	Health int        `json:"health"`
	Body   []apiCoord `json:"body"`
}

type apiBoard struct {
	Width  int        `json:"width"`
	Height int        `json:"height"`
	Food   []apiCoord `json:"food"`
	Snakes []apiSnake `json:"snakes"`
}

type apiGame struct {
	ID      string `json:"id"`
	Timeout int    `json:"timeout"`
}

type GameState struct {
	Game  apiGame  `json:"game"`
	Turn  int      `json:"turn"`
	Board apiBoard `json:"board"`
	You   apiSnake `json:"you"`
}

// LoadBoard converts an API request into search state. Our snake always lands
// in slot 0 - eval and search rely on that. Returns false if the board doesn't
// fit in 128 bits (caller should fall back to something simple).
func LoadBoard(gs *GameState) (*Board, bool) {
	w, h := gs.Board.Width, gs.Board.Height
	if w*h > 128 || len(gs.Board.Snakes) > MaxSnakes {
		return nil, false
	}
	b := &Board{Geo: NewGeometry(w, h)}
	idx := func(c apiCoord) int { return c.Y*w + c.X }

	for _, f := range gs.Board.Food {
		b.Food = b.Food.Or(BitAt(idx(f)))
	}

	load := func(slot int, as *apiSnake) {
		s := &b.Snakes[slot]
		s.Alive = true
		s.Health = as.Health
		s.Length = len(as.Body)
		// API body is head-first; the buffer wants tail..head order.
		for i := len(as.Body) - 1; i >= 0; i-- {
			cell := idx(as.Body[i])
			s.headPos = (s.headPos + 1) & bufMask
			s.buf[s.headPos] = uint8(cell)
			s.Body = s.Body.Or(BitAt(cell))
		}
		s.tailPos = 1 // first push above landed at slot 1 (headPos started at 0)
		s.HeadCell = idx(as.Body[0])
	}

	load(0, &gs.You)
	slot := 1
	for i := range gs.Board.Snakes {
		if gs.Board.Snakes[i].ID == gs.You.ID {
			continue
		}
		load(slot, &gs.Board.Snakes[i])
		slot++
	}
	b.N = slot
	return b, true
}
