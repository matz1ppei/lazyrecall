# Lessons Learned

- Daily Session-style batch saves should not split SQLite writes across parallel commands when the UI depends on all writes completing together.
- When a user-visible metric such as `Today` depends on persisted data, the write path should surface errors in both logs and UI instead of silently continuing.
- A "done" screen should not present a success state when persistence failed, even if the in-memory phase flow completed.
