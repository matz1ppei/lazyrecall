# Daily Session save fix and debug logging

- [x] Confirm root cause of `Today` not updating after Daily Session
- [x] Add deterministic Daily Session save path that updates reviews and closes the session together
- [x] Add minimum human-readable debug logging for Daily Session progress and save failures
- [x] Add regression tests for Daily Session result persistence
- [x] Run the relevant test suite and review results

## Review

- Root cause: Daily Session final scoring used parallel DB writes, so `review_sessions.ended_at` could be written while `reviews.reviewed_at` updates silently failed under SQLite write contention.
- Fix: moved Daily Session scoring into one transactional save path and only advanced UI after the save completed.
- Debugging: added `lazyrecall.log` file logging for session start, phase progress, save success, and save failures.
- Review follow-up: phase-progress save failures now surface in the UI, and final-save failure no longer shows a misleading success summary.
- Verification: `go test ./...`
