# tmux_s

Build a tmux session from a small YAML file and attach to it.

```
tmux_s                      list the sessions that are defined
tmux_s my_session           build (or attach to) the session named roblox
tmux_s my_session --print   show the tmux commands without running any
tmux_s my_session --detach  build the session but stay where you are
tmux_s save my_session      write a running session out as a session file
tmux_s update               update to the latest release
```

A session that is already running is attached to as-is, never rebuilt. Attaching from
outside tmux replaces the process with `tmux attach-session`; from inside, it is a
`switch-client`.

Nothing is generated and no shell is involved: a session file becomes a list of tmux
argv slices run directly. A `send-keys` payload is one argument, so `&&`, quotes and
`$` in a command line need no escaping — write the line you want typed.

## Session files

Every `*.yaml` under the configured source directories. The session's name is the
filename, so `~/.tmux_s/sessions/roblox.yaml` defines `roblox`.

```yaml
dir: ~/some/dir             # optional; default cwd. Inherited by windows and panes.
windows:
  remote:
    keys: echo "don't run this command"
    enter: false            # type it into the pane, don't press Enter
    focus: true             # the window selected on attach
  notes:                    # no numbered keys -> a single pane
    keys: gote my_vault --vault
  ports:
    layout: even-horizontal # tiled | even-horizontal | even-vertical | main-vertical | ...
    0:                      # numbered keys -> panes, created in index order
      keys: ssh -L 34783:localhost:34783 my_server_
    1:
      dir: ~/logs
      keys:                 # a list -> one send-keys per line
        - clear
        - tail -f out.log
      focus: true           # the pane selected within this window
  term: cd ~/another/dir        # a bare string is shorthand for `keys:`
```

| key | on | meaning |
| --- | --- | --- |
| `session` | session | overrides the filename as the session's name |
| `dir` | session, window, pane | `-c` working directory; falls pane → window → session |
| `windows` | session | mapping of window name to window, in creation order |
| `layout` | window | tmux layout, applied after the splits. Default `tiled` |
| `focus` | window, pane | selected on attach. Default: the first window, first pane |
| `keys` | pane | one line, or a list of lines, sent to the pane |
| `enter` | pane | `false` types the line without running it. Default `true` |

Two formats are accepted. `session`, `dir` or `windows` at the top
level defines a single session. Any other top-level mapping is read as session name → session config,
so several sessions can share one file:

```yaml
session1:
  windows:
    remote: ssh my_server
session2:
  dir: ~/some/dir
  windows:
    files: ls
```

Within a window, integer keys are panes and string keys are the window's own fields. A
window mapping with no integer keys describes that window's single pane, so `keys` and
`enter` sit directly on it. Mixing the two is an error.

`~` and `$VAR` are expanded in every `dir`, tmux's `-c` takes a literal path and
nothing here passes through a shell that would expand them.

## Saving a running session

`tmux_s save <session> [name|path]` goes the other way: it reads a session you built by hand
and writes the file that rebuilds it.

```
tmux_s save go-dev                     ~/.tmux_s/sessions/go-dev.yaml
tmux_s save go-dev backup              ~/.tmux_s/sessions/backup.yaml, as `backup`
tmux_s save go-dev ~/my-session.yaml   somewhere else
tmux_s save go-dev ~/dotfiles/sessions a directory, so go-dev.yaml inside it
tmux_s save go-dev --overwrite         replace the file that is already there
```

A bare second argument is a **session name**, not a file in the current directory. It
lands in the sessions directory beside the rest *and renames the copy*, so
`tmux_s save go-dev backup` gives you a `backup` you can actually build — a second file
still called `go-dev` would only shadow the original, and `--list` would show two of them
where one is unreachable. A **path** says where the file goes and leaves the name alone.

A missing `.yaml` is added either way: only `.yaml` and `.yml` are scanned, so a file
saved without one would sit there being ignored.

It captures the windows and their order, the names, the panes in each, the layout that
places them, every pane's working directory, and which window and pane are active. The
layout is tmux's own layout string, so the split offsets come back exactly as they were.
Directories are written at the highest level that covers them and contracted to `~`, so
the file stays readable and portable.

It does **not** capture commands. tmux only knows what a pane is running *now* — a shell
when the pane is idle — never the line that was typed, so a guessed `keys:` would be
wrong either way. A saved session starts its panes empty; add the `keys:` worth keeping
by hand.

Without a path the file goes to the first configured source directory — the first,
because a name found in an earlier source shadows a later one, so anywhere else would
write a file `--list` marks as shadowed.

**An existing file is not overwritten without `--overwrite`.** The file being replaced is
the one with your hand-added `keys:` in it, and a capture cannot put those back.

## Config

`~/.tmux_s/config.yaml`, optional:

```yaml
sources:
  - ~/.tmux_s/sessions        # the default when sources is absent or empty
  - ~/dotfiles/tmux_sessions
```

Each directory is scanned non-recursively for `*.yaml`/`*.yml`. **A source that does not
exist is skipped silently**, makes one dotfiles config portable across
machines that define different sessions. On a name collision the earlier source wins, so
a machine-local directory listed first overrides the shared one; `tmux_s --list` marks
the shadowed entry.

Run `tmux_s config` to materialize the default file and edit it with `$EDITOR` (falling
back to `$VISUAL`). `tmux_s config --dir` opens `~/.tmux_s` in the system file manager.

## Install

Unix:
```bash
curl -fsSL https://raw.githubusercontent.com/brohd11/tmux_s/main/install.sh | sh
```

Windows:
```powershell
irm https://raw.githubusercontent.com/brohd11/tmux_s/main/install.ps1 | iex
```

To update:
```
tmux_s update
```

More install details (location, flags, etc): [shared install reference](https://github.com/brohd11/goutil/blob/main/docs/install.md).

**macOS note:** a binary downloaded **in a browser** gets quarantined by Gatekeeper. Clear it
with `xattr -dr com.apple.quarantine path/to/binary`. This doesn't apply to the installer
above; the attribute is set by browsers, not by `curl`.
