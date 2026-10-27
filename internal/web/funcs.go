package web

// Fonctions de template partagées par toutes les pages. Elles ne renvoient que des chaînes, des
// nombres ou des booléens : html/template échappe toujours leur résultat (aucune ne renvoie de
// template.HTML).
//
// Les classes DaisyUI écrites ici en toutes lettres (badgeClass) sont vues par Tailwind, qui lit
// aussi les fichiers Go de ce répertoire (@source de tailwind/input.css).

import (
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// avatarPrefix est le seul hôte d'avatars accepté (CSP img-src).
const avatarPrefix = "https://avatars.githubusercontent.com/"

// maxFirstLine borne la première ligne d'un message de commit affichée dans une liste.
const maxFirstLine = 120

// funcs renvoie les fonctions de template. Elles lisent l'horloge (s.now) et le fuseau (s.loc) au
// moment du rendu.
//
//	relTime t        « il y a 2 h », « à l'instant », « jamais » (activity.Relative, fuseau d'affichage)
//	localTime t      « 27/10/2026 à 14:05 » en heure locale (pour un attribut title) ; « » si zéro
//	localDate t      « 27/10/2026 » ; localClock t : « 14:05 » ; « » si zéro
//	isoTime t        « 2026-10-27T13:05:00Z » (attribut datetime de <time>) ; « » si zéro
//	badgeClass st    classe DaisyUI de l'état : badge-success (Actif), badge-warning (Flemme),
//	                 badge-error (Dead), badge-neutral (états gris)
//	stateText res    libellé de l'état : « Actif », « Flemme · 9 j », « Dead · 42 j », « Jamais synchronisé »…
//	clean s          textclean.Clean (caractères de contrôle et séquences d'échappement retirés)
//	firstLine s      première ligne nettoyée, coupée à 120 caractères (« … »)
//	truncate n s     s nettoyé, coupé à n caractères (« … »)
//	shortSHA s       7 premiers caractères
//	initial s        première lettre en majuscule (avatar de repli) ; « ? » si aucune
//	avatarURL s      s s'il commence par https://avatars.githubusercontent.com/, sinon « »
//	plural n un plusieurs   « 1 commit », « 3 commits », « 0 commit »
//	add a b, sub a b        entiers
//	repoPath id      « /repos/12 »
//	contribPath id   « /contributors/12 » (page d'un contributeur, 0.5.0)
//	profileURL s     s si c'est une adresse https:// (page de profil donnée par le provider), sinon « »
//	interval d       « 30 min », « 1 h », « 1 h 30 », « manuelle seule » (0)
//	percent a b      a*100/b arrondi, entre 0 et 100 (0 si b <= 0)
//	dict k1 v1 k2 v2…       map[string]any, pour passer plusieurs valeurs à un template
//	withQuery path k1 v1…   path suivi de la query string encodée (url.Values) ; paires de valeur vide
//	                 ou 0 omises : « /repos/3?author=a%26b&page=2 ». À utiliser pour TOUTE URL avec des
//	                 données, y compris dans hx-get / hx-push-url, que html/template n'encode pas (ce
//	                 sont pour lui des attributs de texte)
func (s *server) funcs() template.FuncMap {
	return template.FuncMap{
		"relTime":     func(t time.Time) string { return activity.Relative(t, s.now(), s.loc, true) },
		"localTime":   func(t time.Time) string { return s.formatLocal(t, "02/01/2006 à 15:04") },
		"localDate":   func(t time.Time) string { return s.formatLocal(t, "02/01/2006") },
		"localClock":  func(t time.Time) string { return s.formatLocal(t, "15:04") },
		"isoTime":     isoTime,
		"badgeClass":  badgeClass,
		"stateText":   stateText,
		"clean":       textclean.Clean,
		"firstLine":   firstLine,
		"truncate":    truncate,
		"shortSHA":    shortSHA,
		"initial":     initial,
		"avatarURL":   avatarURL,
		"plural":      pluralize,
		"add":         func(a, b int) int { return a + b },
		"sub":         func(a, b int) int { return a - b },
		"repoPath":    repoPath,
		"contribPath": contributorPath,
		"profileURL":  profileURL,
		"interval":    formatInterval,
		"percent":     percent,
		"dict":        dict,
		"withQuery":   withQuery,
	}
}

// formatLocal écrit t dans le fuseau d'affichage avec layout ; « » si t est zéro.
func (s *server) formatLocal(t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	return t.In(s.loc).Format(layout)
}

func isoTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// badgeClass renvoie la classe DaisyUI de la pastille d'un état.
func badgeClass(st activity.State) string {
	switch st {
	case activity.StateActive:
		return "badge-success"
	case activity.StateFlemme:
		return "badge-warning"
	case activity.StateDead:
		return "badge-error"
	default:
		return "badge-neutral"
	}
}

// stateText renvoie le libellé d'un état, avec l'âge du dernier commit pour Flemme et Dead.
func stateText(res activity.Result) string {
	switch res.State {
	case activity.StateFlemme, activity.StateDead:
		return fmt.Sprintf("%s · %d j", res.State.Label(), res.Days)
	default:
		return res.State.Label()
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return truncate(maxFirstLine, line)
}

func truncate(n int, s string) string {
	s = strings.TrimSpace(textclean.Clean(s))
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:n])) + "…"
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func initial(s string) string {
	for _, r := range textclean.Clean(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

func avatarURL(s string) string {
	if strings.HasPrefix(s, avatarPrefix) && !strings.ContainsAny(s, "\"'<> \\") {
		return s
	}
	return ""
}

func pluralize(n int, one, many string) string {
	if n <= 1 && n >= -1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func repoPath(id int64) string {
	return "/repos/" + strconv.FormatInt(id, 10)
}

// contributorPath renvoie la page d'un contributeur (0.5.0).
func contributorPath(id int64) string {
	return "/contributors/" + strconv.FormatInt(id, 10)
}

// profileURL renvoie s, page de profil d'un compte telle que donnée par le provider et enregistrée en
// base, si c'est une adresse https:// sans caractère suspect ; sinon « » (pas de lien). Aucune adresse
// de forge n'est construite ici.
func profileURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.ContainsAny(s, "\"'<> \\") {
		return ""
	}
	return s
}

// formatInterval écrit un intervalle de synchro en français.
func formatInterval(d time.Duration) string {
	if d <= 0 {
		return "manuelle seule"
	}
	h, m := int(d/time.Hour), int((d%time.Hour)/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", m)
	case m == 0:
		return fmt.Sprintf("%d h", h)
	default:
		return fmt.Sprintf("%d h %02d", h, m)
	}
}

func percent(a, b int) int {
	if b <= 0 {
		return 0
	}
	return min(max((a*100+b/2)/b, 0), 100)
}

func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict : nombre impair d'arguments")
	}
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict : clé %v qui n'est pas une chaîne", pairs[i])
		}
		m[key] = pairs[i+1]
	}
	return m, nil
}

// withQuery renvoie path suivi de la query string des paires (clé, valeur) dont la valeur n'est ni
// vide ni nulle, encodée par url.Values (clés triées).
func withQuery(path string, pairs ...any) (string, error) {
	if len(pairs)%2 != 0 {
		return "", errors.New("withQuery : nombre impair d'arguments")
	}
	q := url.Values{}
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return "", fmt.Errorf("withQuery : clé %v qui n'est pas une chaîne", pairs[i])
		}
		var v string
		switch x := pairs[i+1].(type) {
		case string:
			v = x
		case int:
			if x != 0 {
				v = strconv.Itoa(x)
			}
		case int64:
			if x != 0 {
				v = strconv.FormatInt(x, 10)
			}
		default:
			return "", fmt.Errorf("withQuery : valeur de type %T non gérée pour %q", x, key)
		}
		if v != "" {
			q.Set(key, v)
		}
	}
	if len(q) == 0 {
		return path, nil
	}
	return path + "?" + q.Encode(), nil
}
