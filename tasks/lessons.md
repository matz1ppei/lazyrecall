# Lessons Learned

- Daily Session-style batch saves should not split SQLite writes across parallel commands when the UI depends on all writes completing together.
- When a user-visible metric such as `Today` depends on persisted data, the write path should surface errors in both logs and UI instead of silently continuing.
- A "done" screen should not present a success state when persistence failed, even if the in-memory phase flow completed.
- AI-dependent screens should guard both at the menu entrypoint and inside the screen model, so future direct navigation paths cannot reintroduce panics.
- Card creation and initial scheduling state are effectively one unit of data in this app, so they should be written atomically rather than as best-effort follow-up calls.
- When a metric mixes per-card state and per-event session logs, both paths need the same day boundary rule; otherwise sessions that cross midnight drift between days.
