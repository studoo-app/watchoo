package web

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/studoo-app/watchoo/internal/selfupdate"
)

// fixedUpdates est une UpdateSource figée.
type fixedUpdates struct {
	st selfupdate.Status
	ok bool
}

func (f fixedUpdates) Available() (selfupdate.Status, bool) { return f.st, f.ok }

var newVersion = selfupdate.Status{Current: "v0.5.0", Latest: "v0.6.0", UpdateAvailable: true,
	ReleaseURL: "https://github.com/studoo-app/watchoo/releases/tag/v0.6.0"}

// Bandeau de nouvelle version : sur toutes les pages quand une version plus récente est connue.
func TestUpdateBannerShown(t *testing.T) {
	e := newTestEnv(t, withUpdates(fixedUpdates{st: newVersion, ok: true}, false))
	ids := e.seed()
	for _, path := range []string{"/", "/contributors", "/settings", "/repos/" + strconv.FormatInt(ids.Alice, 10), "/introuvable"} {
		w := e.get(path)
		assertContains(t, w, `id="update-banner"`, "v0.5.0 → v0.6.0", "watchoo update",
			`href="https://github.com/studoo-app/watchoo/releases/tag/v0.6.0"`, "Notes de version")
		assertNotContains(t, w, "tag de l'image Docker")
	}
}

func TestUpdateBannerDocker(t *testing.T) {
	e := newTestEnv(t, withUpdates(fixedUpdates{st: newVersion, ok: true}, true))
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="update-banner"`, "v0.6.0", "tag de l'image Docker")
	assertNotContains(t, w, "<code>watchoo update</code>")
}

func TestUpdateBannerAbsent(t *testing.T) {
	for name, opt := range map[string]envOption{
		"sans source":  func(*testEnv) {},
		"aucune":       withUpdates(fixedUpdates{}, false),
		"à jour":       withUpdates(fixedUpdates{st: selfupdate.Status{Current: "v0.6.0", Latest: "v0.6.0"}, ok: true}, false),
		"monitor neuf": withUpdates(selfupdate.NewMonitor(selfupdate.MonitorOptions{}), false),
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t, opt)
			w := e.get("/")
			assertStatus(t, w, http.StatusOK)
			assertNotContains(t, w, `id="update-banner"`, "Nouvelle version")
		})
	}
}

// Pas de bandeau sur une page publique (avant authentification).
func TestUpdateBannerNotOnPublicPages(t *testing.T) {
	e := newTestEnv(t, withAuth("prof", "mot-de-passe-solide"), withUpdates(fixedUpdates{st: newVersion, ok: true}, false))
	w := e.get("/")
	assertStatus(t, w, http.StatusUnauthorized)
	assertNotContains(t, w, `id="update-banner"`, "v0.6.0")
}
