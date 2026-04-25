# Lessons Learned

- Daily Session-style batch saves should not split SQLite writes across parallel commands when the UI depends on all writes completing together.
- When a user-visible metric such as `Today` depends on persisted data, the write path should surface errors in both logs and UI instead of silently continuing.
- A "done" screen should not present a success state when persistence failed, even if the in-memory phase flow completed.
- AI-dependent screens should guard both at the menu entrypoint and inside the screen model, so future direct navigation paths cannot reintroduce panics.
- Card creation and initial scheduling state are effectively one unit of data in this app, so they should be written atomically rather than as best-effort follow-up calls.
- When a metric mixes per-card state and per-event session logs, both paths need the same day boundary rule; otherwise sessions that cross midnight drift between days.
- If a flat-file setting is keyed by user-editable card text, edit flows need to keep that file synchronized when the key changes, not just when the boolean flag changes.
- For list-style TUI screens, local filtering is often enough and keeps the feature low-risk as long as cursor/offset logic is clamped against the filtered result set.
- Once local filtering exists, local sorting can usually live in the same projection layer; keeping both in one derived list avoids subtle mismatches between what the user sees and what edit/save actions target.
- For resumable multi-phase flows, persisting phase-boundary snapshots is often the simplest reliable cutoff: it preserves meaningful progress without needing to serialize every in-phase cursor and timer.
- When one metric intentionally excludes a behavior (like standalone practice from `Today`), the UI should surface the excluded behavior nearby instead of only documenting what the main metric means.
- When adopting an algorithm that returns precise timestamps, rounding them away at the persistence boundary silently changes the learning behavior more than any parameter tweak would.
