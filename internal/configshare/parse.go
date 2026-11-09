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

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
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
	// FlemmeAfterDays et DeadAfterDays sont les seuils d'activité du dépôt ; nil = non fourni (seuils
	// existants gardés), 0 = revenir au seuil de l'espace.
	FlemmeAfterDays *int
	DeadAfterDays   *int
	// ConventionsSince est la date de début des conventions du dépôt, AAAA-MM-JJ ; nil = non fournie
	// (date existante gardée), "" = revenir à la date de l'espace.
	ConventionsSince *string
}

// Parse lit et valide un document (MaxFileSize octets au plus). Erreurs, toutes dans la langue du
// processus et enveloppant une erreur sentinelle :
//   - ErrTooLarge : plus de MaxFileSize octets ;
//   - ErrUnsupportedFormat : clé format absente ou différente de Format ;
//   - ErrNewerVersion : version supérieure à Version ;
//   - ErrInvalid : JSON invalide ou suivi d'un autre texte, version absente ou inférieure à 1, clé
//     inconnue (à tout niveau, y compris github ou web dans settings), plus de MaxRepositories dépôts,
//     provider autre que github, repo invalide (repoimport.ParseRef), doublon (sans tenir compte de la
//     casse), label invalide (repoimport.ValidateLabel), seuils d'activité d'un dépôt invalides
//     (repoimport.ValidateThresholds), date de début des conventions d'un dépôt invalide
//     (repoimport.ParseConventionsSince), motif de branche vide, avec virgule ou invalide,
//     durée illisible, motif de branches.exclude ou fuseau avec caractère de contrôle, langue autre
//     que « en » ou « fr », texte informatif invalide. Les problèmes sont tous cités (20 au plus).
//
// Les règles qui dépendent de l'espace (dead_after_days > flemme_after_days, bornes, fuseau inconnu)
// sont vérifiées par Plan, après fusion avec la configuration de l'espace.
func Parse(r io.Reader) (*File, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, errWrapped(i18n.T("configshare.parse.read"), err)
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
		return nil, invalidf("%s", jsonMessage(err))
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, invalidf("%s", i18n.T("configshare.parse.trailing"))
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
		return invalidf("%s", jsonMessage(err))
	}
	var format string
	if len(head.Format) == 0 || json.Unmarshal(head.Format, &format) != nil || format != Format {
		return fmt.Errorf("%w %s", ErrUnsupportedFormat, i18n.T("configshare.parse.format_key", Format))
	}
	if len(head.Version) == 0 || string(head.Version) == "null" {
		return invalidf("%s", i18n.T("configshare.parse.version_missing"))
	}
	var version int
	if err := json.Unmarshal(head.Version, &version); err != nil {
		return invalidf("%s", i18n.T("configshare.parse.version_int"))
	}
	switch {
	case version > Version:
		return fmt.Errorf("%w %s", ErrNewerVersion, i18n.T("configshare.parse.version_newer", version, Version))
	case version < 1:
		return invalidf("%s", i18n.T("configshare.parse.version_invalid", version, Version))
	}
	return nil
}

// invalidf renvoie une erreur qui enveloppe ErrInvalid suivie du détail (« fichier de configuration
// invalide : détail »).
func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w%s%s", ErrInvalid, i18n.T("configshare.sep"), fmt.Sprintf(format, args...))
}

// jsonMessage décrit une erreur de encoding/json en mots simples, dans la langue du processus.
func jsonMessage(err error) string {
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return i18n.T("configshare.json.syntax", syntax.Offset)
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return i18n.T("configshare.json.incomplete")
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			return i18n.T("configshare.json.want_object", typeErr.Value)
		}
		return i18n.T("configshare.json.type", field, typeErr.Value, goTypeName(typeErr.Type.String()))
	}
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "json: unknown field "); ok {
		return i18n.T("configshare.json.unknown_key", name, Format, Version)
	}
	if strings.Contains(msg, "parsing time") {
		return i18n.T("configshare.json.exported_at")
	}
	return i18n.T("configshare.json.unreadable")
}

