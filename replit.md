# Project setup

This repo contains two Battlesnake engines:

- **`gosnake/` — the Go engine (what gets deployed).** Bitboard state,
  Voronoi eval, full N-player paranoid minimax.
- **`main.py` — the Python engine (reference implementation).** The Go
  engine is a port of its validated heuristics; keep it runnable for
  A/B comparisons (`PORT=5000 python main.py`).

## Run

The Replit workflow builds and runs the Go engine:

```sh
cd gosnake && go build -o /tmp/gosnake . && PORT=5000 /tmp/gosnake
```

Deployment does NOT compile: it runs a prebuilt static Linux binary
from `bin/`, picked by architecture. After changing the engine, rebuild
both binaries (commands are in the `.replit` deployment comment) and
commit them.

The server binds `0.0.0.0` and exposes the Battlesnake API:

- `GET /` — snake metadata
- `POST /start` — game start callback
- `POST /move` — move callback
- `POST /end` — game end callback

## Tests

```sh
cd gosnake && go test ./...
```
