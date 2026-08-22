package tmux

import (
	"bytes"
	"strings"
	"testing"

	"github.com/brohd11/tmux_s/internal/spec"
)

// lines renders a plan the way --print does, so the assertions read as the commands a
// user would see.
func lines(cmds []Command) []string {
	var buf bytes.Buffer
	_ = Print(&buf, cmds)
	return strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
}

func pane(keys string) spec.Pane { return spec.Pane{Keys: []string{keys}, Enter: true} }

// The full shape for a multi-window, multi-pane session: creation order, -c inheritance,
// the pane-id targeting, the layout after the splits, and the trailing select-window.
func TestPlan(t *testing.T) {
	s := spec.Session{
		Name: "roblox",
		Dir:  "/home/b",
		Windows: []spec.Window{
			{Name: "remote", Focus: true, Panes: []spec.Pane{{Keys: []string{"ssh brohd"}, Enter: false}}},
			{Name: "notes", Panes: []spec.Pane{pane("gote roblox")}},
			{Name: "ports", Layout: "even-horizontal", Panes: []spec.Pane{
				pane("ssh -L 1:localhost:1 brohd"),
				{Dir: "/var/log", Keys: []string{"clear", "tail -f a"}, Enter: true, Focus: true},
			}},
		},
	}
	want := []string{
		"tmux new-session -d -s roblox -n remote -c /home/b -P -F '#{pane_id}'",
		"tmux send-keys -t %pane0 'ssh brohd'",
		"tmux new-window -t roblox: -n notes -c /home/b -P -F '#{pane_id}'",
		"tmux send-keys -t %pane1 'gote roblox' C-m",
		"tmux new-window -t roblox: -n ports -c /home/b -P -F '#{pane_id}'",
		"tmux split-window -t %pane2 -c /var/log -P -F '#{pane_id}'",
		"tmux select-layout -t roblox:ports even-horizontal",
		"tmux send-keys -t %pane2 'ssh -L 1:localhost:1 brohd' C-m",
		"tmux send-keys -t %pane3 clear C-m",
		"tmux send-keys -t %pane3 'tail -f a' C-m",
		"tmux select-pane -t %pane3",
		"tmux select-window -t roblox:remote",
	}
	got := lines(Plan(s))
	if len(got) != len(want) {
		t.Fatalf("got %d commands, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("command %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
}

// With no window marked, the first one is selected — the same default the bash scripts
// got by ending on an explicit select-window.
func TestPlanDefaultsToTheFirstWindow(t *testing.T) {
	s := spec.Session{Name: "s", Windows: []spec.Window{
		{Name: "a", Panes: []spec.Pane{pane("x")}},
		{Name: "b", Panes: []spec.Pane{pane("y")}},
	}}
	got := lines(Plan(s))
	if last := got[len(got)-1]; last != "tmux select-window -t s:a" {
		t.Errorf("last command = %q, want select-window on the first window", last)
	}
}

// A split leaves the new pane active, so a multi-pane window with nothing marked must
// still be walked back to pane 0. A single-pane window needs no select-pane at all.
func TestPlanPaneFocus(t *testing.T) {
	multi := lines(Plan(spec.Session{Name: "s", Windows: []spec.Window{
		{Name: "w", Panes: []spec.Pane{pane("a"), pane("b")}},
	}}))
	if !contains(multi, "tmux select-pane -t %pane0") {
		t.Errorf("multi-pane window did not select pane 0:\n%s", strings.Join(multi, "\n"))
	}
	single := lines(Plan(spec.Session{Name: "s", Windows: []spec.Window{
		{Name: "w", Panes: []spec.Pane{pane("a")}},
	}}))
	for _, l := range single {
		if strings.HasPrefix(l, "tmux select-pane") {
			t.Errorf("single-pane window emitted %q", l)
		}
	}
}

// Panes without a directory of their own fall back to the session's, and a window with
// no directory anywhere gets no -c so tmux uses the cwd.
func TestPlanDirFallback(t *testing.T) {
	got := lines(Plan(spec.Session{Name: "s", Windows: []spec.Window{
		{Name: "w", Panes: []spec.Pane{pane("a")}},
	}}))
	if strings.Contains(got[0], "-c") {
		t.Errorf("no directory anywhere, but got %q", got[0])
	}
}

// Real send-keys payloads are full of quotes, && and $; they cross no shell, so nothing
// in them may be mangled on the way to tmux.
func TestSendKeysPayloadIsOneArgument(t *testing.T) {
	line := `ssh -t brohd "tmux new-session -A -s roblox" && echo $HOME`
	cmds := Plan(spec.Session{Name: "s", Windows: []spec.Window{
		{Name: "w", Panes: []spec.Pane{{Keys: []string{line}, Enter: true}}},
	}})
	for _, c := range cmds {
		if c[0] != "send-keys" {
			continue
		}
		if c[3] != line {
			t.Fatalf("payload = %q, want it verbatim", c[3])
		}
		return
	}
	t.Fatal("no send-keys in the plan")
}

// Every pane-creating command must capture its id; nothing else may.
func TestCapture(t *testing.T) {
	for _, c := range []Command{{"new-session"}, {"new-window"}, {"split-window"}} {
		if !c.Capture() {
			t.Errorf("%v does not capture", c)
		}
	}
	for _, c := range []Command{{"send-keys"}, {"select-pane"}, {"select-layout"}, {}} {
		if c.Capture() {
			t.Errorf("%v captures", c)
		}
	}
}

// resolve is what turns the plan's placeholders into the ids tmux handed back.
func TestResolve(t *testing.T) {
	got := resolve(Command{"send-keys", "-t", paneRef(1), "ls"}, []string{"%4", "%7"})
	if got[2] != "%7" {
		t.Errorf("target = %q, want %%7", got[2])
	}
	if got[3] != "ls" {
		t.Errorf("payload = %q, want ls", got[3])
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
