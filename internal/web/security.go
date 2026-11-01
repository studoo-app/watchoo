package web

// Exposition réseau de l'interface (documentation/fr/06-interface-web.md, « Sécurité ») : refus de
// démarrer hors boucle locale sans authentification, et liste des valeurs acceptées de l'en-tête Host
// (protection contre le DNS rebinding, web.allowed_hosts pour les noms publics d'un reverse proxy).

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// MinPasswordLength est la longueur minimale (en octets) du mot de passe de l'authentification basique.
const MinPasswordLength = 12

// IsLoopbackHost indique si host (partie hôte de web.addr, sans port ni crochets) ne désigne que la
// boucle locale : « localhost » (sans tenir compte de la casse), une adresse de 127.0.0.0/8 ou ::1.
// Un hôte vide (« :8080 », toutes les interfaces), 0.0.0.0, :: ou un autre nom ne l'est pas.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return addr.Unmap().IsLoopback()
}

// CheckExposure applique les règles de démarrage de « watchoo serve » pour l'adresse addr (hôte:port) :
//   - web.auth.username sans mot de passe (variable workspace.WebPasswordEnv vide), ou l'inverse :
//     erreur ;
//   - mot de passe de moins de MinPasswordLength octets : erreur ;
//   - hors boucle locale (IsLoopbackHost faux) sans authentification basique : erreur, sauf si
//     trustedProxy (web.trusted_proxy) avec au moins un nom dans hosts (web.allowed_hosts, les noms
//     publics transmis par le proxy dans l'en-tête Host) ; un avertissement est alors renvoyé, à
//     afficher au démarrage.
//
// Les messages sont dans la langue du processus (affichés par la CLI au démarrage) et ne citent
// jamais le mot de passe.
func CheckExposure(addr string, auth Auth, trustedProxy bool, hosts []string) (warning string, err error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", errors.New(i18n.T("web.expose.addr_invalid", addr))
	}
	switch {
	case auth.Username != "" && auth.Password == "":
		return "", errors.New(i18n.T("web.expose.no_password", workspace.WebPasswordEnv))
	case auth.Username == "" && auth.Password != "":
		return "", errors.New(i18n.T("web.expose.no_username", workspace.WebPasswordEnv))
	case auth.Password != "" && len(auth.Password) < MinPasswordLength:
		return "", errors.New(i18n.T("web.expose.short_password", workspace.WebPasswordEnv, MinPasswordLength))
	}
	if IsLoopbackHost(host) || auth.Enabled() {
		return "", nil
	}
	if trustedProxy && len(hosts) == 0 {
		return "", errors.New(i18n.T("web.expose.proxy_no_hosts", addr))
	}
	if trustedProxy {
		return i18n.T("web.expose.proxy_warning", addr), nil
	}
	return "", errors.New(i18n.T("web.expose.refused", addr, workspace.WebPasswordEnv))
}

// hostPolicy est la liste des valeurs acceptées de l'en-tête Host (protection contre le DNS
// rebinding). Une politique nil n'impose rien.
type hostPolicy struct {
	exact []string // valeurs exactes (en minuscules), port compris : localhost:8080, [::1]:8080…
	names []string // noms acceptés quel que soit le port (web.allowed_hosts), en minuscules, sans crochets
}

// allows indique si l'en-tête Host host est accepté.
func (p *hostPolicy) allows(host string) bool {
	if p == nil {
		return true
	}
	host = strings.ToLower(host)
	if slices.Contains(p.exact, host) {
		return true
	}
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	return name != "" && slices.Contains(p.names, name)
}

// allowedHosts construit le contrôle de l'en-tête Host pour l'adresse écoutée listenAddr :
//   - valeurs exactes : localhost, 127.0.0.1, [::1] et l'hôte écouté (s'il n'est pas « toutes les
//     interfaces »), chacun suivi du port (et aussi sans port pour le port 80) ;
//   - noms de extra (web.allowed_hosts), acceptés avec n'importe quel port : les noms publics
//     transmis par un reverse proxy.
//
// Le contrôle s'applique en écoute locale, derrière un reverse proxy déclaré sans authentification
// basique (trustedProxy && !authEnabled : le proxy ne protège pas d'une page rebindée vers
// l'adresse écoutée), et dès que extra n'est pas vide. Il n'est levé (nil) qu'hors boucle locale avec
// l'authentification basique et sans web.allowed_hosts : le navigateur n'a alors pas d'identifiants
// pour le nom d'un autre site.
func allowedHosts(listenAddr string, extra []string, trustedProxy, authEnabled bool) (*hostPolicy, error) {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return nil, fmt.Errorf("%s%s%w", i18n.T("web.start.addr_invalid", listenAddr), i18n.T("web.colon"), err)
	}
	loopback := IsLoopbackHost(host)
	if !loopback && !(trustedProxy && !authEnabled) && len(extra) == 0 {
		return nil, nil
	}
	names := []string{"localhost", "127.0.0.1", "::1"}
	if addr, err := netip.ParseAddr(host); host != "" && (err != nil || !addr.IsUnspecified()) {
		names = append(names, strings.ToLower(host))
	}
	p := &hostPolicy{}
	seen := map[string]bool{}
	for _, name := range names {
		for _, h := range []string{net.JoinHostPort(name, port), bracket(name)} {
			if h == bracket(name) && port != "80" {
				continue
			}
			if !seen[h] {
				seen[h] = true
				p.exact = append(p.exact, h)
			}
		}
	}
	for _, name := range extra {
		name = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(name), "["), "]"))
		if name != "" && !slices.Contains(p.names, name) {
			p.names = append(p.names, name)
		}
	}
	return p, nil
}

// bracket met une adresse IPv6 entre crochets (forme de l'en-tête Host sans port).
func bracket(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}
