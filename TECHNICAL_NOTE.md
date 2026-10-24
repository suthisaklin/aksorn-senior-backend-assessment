# Technical Note

## 1. Design decision

Progress is computed on every read from all stored attempts, in Go: one `List` call,
then the highest score per lesson in a map. The loop walks `FixedCourse()` rather
than the attempts, so every lesson appears even with no attempts, and the map's
`seen` flag keeps "no attempt" (`null`) apart from a real score of 0.
Tradeoff: there is no second copy of the data to fall out of sync, but each read
transfers every attempt the learner has made, so cost grows with history.

## 2. Design questions

### Retried requests

Add a `UNIQUE (attempt_id)` constraint (after removing existing duplicates), then
`INSERT ... ON CONFLICT (attempt_id) DO NOTHING RETURNING id`. If no row comes
back, read the stored attempt: same content → return it as the original result;
different content → 409 (`ErrConflict` already exists). For simultaneous requests
the unique index decides: the second insert waits for the first to commit, then
conflicts and takes the replay path, so only one row is ever stored.

### Concurrent requests

No. Each attempt is a separate `INSERT` of a new row; nothing is read-modify-written,
so neither write can overwrite the other. A progress read is one `SELECT`, so it sees
neither, one, or both attempts as committed at that moment (best score unchanged,
70 or 90). Any read after both commits returns 90. No extra protection is needed.

### Redis

Not as the first step. First compute in SQL (`MAX(score) GROUP BY lesson_id`) with
an index on `(learner_id, course_id, lesson_id, score)`, which returns at most three
rows. If that is still slow, cache the computed progress per learner and course
with a short TTL, deleted after each successful insert. PostgreSQL stays the source
of truth: if Redis is down or loses data, reads fall back to PostgreSQL and only
get slower. A stale entry can survive a race between a read and a delete until
its TTL ends.

## 3. Limitations and production readiness

- `attempt_id` is not unique yet: a retried request stores a second row (it does not
  change the best score, but the history is wrong).
- The percent divides by the number of required lessons; a course with none would
  panic. The fixed course has two.
- Tests need the PostgreSQL from setup; there are no HTTP tests, and golangci-lint
  was not run.
- Before production: idempotency as above, query plans and load tests with large
  histories, timeouts on database calls, and logging and metrics for errors.

## 4. Time and AI tools

Time spent: 2 hours 16 minutes

AI tools used: Claude Code, to review the service code, draft the application tests
and check they fail when the rules are broken, and draft this note. I reviewed and
can explain all of the submitted code.
