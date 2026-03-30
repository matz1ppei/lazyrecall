# poc-anki-claude — Architecture Blueprint

## Overview

A terminal-based spaced-repetition flashcard app (Anki clone) written in Go.
Cards are stored in SQLite. Reviews are scheduled using the SM-2 algorithm.
AI features (hint generation, auto-content fetch) are optional and backed by
either a local Ollama server or the Anthropic Claude API, selected at runtime
via environment variable.

---

## Directory Structure

```
poc-anki-claude/
├── main.go
├── go.mod
├── go.sum
├── .env.example
├── db/
│   ├── schema.sql
│   ├── db.go          # Open / migrate
│   ├── cards.go       # Card CRUD
│   └── reviews.go     # Review CRUD + due-query
├── srs/
│   ├── sm2.go         # SM-2 algorithm
│   └── sm2_test.go
├── ai/
│   ├── client.go      # Client interface + Factory
│   ├── ollama.go      # Ollama implementation (OpenAI-compat)
│   └── claude.go      # Anthropic Claude implementation
├── tui/
│   ├── app.go         # bubbletea root model + screen router
│   ├── home.go        # HomeModel
│   ├── add.go         # AddModel
│   ├── review.go      # ReviewModel
│   └── fetch.go       # FetchModel (AI content generation)
└── importer/
    └── csv.go         # CSV import
```

---

## Technology Stack

| Layer      | Library / Tool                        |
|------------|---------------------------------------|
| Language   | Go 1.22+                              |
| TUI        | github.com/charmbracelet/bubbletea    |
| TUI styles | github.com/charmbracelet/lipgloss     |
| DB         | modernc.org/sqlite (CGO-free)         |
| HTTP       | net/http (stdlib)                     |
| Anthropic  | github.com/anthropics/anthropic-sdk-go |

---

## Database Schema

### Table: `cards`

```sql
CREATE TABLE IF NOT EXISTS cards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    front       TEXT    NOT NULL,
    back        TEXT    NOT NULL,
    hint        TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

### Table: `reviews`

```sql
CREATE TABLE IF NOT EXISTS reviews (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id      INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    due_date     DATE    NOT NULL DEFAULT (date('now')),
    interval     INTEGER NOT NULL DEFAULT 1,   -- days
    ease_factor  REAL    NOT NULL DEFAULT 2.5,
    repetitions  INTEGER NOT NULL DEFAULT 0,
    last_rating  INTEGER,                       -- 0-5
    reviewed_at  DATETIME
);
```

One `reviews` row per card; updated in-place after each review session.

---

## SRS: SM-2 Algorithm

Input: current `(interval, ease_factor, repetitions)` + user rating `q` (0–5).

Rules (standard SM-2):
- If `q < 3`: reset `repetitions = 0`, `interval = 1`
- Else:
  - `repetitions == 0` → `interval = 1`
  - `repetitions == 1` → `interval = 6`
  - else → `interval = round(interval * ease_factor)`
  - `ease_factor = ease_factor + (0.1 - (5-q)*(0.08 + (5-q)*0.02))`
  - clamp `ease_factor` to minimum 1.3
  - increment `repetitions`

Signature:

```go
// srs/sm2.go
type CardState struct {
    Interval    int
    EaseFactor  float64
    Repetitions int
}

func NextState(s CardState, rating int) CardState
```

---

## AI Layer

### Interface (`ai/client.go`)

```go
type Client interface {
    GenerateHint(ctx context.Context, front, back string) (string, error)
    GenerateCard(ctx context.Context, topic string) (front, back, hint string, err error)
}

