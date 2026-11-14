package web

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/studoo-app/watchoo/internal/selfupdate"
)

// fixedUpdates is an UpdateSource that never changes.
type fixedUpdates struct {
	st selfupdate.Status
	ok bool
}

func (f fixedUpdates) Available() (selfupdate.Status, bool) { return f.st, f.ok }

var newVersion = selfupdate.Status{Current: "v0.5.0", Latest: "v0.6.0", UpdateAvailable: true,
	ReleaseURL: "https://github.com/studoo-app/watchoo/releases/tag/v0.6.0"}

// New version banner: on every page when a more recent version is known.
func TestUpdateBannerShown(t *testing.T) {
	e := newTestEnv(t, withUpdates(fixedUpdates{st: newVersion, ok: true}, false))
	ids := e.seed()
	for _, path := range []string{"/", "/contributors", "/settings/repos", "/repos/" + strconv.FormatInt(ids.Alice, 10), "/not-found"} {
		w := e.get(path)
		assertContains(t, w, `id="update-banner"`, "v0.5.0 → v0.6.0", "watchoo update",
			`href="https://github.com/studoo-app/watchoo/releases/tag/v0.6.0"`, "Release notes",
			"New version of Watchoo available:")
		assertNotContains(t, w, "tag of the Docker image")
	}
}

func TestUpdateBannerDocker(t *testing.T) {
	e := newTestEnv(t, withUpdates(fixedUpdates{st: newVersion, ok: true}, true))
	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="update-banner"`, "v0.6.0", "tag of the Docker image")
	assertNotContains(t, w, "<code>watchoo update</code>")
}

func TestUpdateBannerAbsent(t *testing.T) {
	for name, opt := range map[string]envOption{
		"without source": func(*testEnv) {},
		"none":           withUpdates(fixedUpdates{}, false),
		"up to date":     withUpdates(fixedUpdates{st: selfupdate.Status{Current: "v0.6.0", Latest: "v0.6.0"}, ok: true}, false),
		"new monitor":    withUpdates(selfupdate.NewMonitor(selfupdate.MonitorOptions{}), false),
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t, opt)
			w := e.get("/")
			assertStatus(t, w, http.StatusOK)
			assertNotContains(t, w, `id="update-banner"`, "New version")
		})
	}
}

// No banner on a public page (before authentication).
func TestUpdateBannerNotOnPublicPages(t *testing.T) {
	e := newTestEnv(t, withAuth("prof", "a-strong-password"), withUpdates(fixedUpdates{st: newVersion, ok: true}, false))
	w := e.get("/")
	assertStatus(t, w, http.StatusUnauthorized)
	assertNotContains(t, w, `id="update-banner"`, "v0.6.0")
}
