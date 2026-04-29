CREATE TABLE IF NOT EXISTS cards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    front       TEXT    NOT NULL,
    back        TEXT    NOT NULL,
    hint        TEXT    NOT NULL DEFAULT '',
    example             TEXT    NOT NULL DEFAULT '',
    example_translation TEXT    NOT NULL DEFAULT '',
    example_word        TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS batch_stats (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at  DATETIME NOT NULL DEFAULT (datetime('now')),
    topic       TEXT    NOT NULL,
    batch_num   INTEGER NOT NULL,
    total_batches INTEGER NOT NULL,
    rank_start  INTEGER NOT NULL,
    rank_end    INTEGER NOT NULL,
    elapsed_ms  INTEGER NOT NULL,
    success     INTEGER NOT NULL DEFAULT 1,
    error_msg   TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS daily_sessions (
    date         DATE    PRIMARY KEY,
    review_done  INTEGER NOT NULL DEFAULT 0,
    match_done   INTEGER NOT NULL DEFAULT 0,
    reverse_done INTEGER NOT NULL DEFAULT 0,
    blank_done   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS reviews (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id      INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    due_date     DATETIME NOT NULL DEFAULT (datetime('now', 'localtime')),
    interval     INTEGER NOT NULL DEFAULT 1,
    ease_factor  REAL    NOT NULL DEFAULT 2.5,
    repetitions  INTEGER NOT NULL DEFAULT 0,
    last_rating  INTEGER,
    reviewed_at  DATETIME,
    stability    REAL    NOT NULL DEFAULT 0,
    difficulty   REAL    NOT NULL DEFAULT 0,
    fsrs_state   INTEGER NOT NULL DEFAULT 0,
    lapses       INTEGER NOT NULL DEFAULT 0,
    last_review  DATETIME
);

CREATE TABLE IF NOT EXISTS practice_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    mode        TEXT     NOT NULL,
    started_at  DATETIME NOT NULL,
    finished_at DATETIME NOT NULL,
    total       INTEGER  NOT NULL,
    correct     INTEGER  NOT NULL
);

CREATE TABLE IF NOT EXISTS review_sessions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at     DATETIME NOT NULL,
    ended_at       DATETIME,
    mode           TEXT NOT NULL,
    day_session_no INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS review_events (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    review_session_id INTEGER NOT NULL REFERENCES review_sessions(id),
    card_id           INTEGER NOT NULL REFERENCES cards(id),
    position          INTEGER NOT NULL,
    response_time_ms  INTEGER NOT NULL,
    correct           INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS analysis_milestones (
    key        TEXT PRIMARY KEY,
    int_value  INTEGER NOT NULL DEFAULT 0,
    text_value TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS daily_session_mix (
    review_session_id INTEGER PRIMARY KEY REFERENCES review_sessions(id) ON DELETE CASCADE,
    selected_count    INTEGER NOT NULL DEFAULT 0,
    overdue_count     INTEGER NOT NULL DEFAULT 0,
    learning_count    INTEGER NOT NULL DEFAULT 0,
    review_count      INTEGER NOT NULL DEFAULT 0,
    new_count         INTEGER NOT NULL DEFAULT 0,
    final_pass_count  INTEGER NOT NULL DEFAULT 0,
    retry_card_count  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS daily_session_phase_metrics (
    review_session_id INTEGER NOT NULL REFERENCES review_sessions(id) ON DELETE CASCADE,
    phase            TEXT    NOT NULL,
    item_count       INTEGER NOT NULL DEFAULT 0,
    correct_count    INTEGER NOT NULL DEFAULT 0,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    skipped          INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (review_session_id, phase)
);
