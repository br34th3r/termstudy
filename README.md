# termstudy

A terminal study tool: browse large markdown notes, edit them in your `$EDITOR`
(vim), and review flashcards with SM-2 spaced repetition. Built for a
Claude-assisted workflow — ask Claude to generate study notes and decks, then
study them from the terminal (great in a tmux pane next to vim).

## Install

```sh
go build -o termstudy .
# optionally: go install .
```

## Run

```sh
termstudy              # launch the TUI
termstudy -paths       # print the data directories and exit
termstudy -notes ./n -decks ./d   # override locations
```

On first run it creates `~/.termstudy/{notes,decks}` with a sample note and
deck.

## Screens

- **Notes** — a file list on the left, live markdown preview on the right.
  - `↑/↓` (or `j/k`) move between notes, `J/K` scroll the preview
  - `e` / `enter` open the note in `$EDITOR`; it reloads when you exit
  - `r` rescan the notes directory, `esc` back
- **Review** — pick a deck, study its due cards.
  - `space` / `enter` reveal the answer
  - grade `1` Again · `2` Hard · `3` Good · `4` Easy
  - progress saves after every card; `esc` ends the session

## The Claude workflow

1. **Notes:** ask Claude to write study material as markdown into
   `~/.termstudy/notes/` (subfolders are fine). Press `r` in the Notes screen
   to pick them up.
2. **Decks:** ask Claude to generate a flashcard deck as a JSON file in
   `~/.termstudy/decks/`. You only need `name` and each card's `front`/`back` —
   termstudy fills in and persists the scheduling state on first load.

### Deck format

```json
{
  "name": "Cell Biology",
  "cards": [
    { "front": "What is the powerhouse of the cell?", "back": "The mitochondrion." },
    { "front": "What does ATP stand for?", "back": "Adenosine triphosphate." }
  ]
}
```

Card faces may contain markdown. After review, termstudy adds scheduling fields
(`id`, `ease`, `interval`, `reps`, `lapses`, `due`) to each card — leave them be.

## Spaced repetition

Scheduling uses the SM-2 algorithm (as popularized by SuperMemo/Anki). New cards
are due immediately; grading **Again** requeues the card within the session and
resets its interval. A card is "due" when its `due` date is today or earlier.
```
