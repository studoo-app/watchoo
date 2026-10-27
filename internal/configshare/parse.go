package configshare

// Lecture et validation d'un document venu d'une autre personne (donnée non fiable).

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// maxReportedErrors borne le nombre de problèmes cités dans une erreur de validation.
const maxReportedErrors = 20

// File est un document validé par Parse, prêt pour Plan et Apply.
type File struct {
	// Source décrit l'origine du document, telle qu'écrite dans le fichier (textes non fiables,
	// validés : UTF-8, sans caractère de contrôle, MaxTextRunes au plus ; à nettoyer à l'affichage
	// comme tout texte).
	Source Source
	// Settings sont les réglages communs fournis (champs nil = non fournis).
	Settings workspace.SharedSettings
	// Repositories sont les dépôts du document, dans l'ordre du fichier.
	Repositories []Entry
}

// Source décrit l'origine d'un document.
type Source struct {
	WorkspaceName  string    // workspace.name ; "" si absent
	WatchooVersion string    // watchoo_version ; "" si absent
	ExportedAt     time.Time // exported_at en UTC ; zéro si absent
}

// Entry est un dépôt validé du document.
type Entry struct {
	Index int            // position dans repositories (1 = premier)
	Ref   repoimport.Ref // dépôt (owner/name tels qu'écrits)
	// Label est le nom affiché ; "" = non fourni (le nom affiché existant est gardé).
	Label string
	// Branches sont les motifs (sans doublon, ordre gardé) ; nil = non fourni (branches existantes
	// gardées, nouveau dépôt : toutes), vide non nil = toutes les branches.
	Branches []string
}

// Parse lit et valide un document (MaxFileSize octets au plus). Erreurs, toutes en français et
// enveloppant une erreur sentinelle :
//   - ErrTooLarge : plus de MaxFileSize octets ;
//   - ErrUnsupportedFormat : clé format absente ou différente de Format ;
//   - ErrNewerVersion : version supérieure à Version ;
//   - ErrInvalid : JSON invalide ou suivi d'un autre texte, version absente ou inférieure à 1, clé
//     inconnue (à tout niveau, y compris github ou web dans settings), plus de MaxRepositories dépôts,
//     provider autre que github, repo invalide (repoimport.ParseRef), doublon (sans tenir compte de la
//     casse), label invalide (repoimport.ValidateLabel), motif de branche vide, avec virgule ou invalide,
//     durée illisible, motif de branches.exclude ou fuseau avec caractère de contrôle, texte
//     informatif invalide. Les problèmes sont tous cités (20 au plus).
//
// Les règles qui dépendent de l'espace (dead_after_days > flemme_after_days, bornes, fuseau inconnu)
// sont vérifiées par Plan, après fusion avec la configuration de l'espace.
func Parse(r io.Reader) (*File, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("lecture du fichier de configuration : %w", err)
	}
	return ParseBytes(data)
}

// ParseBytes est Parse sur un contenu déjà lu.
func ParseBytes(data []byte) (*File, error) {
	if len(data) > MaxFileSize {
		return nil, ErrTooLarge
	}
	if err := checkHeader(data); err != nil {
		return nil, err
	}

	var doc Document
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w : %s", ErrInvalid, jsonMessage(err))
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w : texte après l'objet JSON (un seul objet attendu)", ErrInvalid)
	}
	return validate(&doc)
}

// checkHeader lit format et version avant le décodage strict, pour que les erreurs les plus utiles
// (pas un fichier Watchoo, version plus récente) passent avant celles des autres clés.
func checkHeader(data []byte) error {
	var head struct {
		Format  json.RawMessage `json:"format"`
		Version json.RawMessage `json:"version"`
	}
	// Premier objet seulement : un texte qui suit est signalé par ParseBytes.
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&head); err != nil {
		return fmt.Errorf("%w : %s", ErrInvalid, jsonMessage(err))
	}
	var format string
	if len(head.Format) == 0 || json.Unmarshal(head.Format, &format) != nil || format != Format {
		return fmt.Errorf("%w (clé format absente ou différente de %q)", ErrUnsupportedFormat, Format)
	}
	if len(head.Version) == 0 || string(head.Version) == "null" {
		return fmt.Errorf("%w : clé version absente", ErrInvalid)
	}
	var version int
	if err := json.Unmarshal(head.Version, &version); err != nil {
		return fmt.Errorf("%w : version : nombre entier attendu", ErrInvalid)
	}
	switch {
	case version > Version:
		return fmt.Errorf("%w (format version %d, cette version de Watchoo lit la version %d) : "+
			"mettez Watchoo à jour", ErrNewerVersion, version, Version)
	case version < 1:
		return fmt.Errorf("%w : version %d invalide (%d attendue)", ErrInvalid, version, Version)
	}
	return nil
}

