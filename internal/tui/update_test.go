package tui

// Tests of the new-version notice in the TUI (Deps.UpdateCheck): line under the header.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

const tuiNotice = `A new version of Watchoo is available: v0.6.1 → v0.6.2. Quit, then run "watchoo update".`

var newVersion = UpdateInfo{Current: "v0.6.1", Latest: "v0.6.2"}

// countingCheck returns a check that answers info, err, and the number of its calls.
func countingCheck(info UpdateInfo, err error) (func(context.Context) (UpdateInfo, error), *int) {
	n := new(int)
	return func(ctx context.Context) (UpdateInfo, error) {
		*n++
		if _, ok := ctx.Deadline(); !ok {
			return UpdateInfo{}, errors.New("check without deadline")
		}
		return info, err
	}, n
}

// noticeLine returns the screen line under the header.
func noticeLine(screen string) string {
	return strings.Split(screen, "\n")[headerHeight]
}

func TestUpdateNoticeShownUnderHeader(t *testing.T) {
	deps := testDeps(newFakeData(), &fakeSyncer{})
	check, calls := countingCheck(newVersion, nil)
	deps.UpdateCheck = check
	l := newLoop(t, deps)
	l.send(tea.WindowSizeMsg{Width: 120, Height: 24})
	l.until(func(m Model) bool { return m.update.Latest != "" })
	if got := noticeLine(l.screen()); got != tuiNotice {
		t.Errorf("line under the header = %q; want the notice", got)
	}
	// The layout does not move: the notice takes the empty line under the header.
	if n := len(strings.Split(l.screen(), "\n")); n != 24 {
		t.Errorf("%d lines; want 24", n)
	}
	// The notice stays shown after a view change and a footer message.
	l.press("2", "S")
	l.settle()
	mustContain(t, l.screen(), tuiNotice)
	if *calls != 1 {
		t.Errorf("%d checks; want 1 (at startup)", *calls)
	}
}

func TestUpdateNoticeVariants(t *testing.T) {
	docker := UpdateInfo{Current: "v0.6.1", Latest: "v0.6.2", Docker: true}
	tests := []struct {
		name    string
		info    UpdateInfo
		width   int
		unicode bool
		want    string
	}{
		{name: "docker", info: docker, width: 120, unicode: true,
			want: "A new version of Watchoo is available: v0.6.1 → v0.6.2. Change the tag of the Docker image to update it."},
		{name: "ascii", info: newVersion, width: 120,
			want: `A new version of Watchoo is available: v0.6.1 -> v0.6.2. Quit, then run "watchoo update".`},
		{name: "80 columns", info: newVersion, width: 80, unicode: true,
			want: `Watchoo v0.6.2 available: quit, then run "watchoo update".`},
		{name: "80 columns docker", info: docker, width: 80, unicode: true,
			want: "Watchoo v0.6.2 available: change the tag of the Docker image."},
		{name: "very narrow", info: newVersion, width: 30, unicode: true, want: "Watchoo v0.6.2 available: qui…"},
		{name: "sequence cleaned", info: UpdateInfo{Current: "v0.6.1", Latest: "v0.6.2\x1b[31m"}, width: 120, unicode: true,
			want: tuiNotice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(newFakeData(), &fakeSyncer{})
			deps.Unicode = tt.unicode
			m := NewModel(t.Context(), deps)
			next, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: 24})
			next, _ = next.(Model).Update(updateMsg{info: tt.info})
			if got := noticeLine(next.(Model).View().Content); got != tt.want {
				t.Errorf("notice = %q; want %q", got, tt.want)
			}
		})
	}
	// In color: the notice is styled, without exceeding the width.
	deps := testDeps(newFakeData(), &fakeSyncer{})
	deps.Color = true
	m := NewModel(t.Context(), deps)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	next, _ = next.(Model).Update(updateMsg{info: newVersion})
	line := noticeLine(next.(Model).View().Content)
	if !strings.Contains(line, "\x1b[") || width(line) > 80 {
		t.Errorf("colored notice = %q (width %d)", line, width(line))
	}
}

func TestUpdateNoticeAbsent(t *testing.T) {
	tests := []struct {
		name   string
		check  func(context.Context) (UpdateInfo, error)
		cancel bool
	}{
		{name: "without check"},
		{name: "up to date", check: func(context.Context) (UpdateInfo, error) { return UpdateInfo{}, nil }},
		{name: "error", check: func(context.Context) (UpdateInfo, error) {
			return newVersion, errors.New("offline")
		}},
		{name: "cancelled", cancel: true, check: func(ctx context.Context) (UpdateInfo, error) {
			select {
			case <-ctx.Done():
				return UpdateInfo{}, ctx.Err()
			case <-time.After(5 * time.Second):
				return newVersion, nil
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps(newFakeData(), &fakeSyncer{})
			deps.UpdateCheck = tt.check
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			m := NewModel(ctx, deps)
			if tt.cancel {
				cancel() // quitting before the answer
			}
			if tt.check != nil {
				next, _ := m.Update(m.updateCmd()())
				m = next.(Model)
			}
			if m.update.Latest != "" {
				t.Errorf("new version = %+v; want none", m.update)
			}
			if got := noticeLine(m.View().Content); got != "" {
				t.Errorf("line under the header = %q; want empty", got)
			}
		})
	}
}
