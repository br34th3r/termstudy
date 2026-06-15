# termstudy — notes for Claude

A Go (Bubble Tea) terminal app for studying markdown notes and SM-2 flashcards.

## Layout

- `main.go` — entry point, flags (`-notes`, `-decks`, `-paths`), program setup.
- `config.go` — resolves `~/.termstudy/{notes,decks}`, first-run seeding, note
  CRUD (`LoadNotes`, `CreateNote`, `RenameNote`, `DeleteNote`), and the **field**
  model (`Field`, `LoadFields`, `Config.ForField`, `Config.CreateField`,
  `Config.RenameField`, `Config.DeleteField`, loose-file migration).
- `srs.go` — `Card` type and the SM-2 algorithm (`Card.Review`, `Card.IsDue`). Pure logic.
- `store.go` — `Deck` type, loading/saving JSON decks, due-card selection, and
  CRUD (`CreateDeck`, `Deck.Rename`, `Deck.Delete`, `Deck.AddCard`,
  `Deck.UpdateCard`, `Deck.RemoveCard`).
- `ui.go` — root model: screen routing, main menu, shared styles/helpers.
- `fields.go` — field picker (entry screen): lists subjects; `n` new, `e`
  rename, `d` delete (confirmed).
- `notes.go` — notes browser (list + glamour preview); `e` edit in `$EDITOR`,
  `a` new, `R` rename, `d` delete (confirmed).
- `decks.go` — deck list with due counts; `enter` review, `c` manage cards, `n`
  new, `e` rename, `d` delete (confirmed).
- `cards.go` — per-deck card manager (`screenCards`): lists a deck's cards and
  supports `a` add, `e` edit, `d` delete via a front/back form.
- `review.go` — review session UI (reveal, grade, requeue, persist).
- `srs_test.go` — tests for SM-2 progression and deck round-tripping.
- `config_test.go` — tests for field seeding, migration, creation, and scoping.

## Fields

Notes and decks are organized into **fields** (subjects, e.g. CyberSecurity,
Spanish). A field is a subdirectory shared by the notes and decks trees:
`~/.termstudy/notes/<field>` and `~/.termstudy/decks/<field>`. The app opens on
the field picker (`screenFields`); selecting a field hands the notes/decks
sub-models a `Config.ForField(field)`-scoped config, then routes to the menu.
On first run a `general` field is seeded; pre-field loose top-level files are
migrated into `general` on startup so nothing is hidden.

## Conventions

- Sub-models follow a `setSize(w,h)`, `update(msg) (model, cmd)`, `view() string`
  pattern; the root model in `ui.go` owns them and routes by `screen`.
- Cross-screen navigation uses `tea.Cmd`s emitting custom messages
  (`switchMsg`, `startReviewMsg`, `reloadDecksMsg`, `manageCardsMsg`).
- List screens share a CRUD keybinding convention: `a` add, `e` edit/rename,
  `d` delete. Destructive deletes prompt for a `y`/any-key confirmation rendered
  in the help bar (a `confirming` flag on the sub-model). Notes keep `e` for
  "edit in `$EDITOR`" and use `R` for rename.
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