// jsonMessage traduit une erreur de encoding/json en français.
func jsonMessage(err error) string {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return fmt.Sprintf("JSON invalide (octet %d)", syntax.Offset)
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "JSON incomplet ou vide"
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			return fmt.Sprintf("objet JSON attendu, reçu %s", typeErr.Value)
		}
		return fmt.Sprintf("%s : type %s refusé (attendu : %s)", field, typeErr.Value, goTypeName(typeErr.Type.String()))
	}
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "json: unknown field "); ok {
		return fmt.Sprintf("clé inconnue %s (seules les clés du format %s v%d sont acceptées ; "+
			"github.* et web.* ne se partagent jamais)", name, Format, Version)
	}
	if strings.Contains(msg, "parsing time") {
		return "exported_at : date RFC 3339 attendue (ex. « 2026-10-27T10:00:00Z »)"
	}
	return "JSON illisible"
}

// goTypeName décrit un type Go attendu en mots simples.
func goTypeName(t string) string {
	t = strings.TrimLeft(t, "*")
	switch {
	case t == "string":
		return "texte"
	case t == "int":
		return "nombre entier"
	case t == "bool":
		return "true ou false"
	case strings.HasPrefix(t, "[]"):
		return "liste"
	default:
		return "objet"
	}
}

// validate vérifie le contenu d'un document décodé et renvoie le File correspondant.
func validate(doc *Document) (*File, error) {
	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	f := &File{Source: Source{
		WorkspaceName:  doc.Workspace.Name,
		WatchooVersion: doc.WatchooVersion,
		ExportedAt:     doc.ExportedAt.UTC(),
	}}
	if msg := checkText(doc.Workspace.Name); msg != "" {
		fail("workspace.name : %s", msg)
	}
	if msg := checkText(doc.WatchooVersion); msg != "" {
		fail("watchoo_version : %s", msg)
	}

	if doc.Settings != nil {
		f.Settings = parseSettings(doc.Settings, fail)
	}

	if n := len(doc.Repositories); n > MaxRepositories {
		return nil, fmt.Errorf("%w : %d dépôts, %d au plus", ErrInvalid, n, MaxRepositories)
	}
	seen := make(map[string]int, len(doc.Repositories))
	for i, r := range doc.Repositories {
		idx := i + 1
		where := fmt.Sprintf("dépôt n° %d", idx)
		if r.Provider != "" && r.Provider != model.ProviderGitHub {
			fail("%s : forge %s non prise en charge (seule %q l'est)", where, quote(r.Provider), model.ProviderGitHub)
			continue
		}
		ref, err := repoimport.ParseRef(r.Repo)
		if err != nil {
			fail("%s : %v", where, err)
			continue
		}
		where = fmt.Sprintf("dépôt n° %d (%s)", idx, ref)
		key := strings.ToLower(ref.String())
		if first, dup := seen[key]; dup {
			fail("%s : présent deux fois (déjà en n° %d)", where, first)
			continue
		}
		seen[key] = idx
		if err := repoimport.ValidateLabel(r.Label); err != nil {
			fail("%s : %v", where, err)
			continue
		}
		branches, msg := parseBranches(r.Branches)
		if msg != "" {
			fail("%s : branches : %s", where, msg)
			continue
		}
		f.Repositories = append(f.Repositories, Entry{Index: idx, Ref: ref, Label: r.Label, Branches: branches})
	}

	if len(errs) > 0 {
		if len(errs) > maxReportedErrors {
			extra := len(errs) - maxReportedErrors
			errs = append(errs[:maxReportedErrors], fmt.Sprintf("… et %d autre%s problème%s", extra, plural(extra), plural(extra)))
		}
		return nil, fmt.Errorf("%w ; rien n'a été importé :\n  %s", ErrInvalid, strings.Join(errs, "\n  "))
	}
	return f, nil
}

