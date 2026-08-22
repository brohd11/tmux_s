# tmux_s

Build a tmux session from a small YAML file and attach to it.

```
tmux_s                  list the sessions that are defined
tmux_s roblox           build (or attach to) the session named roblox
tmux_s roblox --print   show the tmux commands without running any
tmux_s roblox --detach  build the session but stay where you are
tmux_s update           update to the latest release
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
dir: ~/main/roblox          # optional; default cwd. Inherited by windows and panes.
windows:
  remote:
    keys: ssh -t brohd "tmux new-session -A -s roblox"
    enter: false            # type it into the pane, don't press Enter
    focus: true             # the window selected on attach
  notes:                    # no numbered keys -> a single pane
    keys: gote roblox --vault
  ports:
    layout: even-horizontal # tiled | even-horizontal | even-vertical | main-vertical | ...
    0:                      # numbered keys -> panes, created in index order
      keys: ssh -L 34783:localhost:34783 brohd
    1:
      dir: ~/logs
      keys:                 # a list -> one send-keys per line
        - clear
        - tail -f out.log
      focus: true           # the pane selected within this window
  term: cd ~/main/go        # a bare string is shorthand for `keys:`
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

Two shapes are accepted per file. One carrying `session`, `dir` or `windows` at the top
level is a single session. Any other top-level mapping is read as session name → spec,
so several sessions can share one file:

```yaml
roblox:
  windows:
    remote: ssh brohd
go-dev:
  dir: ~/main/go
  windows:
    agent: claude
```

Within a window, integer keys are panes and string keys are the window's own fields. A
window mapping with no integer keys describes that window's single pane, so `keys` and
`enter` sit directly on it. Mixing the two is an error.

`~` and `$VAR` are expanded in every `dir` — tmux's `-c` takes a literal path and
nothing here passes through a shell that would expand them.

## Config

`~/.tmux_s/config.yaml`, optional:

```yaml
sources:
  - ~/.tmux_s/sessions        # the default when sources is absent or empty
  - ~/dotfiles/tmux_sessions
```

Each directory is scanned non-recursively for `*.yaml`/`*.yml`. **A source that does not
exist is skipped silently** — that is what makes one dotfiles config portable across
machines that define different sessions. On a name collision the earlier source wins, so
a machine-local directory listed first overrides the shared one; `tmux_s --list` marks
the shadowed entry.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/brohd11/tmux_s/main/install.sh | sh
```

Or from a checkout: `make && ./install_unix.sh`.
