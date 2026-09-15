# Project setup

This is a Python 3 Flask Battlesnake server.

## Run

The Replit workflow runs:

```sh
PORT=5000 python main.py
```

The server binds to `0.0.0.0` and exposes the Battlesnake API in the web preview:

- `GET /` — snake metadata
- `POST /start` — game start callback
- `POST /move` — move callback
- `POST /end` — game end callback

Dependencies are declared in `requirements.txt`.