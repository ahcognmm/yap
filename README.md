# yap

*A Jupyter-style TUI for Markdown. Your README can run now. Yes, that's a threat.*

It's called `yap` because your runbook has been talking a big game for years — pages and pages
of confident instructions, none of which have ever been made to prove anything. Now the talk is
executable. All yap, and finally some do.

## What is this

You know that file. `SETUP.md`. `RUNBOOK.md`. `docs/how-to-deploy-do-not-delete-FINAL-v2.md`.
It's full of code blocks. Every time you need it, you do The Ritual:

1. Open the file in an editor.
2. Select a code block with the mouse like an animal.
3. Cmd-C.
4. Alt-tab to the terminal.
5. Cmd-V.
6. Realize you grabbed the leading `$`.
7. Repeat 47 times, developing a mild personality disorder.

`yap` deletes steps 1 through 7. Open the directory, arrow to the block, hit Enter. It runs.
Output streams in underneath it. Exit code and wall-clock time show up in the corner. The
Markdown file is never modified, because your documentation is documentation, not a scratchpad.

It's a notebook, except the notebook format is "a Markdown file that was already sitting in
your repo doing nothing."

## Install

```sh
git clone https://github.com/ahcognmm/yap
cd yap
go build -o yap .
```

Needs Go 1.26+ and a real terminal. If you pipe it somewhere, it refuses to start and tells you
so, because a TUI writing escape codes into your `less` buffer helps nobody.

## Use

```sh
yap                      # current directory
yap ./docs               # some other directory
yap RUNBOOK.md           # straight to one file
yap --theme catppuccin   # for the pastel-inclined
yap --ignore dist,.venv  # on top of .git/node_modules/vendor/target
```

Left panel is the Explorer — a lazy file tree that only reads a directory when you actually
expand it, and that won't walk in circles if you've built a symlink ouroboros. Markdown files
are tinted blue so you can find them among the noise.

Right panel is the Document — prose rendered nicely via Glamour, code fences sitting there as
selectable, runnable cells.

Fences tagged `sh`, `bash`, `zsh`, or `shell` are runnable. Everything else gets its own tidy
bordered box and a firm refusal to execute. Your `json` block is not a program. Let it go.

## Keys

Press `?` in the app; the footer is generated from the actual keymap, so unlike this table it
is physically incapable of lying to you.

**Everywhere:** `tab` switch panel · `ctrl+b` hide/show the tree · `/` filter · `g`/`G` top and
bottom · `?` help · `q` quit

**Explorer:** `j`/`k` move · `enter` expand a folder or open a file

**Document:** `j`/`k` select a block · `ctrl+u`/`ctrl+d` scroll · `enter` or `r` run ·
`R` run in parallel · `ctrl+c` cancel · `y` copy output · `e` open the file in `$EDITOR` ·
`:` run an ad-hoc command · `ctrl+e` scratch buffer

`shift+enter` is also bound to parallel-run, and it works great in exactly the terminals that
speak the Kitty keyboard protocol. In every other terminal, `shift+enter` and `enter` are
indistinguishable bytes on a wire, and no amount of wanting changes that. `R` always works.
`R` is your friend.

## The part that's actually interesting

Every runnable block in a document runs in **the same `sh` process**. Not a fresh subshell per
block — one long-lived shell, like a notebook kernel:

````markdown
```sh
export DEPLOY_ENV=staging
cd ./services
```

```sh
echo "deploying to $DEPLOY_ENV from $(pwd)"
# -> deploying to staging from /your/repo/services
```
````

State carries. `export` sticks. `cd` sticks. That virtualenv you activated in block one is
still activated in block six. This is the whole point of the project and everything else is
scaffolding around it.

Blocks queue and run strictly in order, because a shell can only do one thing at a time and
pretending otherwise is how you get haunted.

### Escape hatches, in ascending order of chaos

- **`R` — parallel run.** Spawns a throwaway shell in its own process group. For the block that
  starts a dev server and never returns, which would otherwise hold the shared shell hostage
  forever. The tradeoff is symmetric and non-negotiable: a detached block gets a fresh copy of
  the environment and contributes nothing back to the shared one. Separate process, separate
  universe.

- **`:` — the console.** Type a command, it runs in the shared shell, same as any block. This is
  where you `export AWS_PROFILE=whatever` before running the deploy steps, instead of editing
  the runbook to hardcode a secret and then committing it and then having a conversation with
  the security team.

- **`ctrl+e` — the scratch buffer.** Suspends the TUI, drops you into `$EDITOR` with a temp
  `.sh` file, and runs whatever you saved in the shared session when you quit out. For when
  the ad-hoc command is a whole loop with a heredoc in it and the single-line console starts
  feeling like a hostage situation.

### The SIGINT trap, or: how `ctrl+c` doesn't nuke your session

Cancelling sends `SIGINT` to the whole process group, so it actually reaches the `curl` your
shell is blocked on. Problem: a non-interactive shell's default response to `SIGINT` is to
*die*, which would take your entire accumulated session with it every single time you cancelled
anything.

So the session installs `trap ':' INT` at startup — a *caught* signal, deliberately not
`SIG_IGN`. A caught trap resets to default disposition across `exec`, so every command the
shell launches afterward still dies from `ctrl+c` like a normal citizen, while the shell itself
shrugs it off and keeps your environment intact.

Then it blocks until it sees a marker confirming the trap is live, because otherwise a
sufficiently caffeinated user could hit `ctrl+c` in the microseconds before the trap installed
and kill the session on its very first use. Someone thought about this for longer than they'd
like to admit. See `internal/exec/runner.go` for the full comment, written in the tone of a
person who has been through something.

Completion is tracked by injecting a `printf` of a unique marker plus `$?` after each script —
captured immediately, before `printf` itself can clobber the exit status. stdout and stderr
share a single pipe so interleaved output keeps its real ordering, because reading two pipes
concurrently and hoping for the best is not ordering, it's vibes.

## Project layout

```
main.go                 CLI wiring, arg validation, "is this even a terminal"
internal/exec/          the shell session, the markers, the trap, the regret
internal/markdown/      goldmark -> ordered prose/code blocks
internal/fstree/        lazy directory scanning, symlink cycle guard
internal/ui/            Bubble Tea models, views, keymaps, console, overlay
internal/theme/         semantic color tokens, so no view file ever hardcodes a hex
```

Roughly 2,900 lines. `exec` and `fstree` are deliberately Bubble-Tea-free — they return plain
channels and structs, and the UI layer wraps them in `tea.Cmd` itself. This is good design and
also makes them trivially testable, which brings us to:

## Tests

There are none.

Not "coverage is a little thin." Zero test files. The pure, Bubble-Tea-free packages that were
carefully architected to be easy to test have been rewarded for this with nothing at all.

If you were looking for a first contribution, `internal/markdown/parse.go` is a pure function
from `[]byte` to `[]Block` and is basically begging you.

## Notes for the curious

The source references `DESIGN.md` in four separate places. `DESIGN.md` is not in this
repository. It may have existed. It may have been aspirational. `internal/ui/filter.go` cites
"DESIGN.md §9 open question #5" with total confidence, and the citation resolves to the void.

We choose to read this as a feature: the design doc achieved such perfect enlightenment that it
transcended the filesystem. The code still does the thing it says, which is filter visible
nodes only rather than recursively auto-expanding the tree.

Built with [Bubble Tea](https://charm.land), [Glamour](https://github.com/charmbracelet/glamour),
[goldmark](https://github.com/yuin/goldmark), and [urfave/cli](https://github.com/urfave/cli).
Themes are the bundled dark default and Catppuccin Mocha, because it is a law.
