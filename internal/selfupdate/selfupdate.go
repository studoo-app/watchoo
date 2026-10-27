// Package selfupdate met Watchoo lui-même à jour (0.6.0) : il lit la dernière version publiée, la
// compare à la version installée, télécharge et vérifie l'archive de la release, puis remplace le
// binaire en cours. Il porte aussi la vérification périodique de « watchoo serve » (Monitor). Les
// couches cli et web ne font qu'afficher ce qu'il renvoie.
//
// Exception documentée (documentation/03-architecture.md, « Mise à jour de Watchoo ») à la règle « rien
// de spécifique à GitHub hors de internal/provider » : ce package lit les pages PUBLIQUES des releases
// de Watchoo lui-même, sans rapport avec les dépôts suivis. Il n'utilise ni token (ni celui de gh, ni
// aucun autre), ni l'API GitHub (aucun quota), ni go-github : seulement net/http, en GET.
//
//   - Dernière version : comme install.sh, la redirection de <racine>/latest vers <racine>/tag/vX.Y.Z
//     (la page de la release n'est pas téléchargée).
//   - Archive : <racine>/download/vX.Y.Z/watchoo_X.Y.Z_<os>_<arch>.tar.gz (.zip sous Windows) et
//     watchoo_X.Y.Z_checksums.txt (noms de .goreleaser.yaml).
//   - HTTPS seulement, redirections comprises ; HTTP n'est accepté que si la racine elle-même est en
//     HTTP sur la boucle locale (faux serveur des tests), et seulement vers la boucle locale.
package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
)

// DefaultReleasesURL est la racine des releases de Watchoo.
const (
	DefaultReleasesURL = "https://github.com/studoo-app/watchoo/releases"
)

// Plafonds de taille des téléchargements.
const (
	maxArchiveSize   = 100 << 20 // archive de release
	maxChecksumsSize = 1 << 20   // fichier de sommes
	maxBinarySize    = 200 << 20 // binaire extrait de l'archive
	maxPageSize      = 1 << 20   // corps lu (puis jeté) d'une réponse sans redirection
	maxRedirects     = 10
)

// Status est le résultat d'une vérification : version installée, dernière version publiée et liens.
type Status struct {
	Current string // version installée (« v0.5.0 », « dev »)
	Latest  string // dernière version publiée (« v0.6.0 »)
	// UpdateAvailable : Current est une version de release (IsRelease) strictement plus ancienne que
	// Latest. Faux pour une version de développement (comparaison impossible) ou plus récente.
	UpdateAvailable bool
	ReleaseURL      string // page de la release de Latest
	ChangelogURL    string // CHANGELOG.md de Latest
}

// Options règle New. Les champs vides prennent leur valeur de production.
type Options struct {
	// BaseURL est la racine des releases (DefaultReleasesURL) ; les tests y mettent un faux serveur
	// (selfupdatetest).
	BaseURL string
	// UserAgent est l'en-tête User-Agent des requêtes (« watchoo/v0.5.0 ») ; vide = « watchoo ».
	UserAgent string
	// GOOS et GOARCH choisissent l'archive à télécharger ; vides = runtime.GOOS et runtime.GOARCH.
	GOOS, GOARCH string
	// Transport fait les requêtes ; nil = clone de http.DefaultTransport.
	Transport http.RoundTripper
}

// Client lit les releases de Watchoo et installe une version. Sûr pour un usage concurrent.
type Client struct {
	base      *url.URL
	baseErr   error
	allowHTTP bool // racine en HTTP sur la boucle locale (tests)
	userAgent string
	goos      string
	goarch    string
	http      *http.Client // suit les redirections (téléchargements)
	latest    *http.Client // s'arrête à la redirection vers …/tag/vX.Y.Z
}

// New renvoie un Client. Une racine invalide (ou en HTTP hors boucle locale) n'est signalée qu'à la
// première requête, par une erreur.
func New(opts Options) *Client {
	c := &Client{userAgent: opts.UserAgent, goos: opts.GOOS, goarch: opts.GOARCH}
	if c.userAgent == "" {
		c.userAgent = "watchoo"
	}
	if c.goos == "" {
		c.goos = runtime.GOOS
	}
	if c.goarch == "" {
		c.goarch = runtime.GOARCH
	}
	raw := strings.TrimSuffix(opts.BaseURL, "/")
	if raw == "" {
		raw = DefaultReleasesURL
	}
	c.base, c.baseErr = url.Parse(raw)
	if c.baseErr == nil {
		c.allowHTTP = c.base.Scheme == "http" && isLoopback(c.base.Hostname())
		if !c.allowedURL(c.base) {
			c.baseErr = fmt.Errorf("adresse des releases refusée (HTTPS obligatoire) : %s", raw)
		}
	}
	transport := opts.Transport
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	c.http = &http.Client{Transport: transport, CheckRedirect: c.checkRedirect}
	c.latest = &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if err := c.checkRedirect(req, via); err != nil {
			return err
		}
		if isTagURL(req.URL) {
			return http.ErrUseLastResponse // le tag est dans Location : la page n'est pas téléchargée
		}
		return nil
	}}
	return c
}