// goTypeName décrit un type Go attendu en mots simples.
func goTypeName(t string) string {
	t = strings.TrimLeft(t, "*")
	switch {
	case t == "string":
		return i18n.T("configshare.type.text")
	case t == "int":
		return i18n.T("configshare.type.int")
	case t == "bool":
		return i18n.T("configshare.type.bool")
	case strings.HasPrefix(t, "[]"):
		return i18n.T("configshare.type.list")
	default:
		return i18n.T("configshare.type.object")
	}
}

// validate vérifie le contenu d'un document décodé et renvoie le File correspondant.
func validate(doc *Document) (*File, error) {
	var errs []string
	fail := func(key string, args ...any) { errs = append(errs, i18n.T(key, args...)) }

	f := &File{Source: Source{
		WorkspaceName:  doc.Workspace.Name,
		WatchooVersion: doc.WatchooVersion,
		ExportedAt:     doc.ExportedAt.UTC(),
	}}
	if msg := checkText(doc.Workspace.Name); msg != "" {
		fail("configshare.v.field", "workspace.name", msg)
	}
	if msg := checkText(doc.WatchooVersion); msg != "" {
		fail("configshare.v.field", "watchoo_version", msg)
	}

	if doc.Settings != nil {
		f.Settings = parseSettings(doc.Settings, fail)
	}

	if n := len(doc.Repositories); n > MaxRepositories {
		return nil, invalidf("%s", i18n.T("configshare.v.too_many_repos", n, MaxRepositories))
	}
	seen := make(map[string]int, len(doc.Repositories))
	for i, r := range doc.Repositories {
		idx := i + 1
		where := i18n.T("configshare.v.repo_n", idx)
		if r.Provider != "" && r.Provider != model.ProviderGitHub {
			fail("configshare.v.provider", where, quote(r.Provider), model.ProviderGitHub)
			continue
		}
		ref, err := repoimport.ParseRef(r.Repo)
		if err != nil {
			fail("configshare.v.field", where, err)
			continue
		}
		where = i18n.T("configshare.v.repo_n_ref", idx, ref)
		key := strings.ToLower(ref.String())
		if first, dup := seen[key]; dup {
			fail("configshare.v.duplicate", where, first)
			continue
		}
		seen[key] = idx
		if err := repoimport.ValidateLabel(r.Label); err != nil {
			fail("configshare.v.field", where, err)
			continue
		}
		branches, msg := parseBranches(r.Branches)
		if msg != "" {
			fail("configshare.v.branches", where, msg)
			continue
		}
		if err := repoimport.ValidateThresholds(r.FlemmeAfterDays, r.DeadAfterDays); err != nil {
			fail("configshare.v.field", where, err)
			continue
		}
		var since *string
		if r.ConventionsSince != nil {
			date, err := repoimport.ParseConventionsSince(*r.ConventionsSince)
			if err != nil {
				fail("configshare.v.field", where, err)
				continue
			}
			since = &date
		}
		f.Repositories = append(f.Repositories, Entry{Index: idx, Ref: ref, Label: r.Label, Branches: branches,
			FlemmeAfterDays: clonePtr(r.FlemmeAfterDays), DeadAfterDays: clonePtr(r.DeadAfterDays),
			ConventionsSince: since})
	}

	if len(errs) > 0 {
		if len(errs) > maxReportedErrors {
			extra := len(errs) - maxReportedErrors
			more := "configshare.v.more_problems_many"
			if extra == 1 {
				more = "configshare.v.more_problems_one"
			}
			errs = append(errs[:maxReportedErrors], i18n.T(more, extra))
		}
		return nil, fmt.Errorf("%w%s\n  %s", ErrInvalid, i18n.T("configshare.v.nothing_imported_list"), strings.Join(errs, "\n  "))
	}
	return f, nil
}