// parseSettings convertit les réglages du document ; les problèmes sont signalés par fail.
func parseSettings(s *Settings, fail func(string, ...any)) workspace.SharedSettings {
	var out workspace.SharedSettings
	if s.Sync != nil {
		out.SyncAuto = s.Sync.Auto
		out.SyncParallelism = s.Sync.Parallelism
		if s.Sync.Interval != nil {
			d, err := time.ParseDuration(strings.TrimSpace(*s.Sync.Interval))
			if err != nil {
				fail("settings.sync.interval : durée %s illisible (exemples : \"30m\", \"1h30m\", \"0\")", quote(*s.Sync.Interval))
			} else {
				out.SyncInterval = &d
			}
		}
	}
	if s.Activity != nil {
		out.FlemmeAfterDays = s.Activity.FlemmeAfterDays
		out.DeadAfterDays = s.Activity.DeadAfterDays
	}
	if s.Branches != nil && s.Branches.Exclude != nil {
		list := make([]string, 0, len(*s.Branches.Exclude))
		ok := true
		for i, p := range *s.Branches.Exclude {
			switch {
			case strings.TrimSpace(p) == "":
				fail("settings.branches.exclude : le motif n° %d est vide", i+1)
				ok = false
			case strings.ContainsFunc(p, unicode.IsControl):
				fail("settings.branches.exclude : le motif n° %d contient un caractère de contrôle", i+1)
				ok = false
			default:
				if _, err := path.Match(p, ""); err != nil {
					fail("settings.branches.exclude : motif %s invalide (syntaxe de path.Match, ex. « dependabot/* »)", quote(p))
					ok = false
				}
				list = append(list, p)
			}
		}
		if ok {
			out.BranchesExclude = &list
		}
	}
	if s.Timezone != nil {
		if msg := checkText(*s.Timezone); msg != "" {
			fail("settings.timezone : %s", msg)
		} else {
			tz := *s.Timezone
			out.Timezone = &tz
		}
	}
	return out
}

// parseBranches valide les motifs d'un dépôt : chacun non vide, sans virgule (séparateur en base),
// valide pour repoimport.ParseBranchPatterns (provider.ValidateBranch et path.Match) ; doublons retirés.
// nil reste nil (non fourni), une liste vide reste vide (toutes les branches).
func parseBranches(in []string) ([]string, string) {
	if in == nil {
		return nil, ""
	}
	out := make([]string, 0, len(in))
	for i, p := range in {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
			return nil, fmt.Sprintf("le motif n° %d est vide", i+1)
		case strings.Contains(p, ","):
			return nil, fmt.Sprintf("motif %s : virgule interdite (un motif par élément de la liste)", quote(p))
		}
		parsed, err := repoimport.ParseBranchPatterns(p)
		if err != nil {
			return nil, err.Error()
		}
		if !slices.Contains(out, parsed[0]) {
			out = append(out, parsed[0])
		}
	}
	return out, ""
}

// checkText vérifie un texte informatif ou le fuseau : UTF-8 valide, sans caractère de contrôle,
// MaxTextRunes caractères au plus. Renvoie le problème, ou "".
func checkText(v string) string {
	switch {
	case !utf8.ValidString(v):
		return "texte UTF-8 invalide"
	case strings.ContainsFunc(v, unicode.IsControl):
		return "caractère de contrôle interdit"
	case utf8.RuneCountInString(v) > MaxTextRunes:
		return fmt.Sprintf("texte trop long (%d caractères au plus)", MaxTextRunes)
	}
	return ""
}

// quote cite une valeur entre guillemets, tronquée à 80 caractères.
func quote(s string) string {
	if utf8.RuneCountInString(s) > 80 {
		s = string([]rune(s)[:80]) + "…"
	}
	return fmt.Sprintf("%q", s)
}

// plural renvoie « s » si n > 1.
func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}
