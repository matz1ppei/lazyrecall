CREATE TABLE IF NOT EXISTS cards (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    front       TEXT    NOT NULL,
    back        TEXT    NOT NULL,
    hint        TEXT    NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reviews (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    card_id      INTEGER NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    due_date     DATE    NOT NULL DEFAULT (date('now')),
    interval     INTEGER NOT NULL DEFAULT 1,
    ease_factor  REAL    NOT NULL DEFAULT 2.5,
    repetitions  INTEGER NOT NULL DEFAULT 0,
    last_rating  INTEGER,
    reviewed_at  DATETIME
);
