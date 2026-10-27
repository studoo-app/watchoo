package providertest

import (
	"errors"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

func TestPullRequestsReleasesTags(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 101, Owner: "alice", Name: "projet"})
	f.SetPullRequests(101,
		provider.PullRequest{Number: 1, State: provider.PullRequestOpen, UpdatedAt: t0.Add(time.Hour)},
		provider.PullRequest{Number: 2, State: provider.PullRequestClosed, Merged: true, UpdatedAt: t0.Add(3 * time.Hour)},
		provider.PullRequest{Number: 3, State: provider.PullRequestOpen, UpdatedAt: t0.Add(2*time.Hour + 500*time.Millisecond)},
	)
	got, err := f.PullRequests(ctx, "Alice", "Projet", provider.PullRequestsOptions{})
	if err != nil || len(got) != 3 || got[0].Number != 2 || got[2].Number != 1 {
		t.Fatalf("PullRequests : %+v, %v", got, err)
	}
	if !got[1].UpdatedAt.Equal(t0.Add(2 * time.Hour)) {
		t.Errorf("UpdatedAt non tronqué à la seconde : %v", got[1].UpdatedAt)
	}
	got, _ = f.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{UpdatedSince: t0.Add(2*time.Hour + 900*time.Millisecond)})
	if len(got) != 2 || got[1].Number != 3 {
		t.Errorf("UpdatedSince : %+v, veut les PR 2 et 3", got)
	}
	got, _ = f.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{Limit: 1})
	if len(got) != 1 {
		t.Errorf("Limit 1 : %d PR", len(got))
	}
	if log := f.CallLog(); log[len(log)-1].PullOpts.Limit != 1 {
		t.Errorf("journal sans les options : %+v", log[len(log)-1])
	}

	f.SetReleases(101, provider.Release{ID: 1, TagName: "v1", CreatedAt: t0},
		provider.Release{ID: 2, TagName: "v2", CreatedAt: t0.Add(time.Hour)})
	releases, err := f.Releases(ctx, "alice", "projet")
	if err != nil || len(releases) != 2 || releases[0].ID != 2 {
		t.Errorf("Releases : %+v, %v", releases, err)
	}
	f.SetTags(101, provider.Tag{Name: "v2", SHA: SHA("b")}, provider.Tag{Name: "v1", SHA: SHA("a")})
	tags, err := f.Tags(ctx, "alice", "projet")
	if err != nil || len(tags) != 2 || tags[0].Name != "v2" {
		t.Errorf("Tags : %+v, %v", tags, err)
	}

	// Dépôt sans rien : listes vides non nil ; dépôt inconnu : ErrNotFound.
	f.AddRepository(provider.Repository{ID: 102, Owner: "bob", Name: "projet"})
	if p, err := f.PullRequests(ctx, "bob", "projet", provider.PullRequestsOptions{}); err != nil || p == nil || len(p) != 0 {
		t.Errorf("PR de bob : %v, %v", p, err)
	}
	if r, err := f.Releases(ctx, "bob", "projet"); err != nil || r == nil {
		t.Errorf("releases de bob : %v, %v", r, err)
	}
	if tg, err := f.Tags(ctx, "bob", "projet"); err != nil || tg == nil {
		t.Errorf("tags de bob : %v, %v", tg, err)
	}
	if _, err := f.Tags(ctx, "carol", "projet"); !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("dépôt inconnu : %v", err)
	}

	// Erreur injectée (ex. source sans pull requests) et quota simulé.
	f.SetError(MethodPullRequests, provider.ErrUnsupported)
	if _, err := f.PullRequests(ctx, "alice", "projet", provider.PullRequestsOptions{}); !errors.Is(err, provider.ErrUnsupported) {
		t.Errorf("SetError ErrUnsupported : %v", err)
	}
	f.SetRateLimit(provider.RateLimit{Limit: 10, Remaining: 1})
	if _, err := f.Releases(ctx, "alice", "projet"); err != nil {
		t.Errorf("Releases avec 1 requête restante : %v", err)
	}
	if _, err := f.Tags(ctx, "alice", "projet"); !errors.Is(err, provider.ErrRateLimited) {
		t.Errorf("Tags sans quota : %v, veut ErrRateLimited", err)
	}
	if f.Calls(MethodTags) != 4 {
		t.Errorf("%d appels de Tags comptés, veut 4", f.Calls(MethodTags))
	}

	for name, fn := range map[string]func(){
		"PR sans numéro":    func() { f.SetPullRequests(101, provider.PullRequest{State: "open", UpdatedAt: t0}) },
		"PR d'état inconnu": func() { f.SetPullRequests(101, provider.PullRequest{Number: 9, State: "merged", UpdatedAt: t0}) },
		"release sans tag":  func() { f.SetReleases(101, provider.Release{ID: 1}) },
		"tag mal formé":     func() { f.SetTags(101, provider.Tag{Name: "v1", SHA: "abc"}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s : panique attendue", name)
				}
			}()
			fn()
		}()
	}
}