func NewClient() (Client, error)  // reads env vars, returns nil if unconfigured
```

`NewClient` logic:
1. Read `AI_BACKEND` env var (default: `"ollama"`)
2. If `ollama`: create `OllamaClient` (base URL from `OLLAMA_BASE_URL`, model from `OLLAMA_MODEL`)
3. If `claude`: create `ClaudeClient` (requires `ANTHROPIC_API_KEY`)
4. If neither env var is set or key is missing: return `nil, nil` (AI disabled, not an error)

### OllamaClient (`ai/ollama.go`)

Calls `POST {OLLAMA_BASE_URL}/v1/chat/completions` with the OpenAI-compatible
payload. Uses `net/http` + `encoding/json` only (no extra dependency).

Defaults:
- `OLLAMA_BASE_URL` = `http://localhost:11434`
- `OLLAMA_MODEL`    = `qwen2.5:7b`

### ClaudeClient (`ai/claude.go`)

Uses `github.com/anthropics/anthropic-sdk-go`.
Model: `claude-haiku-3-5` (fast + cheap for hints).

---

## TUI Design (Option B — per-screen independent models)

### Screen routing (`tui/app.go`)

```go
type screen int
const (
    screenHome screen = iota
    screenAdd
    screenReview
    screenFetch
)

type App struct {
    screen  screen
    home    HomeModel
    add     AddModel
    review  ReviewModel
    fetch   FetchModel
    // shared deps injected at construction
    db      *sql.DB
    ai      ai.Client  // may be nil
}
```

`App.Update` switches on `App.screen` and delegates to the active sub-model.
Navigation messages (`MsgGoto{Screen}`) are returned as `tea.Cmd` from
sub-models and handled by `App.Update`.

### HomeModel (`tui/home.go`)

Displays:
- Stats: total cards, due today count
- Menu: `[r] Review`, `[a] Add card`, `[f] Fetch with AI`, `[i] Import CSV`, `[q] Quit`

### AddModel (`tui/add.go`)

Multi-step text-input form: Front → Back → Hint (optional).
On confirm: `db.CreateCard(front, back, hint)`.
If AI client is available, offer `[g]` to auto-generate hint from front+back.

### ReviewModel (`tui/review.go`)

Fetch all due cards from DB at load time.
For each card:
1. Show front
2. User presses space → reveal back + hint
3. User rates 0–5 (keys `0`–`5`)
4. Call `srs.NextState`, update review row in DB
5. Advance to next card or show summary screen

### FetchModel (`tui/fetch.go`)

Text input: user types a topic.
On submit: call `ai.Client.GenerateCard(topic)` → show preview → confirm → save to DB.
Show spinner while AI call is in flight (use `tea.Cmd` + channel).

---

## CSV Import (`importer/csv.go`)

Expected columns (header required): `front,back,hint`
`hint` column is optional; empty value is allowed.

```go
func ImportCSV(db *sql.DB, path string) (imported int, err error)
```

---

## Environment Variables

| Variable           | Default                   | Required |
|--------------------|---------------------------|----------|
| `AI_BACKEND`       | `ollama`                  | No       |
| `OLLAMA_BASE_URL`  | `http://localhost:11434`  | No       |
| `OLLAMA_MODEL`     | `qwen2.5:7b`              | No       |
| `ANTHROPIC_API_KEY`| —                         | Only for `claude` backend |

`.env.example`:
```
AI_BACKEND=ollama
OLLAMA_BASE_URL=http://localhost:11434
OLLAMA_MODEL=qwen2.5:7b
# ANTHROPIC_API_KEY=sk-ant-...
```

---

## Error Handling Conventions

- AI errors are non-fatal: log to stderr, show user-visible message in TUI, continue.
- DB errors at startup are fatal (`log.Fatal`).
- DB errors during review update are logged and surfaced in TUI; review session continues.

---

## Testing Strategy

- `srs/sm2_test.go`: table-driven unit tests for boundary ratings (0, 2, 3, 5) and multi-step sequences.
- `db/` functions: tested via in-memory SQLite (`file::memory:?cache=shared`).
- AI layer: tested with an interface mock (`ai/mock.go` generated only for tests).
- TUI: not unit-tested; verified via manual smoke test checklist in `todo.md`.
