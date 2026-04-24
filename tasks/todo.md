# Daily Session save fix and debug logging

- [x] Confirm root cause of `Today` not updating after Daily Session
- [x] Add deterministic Daily Session save path that updates reviews and closes the session together
- [x] Add minimum human-readable debug logging for Daily Session progress and save failures
- [x] Add regression tests for Daily Session result persistence
- [x] Run the relevant test suite and review results
- [x] Prevent first-run setup from crashing when AI is not configured
- [x] Prevent Compose from crashing when AI is not configured
- [x] Add regression tests for AI-disabled setup/compose flows
- [x] Prevent card creation flows from leaving cards without review rows
- [x] Reuse a single DB helper for card + review creation
- [x] Add regression tests for card creation integrity
- [x] Fix `Today` aggregation to count completed session events by completion day
- [x] Add regression tests for sessions that cross midnight
- [x] Run the relevant test suite and review results

## Review

- Root cause: Daily Session final scoring used parallel DB writes, so `review_sessions.ended_at` could be written while `reviews.reviewed_at` updates silently failed under SQLite write contention.
- Fix: moved Daily Session scoring into one transactional save path and only advanced UI after the save completed.
- Debugging: added `lazyrecall.log` file logging for session start, phase progress, save success, and save failures.
- Review follow-up: phase-progress save failures now surface in the UI, and final-save failure no longer shows a misleading success summary.
- AI-disabled hardening: Setup now offers manual card creation instead of crashing, and Compose is blocked both at entry and in-model when AI is unavailable.
- Card integrity: Add / Fetch / CSV import now create the card and initial review row atomically, so new cards cannot become invisible to due-based study flows.
- Today aggregation: completed Daily Session events now count toward the day the session ended, so sessions that cross midnight no longer land on the previous day.
- Verification: `go test ./...`