// isLoopback indique une adresse IP de la boucle locale (127.0.0.0/8, ::1).
func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// allowedURL accepte HTTPS, et HTTP vers la boucle locale si la racine elle-même y est (tests).
func (c *Client) allowedURL(u *url.URL) bool {
	switch u.Scheme {
	case "https":
		return u.Host != ""
	case "http":
		return c.allowHTTP && isLoopback(u.Hostname())
	}
	return false
}

// checkRedirect refuse une redirection hors HTTPS et les chaînes trop longues.
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("trop de redirections")
	}
	if !c.allowedURL(req.URL) {
		return fmt.Errorf("redirection refusée (HTTPS obligatoire) : %s", req.URL.Redacted())
	}
	return nil
}

// isTagURL indique une adresse de page de release : …/tag/<tag>.
func isTagURL(u *url.URL) bool {
	return path.Base(path.Dir(u.Path)) == "tag" && path.Base(u.Path) != "tag"
}

// endpoint renvoie la racine suivie des segments elems.
func (c *Client) endpoint(elems ...string) (*url.URL, error) {
	if c.baseErr != nil {
		return nil, c.baseErr
	}
	return c.base.JoinPath(elems...), nil
}

// get envoie un GET sur u avec client.
func (c *Client) get(ctx context.Context, client *http.Client, u *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("requête vers %s : %w", u.Redacted(), err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requête vers %s : %w", u.Redacted(), err)
	}
	return resp, nil
}

// Latest renvoie le tag de la dernière release publiée (« v0.6.0 ») et l'adresse de sa page, lus dans
// la redirection de <racine>/latest. Un tag qui n'est pas une version finale (pré-version) est refusé.
func (c *Client) Latest(ctx context.Context) (tag, releaseURL string, err error) {
	u, err := c.endpoint("latest")
	if err != nil {
		return "", "", err
	}
	resp, err := c.get(ctx, c.latest, u)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxPageSize))
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusOK {
			return "", "", errors.New("aucune release publiée trouvée (pas de redirection vers une version)")
		}
		return "", "", fmt.Errorf("dernière version illisible : réponse HTTP %d", resp.StatusCode)
	}
	loc, err := resp.Location()
	if err != nil || !isTagURL(loc) {
		return "", "", errors.New("dernière version illisible : redirection sans version")
	}
	if !c.allowedURL(loc) {
		return "", "", fmt.Errorf("redirection refusée (HTTPS obligatoire) : %s", loc.Redacted())
	}
	tag = path.Base(loc.Path)
	v, ok := ParseVersion(tag)
	if !ok || !strings.HasPrefix(tag, "v") || !v.IsFinal() {
		return "", "", fmt.Errorf("dernière version illisible : tag %q inattendu (attendu : vX.Y.Z)", tag)
	}
	page, err := c.endpoint("tag", tag)
	if err != nil {
		return "", "", err
	}
	return tag, page.String(), nil
}

// ChangelogURL renvoie l'adresse du CHANGELOG.md de la version tag, dans le dépôt dont la racine des
// releases est …/releases.
func (c *Client) ChangelogURL(tag string) string {
	if c.baseErr != nil {
		return ""
	}
	repo := *c.base
	repo.Path = strings.TrimSuffix(repo.Path, "/releases")
	return repo.JoinPath("blob", tag, "CHANGELOG.md").String()
}

// Check lit la dernière version publiée et la compare à current (voir Status.UpdateAvailable).
func (c *Client) Check(ctx context.Context, current string) (Status, error) {
	tag, page, err := c.Latest(ctx)
	if err != nil {
		return Status{}, err
	}
	return newStatus(current, tag, page, c.ChangelogURL(tag)), nil
}

// newStatus construit le Status de current face à la dernière version latest.
func newStatus(current, latest, releaseURL, changelogURL string) Status {
	st := Status{Current: current, Latest: latest, ReleaseURL: releaseURL, ChangelogURL: changelogURL}
	cur, okCur := ParseVersion(current)
	last, okLast := ParseVersion(latest)
	st.UpdateAvailable = okCur && cur.IsFinal() && okLast && last.IsFinal() && last.Compare(cur) > 0
	return st
}