// parseSettings convertit les réglages du document ; les problèmes sont signalés par fail.
//
// fail reçoit une clé du catalogue i18n et ses arguments.
func parseSettings(s *Settings, fail func(string, ...any)) workspace.SharedSettings {
	var out workspace.SharedSettings
	if s.Sync != nil {
		out.SyncAuto = s.Sync.Auto
		out.SyncParallelism = s.Sync.Parallelism
		if s.Sync.Interval != nil {
			d, err := time.ParseDuration(strings.TrimSpace(*s.Sync.Interval))
			if err != nil {
				fail("configshare.v.interval", quote(*s.Sync.Interval))
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
				fail("configshare.v.exclude_empty", i+1)
				ok = false
			case strings.ContainsFunc(p, unicode.IsControl):
				fail("configshare.v.exclude_control", i+1)
				ok = false
			default:
				if _, err := path.Match(p, ""); err != nil {
					fail("configshare.v.exclude_invalid", quote(p))
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
			fail("configshare.v.field", "settings.timezone", msg)
		} else {
			tz := *s.Timezone
			out.Timezone = &tz
		}
	}
	if s.Lang != nil {
		// Même forme canonique que config.yaml (« FR » → « fr ») ; seules « en » et « fr » passent.
		if lang := workspace.NormalizeLang(*s.Lang); i18n.Valid(i18n.Lang(lang)) {
			out.Lang = &lang
		} else {
			fail("configshare.v.lang", quote(*s.Lang))
		}
	}
	if s.Conventions != nil {
		parseConventions(s.Conventions, &out, fail)
	}
	return out
}

// parseConventions convertit la charte des conventions (settings.conventions) vers out. Chaque valeur
// est vérifiée seule ; la cohérence avec l'espace (longueurs, au moins un type) l'est par Plan
// (Config.Validate des réglages fusionnés).
func parseConventions(c *ConventionsSettings, out *workspace.SharedSettings, fail func(string, ...any)) {
	if c.Since != nil {
		since := *c.Since
		if _, err := time.Parse(conventions.SinceInputLayout, since); since != "" && err != nil {
			fail("configshare.v.conv_since", quote(since))
		} else {
			out.ConventionsSince = &since
		}
	}
	if c.Branch != nil {
		parseConventionsBranch(c.Branch, out, fail)
	}
	if c.Tags != nil && c.Tags.Milestones != nil {
		out.ConventionsMilestones = parseMilestones(*c.Tags.Milestones, fail)
	}
	parseConventionsFiles(c, out, fail)
	if c.Commit == nil {
		return
	}
	cm := c.Commit
	out.ConventionsTypes = parseConventionsList(cm.Types, "commit.types", conventions.ValidType, fail)
	out.ConventionsScopes = parseConventionsList(cm.Scopes, "commit.scopes", conventions.ValidScope, fail)
	out.ConventionsForbidden = parseConventionsList(cm.Forbidden, "commit.forbidden", nil, fail)
	out.ConventionsMaxSubjectSoft = cm.MaxSubjectSoft
	out.ConventionsMaxSubjectHard = cm.MaxSubjectHard
	if cm.Language != nil {
		if lang := *cm.Language; slices.Contains(workspace.ConventionsLanguages(), lang) {
			out.ConventionsLanguage = &lang
		} else {
			fail("configshare.v.conv_language", quote(lang))
		}
	}
}

// parseConventionsBranch convertit settings.conventions.branch : pattern vide ou RE2 valide, sans
// caractère de contrôle (checkText) ; exclude, chaque motif valide pour path.Match.
func parseConventionsBranch(b *ConventionsBranchSettings, out *workspace.SharedSettings,
	fail func(string, ...any)) {
	if b.Pattern != nil {
		p := *b.Pattern
		msg := checkText(p)
		switch {
		case msg != "":
			fail("configshare.v.field", "settings.conventions.branch.pattern", msg)
		case conventions.ValidBranchPattern(p) != nil:
			fail("configshare.v.conv_branch_pattern", quote(p))
		default:
			out.ConventionsBranchPattern = &p
		}
	}
	valid := func(g string) bool { return conventions.ValidBranchExclude(g) == nil }
	out.ConventionsBranchExclude = parseConventionsList(b.Exclude, "branch.exclude", valid, fail)
}

// parseConventionsFiles convertit settings.conventions.changelog, readme et docs (phase 8.3) : sections
// connues (conventions.ValidReadmeSection), dossiers relatifs sans « . » ni « .. » (conventions.ValidDocsDir,
// conventions.ValidMediaDir), portée des archives admise. Le minimum de mots et les doublons de sections
// sont vérifiés par Plan (Config.Validate des réglages fusionnés).
func parseConventionsFiles(c *ConventionsSettings, out *workspace.SharedSettings, fail func(string, ...any)) {
	if c.Changelog != nil {
		out.ConventionsCategoriesFR = c.Changelog.CategoriesFR
	}
	if c.Readme != nil {
		out.ConventionsReadmeSections = parseConventionsList(c.Readme.Sections, "readme.sections",
			conventions.ValidReadmeSection, fail)
		out.ConventionsReadmeMinWords = c.Readme.MinWords
	}
	if c.Docs == nil {
		return
	}
	text := func(v *string, name string, valid func(string) bool) *string {
		if v == nil {
			return nil
		}
		s := *v
		if msg := checkText(s); msg != "" {
			fail("configshare.v.field", "settings.conventions."+name, msg)
			return nil
		}
		if !valid(s) {
			fail("configshare.v.conv_docs", name, quote(s))
			return nil
		}
		return &s
	}
	out.ConventionsDocsDir = text(c.Docs.Dir, "docs.dir", conventions.ValidDocsDir)
	out.ConventionsDocsMediaDir = text(c.Docs.MediaDir, "docs.media_dir", conventions.ValidMediaDir)
	out.ConventionsArchivesScope = text(c.Docs.ArchivesScope, "docs.archives_scope", conventions.ValidArchivesScope)
}

// parseMilestones valide settings.conventions.tags.milestones : chaque entrée lue par
// conventions.ParseMilestone, chaque version une seule fois. Une liste refusée renvoie nil.
func parseMilestones(in []string, fail func(string, ...any)) *[]string {
	list := make([]string, 0, len(in))
	seen := map[string]bool{}
	ok := true
	for i, v := range in {
		if !utf8.ValidString(v) || strings.ContainsFunc(v, unicode.IsControl) {
			fail("configshare.v.conv_item_control", "tags.milestones", i+1)
			ok = false
			continue
		}
		m, err := conventions.ParseMilestone(v)
		switch {
		case errors.Is(err, conventions.ErrMilestoneDue):
			fail("configshare.v.conv_milestone_due", i+1, quote(v))
			ok = false
		case err != nil:
			fail("configshare.v.conv_milestone", i+1, quote(v))
			ok = false
		case seen[m.Name]:
			fail("configshare.v.conv_milestone_duplicate", i+1, m.Name)
			ok = false
		default:
			seen[m.Name] = true
			list = append(list, v)
		}
	}
	if !ok {
		return nil
	}
	return &list
}

// parseConventionsList valide une liste de settings.conventions.<name> (« commit.types »…) : chaque entrée non vide,
// sans caractère de contrôle et, si valid n'est pas nil, de la forme attendue. nil reste nil (non
// fourni) ; une liste refusée renvoie nil.
func parseConventionsList(in *[]string, name string, valid func(string) bool,
	fail func(string, ...any)) *[]string {
	if in == nil {
		return nil
	}
	list := make([]string, 0, len(*in))
	ok := true
	for i, v := range *in {
		switch {
		case strings.TrimSpace(v) == "":
			fail("configshare.v.conv_item_empty", name, i+1)
			ok = false
		case !utf8.ValidString(v) || strings.ContainsFunc(v, unicode.IsControl):
			fail("configshare.v.conv_item_control", name, i+1)
			ok = false
		case valid != nil && !valid(v):
			fail("configshare.v.conv_item_invalid", name, i+1, quote(v))
			ok = false
		default:
			list = append(list, v)
		}
	}
	if !ok {
		return nil
	}
	return &list
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
			return nil, i18n.T("configshare.v.pattern_empty", i+1)
		case strings.Contains(p, ","):
			return nil, i18n.T("configshare.v.pattern_comma", quote(p))
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
		return i18n.T("configshare.v.text_utf8")
	case strings.ContainsFunc(v, unicode.IsControl):
		return i18n.T("configshare.v.text_control")
	case utf8.RuneCountInString(v) > MaxTextRunes:
		return i18n.T("configshare.v.text_long", MaxTextRunes)
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
