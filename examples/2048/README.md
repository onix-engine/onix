# 2048

A 2048 game built with the Onix module system and HTML/CSS UI, with no external
assets. The example uses the engine repository's Go module and a 540 × 850
logical viewport that scales while preserving its aspect ratio.

From the engine repository root, enter the example directory and run:

```sh
cd examples/2048
go run .
```

- Use the arrow keys, WASD, on-screen direction buttons, or swipe across the board
  to move tiles.
- Matching tiles merge, adding the merged value to the score. Each tile can merge
  only once per move.
- Each valid move spawns a 2 (90% chance) or a 4 (10% chance). Moves that leave the
  board unchanged do not spawn tiles.
- Press Z or click the undo button to restore the previous board, score, and
  random state. One level of undo is available.
- Press R or click the new-game button to restart. The session's best score
  survives restarts and resets when the application exits.
- After reaching 2048, click the continue button or press Enter to keep playing.
  When no moves remain, undo the last move or start a new game.

Project data and HTML live under `resources/`. Game state, merge rules, and input
handling live under `resources/game/`. Fixed-size arrays hold the board, line
buffers, and undo snapshot. Movement logic performs no heap allocations, and
the UI reuses DOM handles to update text and attributes.

From the engine repository root, validate the project, regenerate registration
code, and run the targeted tests:

```sh
go run ./editor/cmd/onix check ./examples/2048
go run ./editor/cmd/onix generate ./examples/2048
go test ./examples/2048 ./examples/2048/resources/game
```

Visual acceptance is manual, following the repository guidelines.
