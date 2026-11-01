package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Booby-trapped commit messages and Git names (OSC, CSI, BEL, C1 CSI sequences, bidi mark): the text
// output of log, contributors and repo list sends no sequence to the terminal; the JSON keeps the
// original text, escaped.
func TestTextOutputNeutralizesEscapes(t *testing.T) {
	h := newWorkspace(t)
	evil := provider.Person{Name: "Eve\x1b]0;titre\x07 ‮Nom", Email: "eve@example.test"}
	h.Provider.AddRepository(provider.Repository{ID: 301, Owner: "eve", Name: "projet",
		URL: "https://github.example.test/eve/projet"})
	addCommits(h.Provider, 301, "main", "",
		fakeCommit{"e1", evil, time.Hour, "fix\x1b]0;PWNED\x07\x1b[2J ok\u009b31m\x1b]8;;https://x.test\x1b\\lien"})
	h.run("repo", "add", "eve/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)

	for _, args := range [][]string{{"log"}, {"contributors"}, {"repo", "list"}} {
		r := h.run(args...).mustSucceed(t)
		if strings.ContainsAny(r.stdout, "\x1b\x07\u009b‮") {
			t.Errorf("%v: control sequence in the text output:\n%q", args, r.stdout)
		}
		assertContains(t, strings.Join(args, " "), r.stdout, "Eve")
	}
	r := h.run("log").mustSucceed(t)
	assertContains(t, "log", r.stdout, "fix ok", "lien")

	r = h.run("log", "--json").mustSucceed(t)
	var rep logReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Commits) != 1 || !strings.Contains(rep.Commits[0].Message, "\x1b]0;PWNED") {
		t.Errorf("log --json: original message lost: %+v", rep.Commits)
	}
	if strings.Contains(r.stdout, "\x1b") {
		t.Errorf("log --json: ESC not escaped:\n%q", r.stdout)
	}
}

func TestRenderTableCleansCells(t *testing.T) {
	st := textStyle{}
	got := renderTable(st, []string{"a", "b"}, [][]string{{"x\x1b]0;pwn\x07y", st.truncate("msg\x1b[31mred\x1b[2J", 60)}}, nil)
	if got != "A   B\nxy  msgred\n" {
		t.Errorf("renderTable = %q", got)
	}
}
