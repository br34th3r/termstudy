# termstudy — notes for Claude

A Go (Bubble Tea) terminal app for studying markdown notes and SM-2 flashcards.

## Layout

- `main.go` — entry point, flags (`-notes`, `-decks`, `-paths`), program setup.
- `config.go` — resolves `~/.termstudy/{notes,decks}`, first-run seeding, note discovery.
- `srs.go` — `Card` type and the SM-2 algorithm (`Card.Review`, `Card.IsDue`). Pure logic.
- `store.go` — `Deck` type, loading/saving JSON decks, due-card selection.
- `ui.go` — root model: screen routing, main menu, shared styles/helpers.
- `notes.go` — notes browser (list + glamour preview, shell-out to `$EDITOR`).
- `decks.go` — deck list with due counts, launches review sessions.
- `review.go` — review session UI (reveal, grade, requeue, persist).
- `srs_test.go` — tests for SM-2 progression and deck round-tripping.

## Conventions

- Sub-models follow a `setSize(w,h)`, `update(msg) (model, cmd)`, `view() string`
  pattern; the root model in `ui.go` owns them and routes by `screen`.
- Cross-screen navigation uses `tea.Cmd`s emitting custom messages
  (`switchMsg`, `startReviewMsg`, `reloadDecksMsg`).
- Decks persist after every grade; `Save` writes atomically via a temp file.
- Hand-authored cards need only `front`/`back`; `Card.normalize` backfills the
  rest and the change is saved back to disk.

## Build / test

```sh
go build ./...
go test ./...
go vet ./...
```

The TUI needs a real TTY; it can't be driven in a plain pipe. Use `-paths` for a
non-interactive sanity check.
