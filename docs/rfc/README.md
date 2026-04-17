# QCore RFCs

Architectural decisions that affect the project's direction live here.
Lower-stakes decisions live in code review and commit messages.

## When to write an RFC

Write one when a decision is:

- **Hard to reverse** — changes the service boundaries, wire protocol,
  persistence model, or public API shape.
- **Controversial** — a reasonable engineer could land on the opposite
  conclusion and you want to record why.
- **Load-bearing for future work** — the choice will show up in many
  future PRs and you want one place to point at.

Not every PR needs an RFC. A new REST endpoint does not. Switching from
PostgreSQL to CockroachDB does.

## Process

1. Copy `_template.md` to `NNNN-short-title.md` with the next free number.
2. Open a draft PR with `Status: Proposed`.
3. Discussion happens in the PR thread, not in comments on the file.
4. When merged, the status flips to `Accepted`. Later RFCs may supersede it.
5. If reality diverges, amend with a `## Amendment — YYYY-MM-DD` section.
   Never rewrite history.

## Index

| # | Title | Status |
|---|-------|--------|
| 0001 | [5G SBA-First Pivot with Refactor-Not-Rebuild](0001-5g-sba-pivot.md) | Proposed |
