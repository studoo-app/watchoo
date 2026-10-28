package tui

// Tests de l'avis de nouvelle version dans le TUI (Deps.UpdateCheck) : ligne sous l'en-tête.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

const tuiNotice = "Nouvelle version de Watchoo disponible : v0.6.1 → v0.6.2. Quittez puis lancez « watchoo update »."

var newVersion = UpdateInfo{Current: "v0.6.1", Latest: "v0.6.2"}

// countingCheck renvoie une vérification qui répond info, err, et le nombre de ses appels.
func countingCheck(info UpdateInfo, err error) (func(context.Context) (UpdateInfo, error), *int) {
	n := new(int)
	return func(ctx context.Context) (UpdateInfo, error) {
		*n++
		if _, ok := ctx.Deadline(); !ok {
			return UpdateInfo{}, errors.New("vérification sans délai")
		}
		return info, err
	}, n
}

// noticeLine renvoie la ligne de l'écran sous l'en-tête.
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
		t.Errorf("ligne sous l'en-tête = %q ; attendu l'avis", got)
	}
	// La mise en page ne bouge pas : l'avis prend la ligne vide sous l'en-tête.
	if n := len(strings.Split(l.screen(), "\n")); n != 24 {
		t.Errorf("%d lignes ; attendu 24", n)
	}
	// L'avis reste affiché après un changement de vue et un message de bas d'écran.
	l.press("2", "S")
	l.settle()
	mustContain(t, l.screen(), tuiNotice)
	if *calls != 1 {
		t.Errorf("%d vérifications ; attendu 1 (à l'ouverture)", *calls)
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
			want: "Nouvelle version de Watchoo disponible : v0.6.1 → v0.6.2. Changez le tag de l'image Docker pour la mettre à jour."},
		{name: "ascii", info: newVersion, width: 120,
			want: "Nouvelle version de Watchoo disponible : v0.6.1 -> v0.6.2. Quittez puis lancez « watchoo update »."},
		{name: "80 colonnes", info: newVersion, width: 80, unicode: true,
			want: "Watchoo v0.6.2 disponible : quittez puis lancez « watchoo update »."},
		{name: "80 colonnes docker", info: docker, width: 80, unicode: true,
			want: "Watchoo v0.6.2 disponible : changez le tag de l'image Docker."},
		{name: "très étroit", info: newVersion, width: 30, unicode: true, want: "Watchoo v0.6.2 disponible : q…"},
		{name: "séquence nettoyée", info: UpdateInfo{Current: "v0.6.1", Latest: "v0.6.2\x1b[31m"}, width: 120, unicode: true,
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
				t.Errorf("avis = %q ; attendu %q", got, tt.want)
			}
		})
	}
	// En couleur : l'avis est stylé, sans dépasser la largeur.
	deps := testDeps(newFakeData(), &fakeSyncer{})
	deps.Color = true
	m := NewModel(t.Context(), deps)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	next, _ = next.(Model).Update(updateMsg{info: newVersion})
	line := noticeLine(next.(Model).View().Content)
	if !strings.Contains(line, "\x1b[") || width(line) > 80 {
		t.Errorf("avis coloré = %q (largeur %d)", line, width(line))
	}
}

func TestUpdateNoticeAbsent(t *testing.T) {
	tests := []struct {
		name   string
		check  func(context.Context) (UpdateInfo, error)
		cancel bool
	}{
		{name: "sans vérification"},
		{name: "à jour", check: func(context.Context) (UpdateInfo, error) { return UpdateInfo{}, nil }},
		{name: "erreur", check: func(context.Context) (UpdateInfo, error) {
			return newVersion, errors.New("hors ligne")
		}},
		{name: "annulée", cancel: true, check: func(ctx context.Context) (UpdateInfo, error) {
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
				cancel() // on quitte avant la réponse
			}
			if tt.check != nil {
				next, _ := m.Update(m.updateCmd()())
				m = next.(Model)
			}
			if m.update.Latest != "" {
				t.Errorf("nouvelle version = %+v ; attendu aucune", m.update)
			}
			if got := noticeLine(m.View().Content); got != "" {
				t.Errorf("ligne sous l'en-tête = %q ; attendu vide", got)
			}
		})
	}
}
