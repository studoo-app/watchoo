package cli

// « watchoo config export / import » : partage de la configuration d'un espace
// (documentation/fr/05-cli.md, « Partage de la configuration »). Affichage seulement : le cœur est
// internal/configshare (Export, Parse, Plan, Apply).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// configFlags regroupe les options de « watchoo config export » et « watchoo config import ».
type configFlags struct {
	output string // export -o / --output : fichier à créer (0600) ; vide = sortie standard
	force  bool   // export --force : écraser le fichier de -o s'il existe
	dryRun bool   // import --dry-run : aperçu avec vérification d'accès, rien n'est écrit
	yes    bool   // import --yes / -y : appliquer sans question (obligatoire hors terminal)
}

// newConfigCmd construit « watchoo config » (export, import). Sans sous-commande : son aide.
func newConfigCmd(deps Deps, opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Partage la configuration de l'espace avec d'autres superviseurs (export, import)",
		Long: `Partage la configuration de l'espace avec les autres personnes qui supervisent le
même groupe : « watchoo config export » écrit les dépôts suivis et les réglages
communs dans un fichier JSON, « watchoo config import » le fusionne dans un autre
espace, après un aperçu. Rien n'est jamais retiré à l'import.`,
		Example: `  watchoo config export -o licence.json
  watchoo config import licence.json --dry-run`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigExportCmd(deps, opts), newConfigImportCmd(deps, opts))
	return cmd
}

// newConfigExportCmd construit « watchoo config export [-o fichier] [--force] ».
func newConfigExportCmd(deps Deps, opts *options) *cobra.Command {
	flags := &configFlags{}
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Écrit la configuration partageable de l'espace dans un fichier JSON",
		Long: `Écrit la configuration partageable de l'espace dans un fichier JSON : les dépôts
suivis (nom affiché, branches) et les réglages communs (sync.*, activity.*,
branches.exclude, timezone). Jamais les réglages propres à cette machine (github.*,
web.*), ni token, ni commit, ni e-mail.

Sans -o, le document est écrit sur la sortie standard. Avec -o, le fichier est créé
en 0600 ; un fichier existant n'est écrasé qu'avec --force.`,
		Example: `  watchoo config export -o licence.json
  watchoo config export > licence.json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigExport(cmd.Context(), deps, opts, flags)
		},
	}
	cmd.Flags().StringVarP(&flags.output, "output", "o", "",
		"écrit le document dans ce `fichier` (créé en 0600) au lieu de la sortie standard")
	cmd.Flags().BoolVar(&flags.force, "force", false, "écrase le fichier de -o s'il existe déjà")
	return cmd
}

// newConfigImportCmd construit « watchoo config import fichier [--dry-run] [--yes] ».
func newConfigImportCmd(deps Deps, opts *options) *cobra.Command {
	flags := &configFlags{}
	cmd := &cobra.Command{
		Use:   "import fichier",
		Short: "Fusionne un fichier de configuration partagé dans l'espace, après un aperçu",
		Long: `Fusionne dans l'espace un fichier écrit par « watchoo config export », après un aperçu.

Tout le fichier est validé avant de modifier quoi que ce soit. Les dépôts absents
sont ajoutés (accès vérifié auprès de GitHub), les dépôts déjà suivis reçoivent le nom
affiché et les branches du fichier, un dépôt arrêté reprend son suivi ; rien n'est
jamais retiré. Si un réglage commun change, .watchoo/config.yaml est réécrit à partir
du modèle commenté (copie de l'ancien dans config.yaml.bak).

Sur un terminal, l'aperçu est suivi d'une question ; sinon --yes est obligatoire,
ainsi qu'avec --json (sans --dry-run). « - » lit le fichier sur l'entrée standard
(--yes obligatoire alors). Le code de retour est non nul s'il reste des dépôts
inaccessibles. L'import ne lance pas de synchro : enchaînez avec « watchoo sync ».`,
		Example: `  watchoo config import licence.json --dry-run
  watchoo config import licence.json
  watchoo config import - --yes < licence.json`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigImport(cmd.Context(), deps, opts, args[0], flags)
		},
	}
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false,
		"affiche l'aperçu et vérifie l'accès aux dépôts, sans rien écrire")
	cmd.Flags().BoolVarP(&flags.yes, "yes", "y", false, "applique sans demander de confirmation")
	return cmd
}

// --- export ---

// runConfigExport écrit le document de partage de l'espace sur la sortie standard ou dans le fichier
// de -o. Le document est le même avec ou sans --json ; jamais de carte d'en-tête.
func runConfigExport(ctx context.Context, deps Deps, opts *options, flags *configFlags) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	doc, err := configshare.Export(ctx, s.ws, s.store, configshare.ExportOptions{
		Now:            s.now(),
		WatchooVersion: appVersion(),
	})
	if err != nil {
		return err
	}
	data, err := doc.Marshal()
	if err != nil {
		return err
	}
	if flags.output == "" {
		return writeText(deps.Stdout, string(data))
	}
	path, err := absPath(deps, flags.output)
	if err != nil {
		return err
	}
	if err := writeExportFile(path, data, flags.force); err != nil {
		var exists *exportExistsError
		if errors.As(err, &exists) {
			return &hintError{
				msg: fmt.Sprintf("le fichier %s existe déjà ; relancez avec --force pour l'écraser", flags.output),
				err: err,
			}
		}
		return err
	}
	if opts.json {
		return nil // le fichier est le résultat
	}
	return writeText(deps.Stdout, fmt.Sprintf("Configuration exportée dans %s : %s, %s.\n",
		textclean.Clean(flags.output),
		countText(len(doc.Repositories), "dépôt", "dépôts"),
		countText(len(sharedKeysOf(doc)), "réglage", "réglages")))
}

// sharedKeysOf compte les réglages exportés (Export les donne tous).
func sharedKeysOf(doc *configshare.Document) []string {
	if doc.Settings == nil {
		return nil
	}
	var keys []string
	add := func(present bool, key string) {
		if present {
			keys = append(keys, key)
		}
	}
	if sy := doc.Settings.Sync; sy != nil {
		add(sy.Auto != nil, "sync.auto")
		add(sy.Interval != nil, "sync.interval")
		add(sy.Parallelism != nil, "sync.parallelism")
	}
	if a := doc.Settings.Activity; a != nil {
		add(a.FlemmeAfterDays != nil, "activity.flemme_after_days")
		add(a.DeadAfterDays != nil, "activity.dead_after_days")
	}
	if b := doc.Settings.Branches; b != nil {
		add(b.Exclude != nil, "branches.exclude")
	}
	add(doc.Settings.Timezone != nil, "timezone")
	return keys
}

// exportExistsError : le fichier de -o existe déjà (sans --force).
type exportExistsError struct{ path string }

func (e *exportExistsError) Error() string { return "le fichier " + e.path + " existe déjà" }
func (e *exportExistsError) Unwrap() error { return fs.ErrExist }

// writeExportFile crée path en 0600 (O_EXCL : jamais d'écriture à travers un fichier ou un lien
// existant) et y écrit data. Avec force, un fichier (ou lien) existant est d'abord retiré ; un
// répertoire ne l'est jamais. En cas d'échec d'écriture, le fichier à moitié écrit est supprimé.
func writeExportFile(path string, data []byte, force bool) error {
	if info, err := os.Lstat(path); err == nil {
		if info.IsDir() {
			return fmt.Errorf("%s est un répertoire : indiquez un nom de fichier", path)
		}
		if !force {
			return &exportExistsError{path: path}
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remplacement de %s : %w", path, err)
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return &exportExistsError{path: path}
		}
		return fmt.Errorf("création de %s : %w", path, err)
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("écriture de %s : %w", path, werr)
	}
	return nil
}

// absPath rend file absolu par rapport au répertoire courant de deps.
func absPath(deps Deps, file string) (string, error) {
	if filepath.IsAbs(file) {
		return file, nil
	}
	cwd, err := deps.Getwd()
	if err != nil {
		return "", fmt.Errorf("répertoire courant inaccessible : %w", err)
	}
	return filepath.Join(cwd, file), nil
}

// --- import ---

// configImportReport est la sortie JSON de « watchoo config import » (aperçu ou bilan).
type configImportReport struct {
	File         string              `json:"file"`
	DryRun       bool                `json:"dry_run"`
	Applied      bool                `json:"applied"`
	Source       configSourceJSON    `json:"source"`
	Settings     configSettingsJSON  `json:"settings"`
	Repositories []configRepoJSON    `json:"repositories"`
	Summary      configSummaryJSON   `json:"summary"`
	Warnings     []configWarningJSON `json:"warnings"`
}

type configSourceJSON struct {
	WorkspaceName  string     `json:"workspace_name"`
	WatchooVersion string     `json:"watchoo_version"`
	ExportedAt     *time.Time `json:"exported_at"`
}

type configSettingsJSON struct {
	Changes    []configSettingChangeJSON `json:"changes"`
	Rewrite    bool                      `json:"rewrite"`
	Written    bool                      `json:"written"`
	BackupFile string                    `json:"backup_file"`
}

type configSettingChangeJSON struct {
	Key    string `json:"key"`
	Before string `json:"before"`
	After  string `json:"after"`
	EnvVar string `json:"env_var"`
}

type configRepoJSON struct {
	Index    int              `json:"index"`
	Repo     string           `json:"repo"`
	Action   string           `json:"action"`
	Reenable bool             `json:"reenable"`
	Before   *configRepoState `json:"before"`
	After    configRepoState  `json:"after"`
	Outcome  string           `json:"outcome"`
	Error    string           `json:"error"`
}

type configRepoState struct {
	Label    string   `json:"label"`
	Branches []string `json:"branches"`
	// Seuils propres au dépôt ; null = seuil de l'espace.
	FlemmeAfterDays *int `json:"flemme_after_days"`
	DeadAfterDays   *int `json:"dead_after_days"`
	Enabled         bool `json:"enabled"`
}

type configSummaryJSON struct {
	Added          int `json:"added"`
	AlreadyTracked int `json:"already_tracked"`
	Updated        int `json:"updated"`
	Reenabled      int `json:"reenabled"`
	Inaccessible   int `json:"inaccessible"`
	Errors         int `json:"errors"`
}

type configWarningJSON struct {
	Code    string `json:"code"`
	Key     string `json:"key"`
	Message string `json:"message"`
}

// newConfigImportReport convertit un aperçu (et, après Apply, un bilan) en sortie JSON.
func newConfigImportReport(file string, dryRun bool, p *configshare.Preview, res *configshare.Result) configImportReport {
	r := configImportReport{
		File:   file,
		DryRun: dryRun,
		Source: configSourceJSON{
			WorkspaceName:  p.Source.WorkspaceName,
			WatchooVersion: p.Source.WatchooVersion,
			ExportedAt:     timeJSON(p.Source.ExportedAt),
		},
		Settings: configSettingsJSON{
			Changes: make([]configSettingChangeJSON, 0, len(p.Settings.Changes)),
			Rewrite: p.Settings.Rewrite,
		},
		Repositories: make([]configRepoJSON, 0, len(p.Repositories)),
		Warnings:     make([]configWarningJSON, 0, len(p.Warnings)),
	}
	for _, c := range p.Settings.Changes {
		r.Settings.Changes = append(r.Settings.Changes, configSettingChangeJSON{
			Key: c.Key, Before: c.Before, After: c.After, EnvVar: c.EnvVar,
		})
	}
	for _, c := range p.Repositories {
		item := configRepoJSON{
			Index:    c.Index,
			Repo:     c.Repo,
			Action:   string(c.Action),
			Reenable: c.Reenable,
			After:    newConfigRepoState(c.After),
			Outcome:  string(c.Outcome),
			Error:    c.Error,
		}
		if c.Before != nil {
			before := newConfigRepoState(*c.Before)
			item.Before = &before
		}
		r.Repositories = append(r.Repositories, item)
	}
	for _, w := range p.Warnings {
		r.Warnings = append(r.Warnings, configWarningJSON{Code: w.Code, Key: w.Key, Message: w.Message})
	}
	sum := p.Summary
	if res != nil {
		r.Applied = true
		sum = res.Summary
		r.Settings.Written = res.Settings.Written
		r.Settings.BackupFile = res.Settings.BackupPath
	}
	r.Summary = configSummaryJSON{
		Added: sum.Added, AlreadyTracked: sum.AlreadyTracked, Updated: sum.Updated,
		Reenabled: sum.Reenabled, Inaccessible: sum.Inaccessible, Errors: sum.Errors,
	}
	return r
}

// newConfigRepoState convertit un état de dépôt (branches jamais null).
func newConfigRepoState(s configshare.RepoState) configRepoState {
	branches := s.Branches
	if branches == nil {
		branches = []string{}
	}
	return configRepoState{Label: s.Label, Branches: branches, Enabled: s.Enabled,
		FlemmeAfterDays: s.FlemmeAfterDays, DeadAfterDays: s.DeadAfterDays}
}

// runConfigImport lit et valide le fichier, calcule l'aperçu, demande confirmation (sauf --yes) et
// applique l'import (sauf --dry-run), puis affiche le bilan.
func runConfigImport(ctx context.Context, deps Deps, opts *options, file string, flags *configFlags) (err error) {
	// Contrôles d'options avant de lire quoi que ce soit.
	if !flags.dryRun && !flags.yes {
		switch {
		case file == "-":
			return &hintError{
				msg: "« - » lit le fichier sur l'entrée standard, qui ne peut plus servir à confirmer : " +
					"relancez avec --yes (ou --dry-run pour un aperçu)",
				err: errNeedYes,
			}
		case opts.json:
			return &hintError{
				msg: "--json sans --dry-run ne pose pas de question : relancez avec --yes pour appliquer " +
					"(ou --dry-run pour l'aperçu)",
				err: errNeedYes,
			}
		}
	}

	f, err := readConfigFile(deps, file)
	if err != nil {
		return err
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)

	planOpts := configshare.PlanOptions{}
	if flags.dryRun && len(f.Repositories) > 0 {
		engine, err := s.engine()
		if err != nil {
			return err
		}
		planOpts.CheckAccess = engine
	}
	preview, err := configshare.Plan(ctx, s.ws, s.store, f, planOpts)
	if err != nil {
		return importParseError(file, err)
	}

	if flags.dryRun {
		report := newConfigImportReport(file, true, preview, nil)
		if opts.json {
			err = writeJSON(deps.Stdout, report)
		} else {
			err = writeText(deps.Stdout, configPreviewText(file, preview, s.loc, true)+
				"\nAperçu seulement (--dry-run) : rien n'a été modifié.\n")
		}
		if err != nil {
			return err
		}
		return configImportFailure(preview.Summary, preview.Repositories)
	}

	// Aperçu, puis confirmation (sauf --yes). En JSON, --yes est obligatoire (vérifié plus haut) et
	// l'aperçu n'est pas écrit à part.
	if !opts.json {
		// Hors terminal, sans --yes : refus avant tout affichage (sortie standard vide en cas d'erreur,
		// comme repo remove) ; l'aperçu reste disponible avec --dry-run.
		if hasChanges(preview) && !flags.yes && !deps.StdinIsTerminal() {
			return errNeedYes
		}
		if err := writeText(deps.Stdout, configPreviewText(file, preview, s.loc, false)); err != nil {
			return err
		}
		if !hasChanges(preview) {
			if err := writeText(deps.Stdout, "Rien à changer : l'espace est déjà à jour.\n"); err != nil {
				return err
			}
		} else if !flags.yes {
			ok, err := confirm(ctx, deps, "Appliquer ces changements ?")
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("import annulé : rien n'a été modifié")
			}
		}
	}

	var adder configshare.Adder
	if len(f.Repositories) > 0 {
		engine, err := s.engine()
		if err != nil {
			return err
		}
		adder = engine
	}
	res, applyErr := configshare.Apply(ctx, s.ws, s.store, adder, f, configshare.ApplyOptions{})
	if res == nil {
		if applyErr == nil {
			applyErr = errors.New("import : aucun résultat")
		}
		return importParseError(file, applyErr)
	}
	report := newConfigImportReport(file, false, res.Preview, res)
	if opts.json {
		err = writeJSON(deps.Stdout, report)
	} else {
		err = writeText(deps.Stdout, "\n"+configResultText(res))
	}
	if err != nil {
		return err
	}
	if applyErr != nil {
		return applyErr
	}
	return configImportFailure(res.Summary, res.Preview.Repositories)
}

// hasChanges indique que l'import changerait quelque chose (réglage, ajout, mise à jour), d'après
// l'aperçu ; sinon la question n'est pas posée.
func hasChanges(p *configshare.Preview) bool {
	s := p.Summary
	return p.Settings.Rewrite || s.Added+s.Updated > 0
}

// readConfigFile lit et valide le fichier de configuration partagé (« - » : entrée standard). Un
// chemin relatif l'est au répertoire courant de deps.
func readConfigFile(deps Deps, file string) (*configshare.File, error) {
	var r io.Reader
	if file == "-" {
		r = deps.Stdin
	} else {
		path, err := absPath(deps, file)
		if err != nil {
			return nil, err
		}
		fh, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("fichier de configuration %s introuvable", file)
			}
			return nil, fmt.Errorf("ouverture du fichier de configuration : %w", err)
		}
		defer fh.Close()
		if info, err := fh.Stat(); err == nil && info.IsDir() {
			return nil, fmt.Errorf("%s est un répertoire, pas un fichier de configuration", file)
		}
		r = fh
	}
	f, err := configshare.Parse(r)
	if err != nil {
		return nil, importParseError(file, err)
	}
	return f, nil
}

// importParseError préfixe une erreur de validation par le nom du fichier et rappelle que rien n'a
// été importé.
func importParseError(file string, err error) error {
	msg := err.Error()
	if !strings.Contains(msg, "rien n'a été importé") {
		return fmt.Errorf("%s : %w ; rien n'a été importé", file, err)
	}
	return fmt.Errorf("%s : %w", file, err)
}

// configImportFailure renvoie l'erreur (code de retour non nul) d'un import qui laisse des dépôts
// inaccessibles ou en erreur ; nil sinon.
func configImportFailure(sum configshare.Summary, changes []configshare.RepoChange) error {
	if sum.Inaccessible+sum.Errors == 0 {
		return nil
	}
	var repos []string
	for _, c := range changes {
		if c.Action == configshare.ActionInaccessible || c.Action == configshare.ActionError {
			repos = append(repos, c.Repo)
		}
	}
	return importFailure(importSummary{Inaccessible: sum.Inaccessible, Errors: sum.Errors}, repos)
}

// --- sortie texte ---

// configPreviewText renvoie l'aperçu texte d'un import (documentation/fr/05-cli.md, « watchoo config
// import fichier ») : origine du fichier, réglages modifiés, dépôts par action, avertissements.
func configPreviewText(file string, p *configshare.Preview, loc *time.Location, dryRun bool) string {
	var b strings.Builder
	title := "IMPORT  " + textclean.Clean(file)
	var origin []string
	if p.Source.WorkspaceName != "" {
		origin = append(origin, "espace « "+textclean.Clean(p.Source.WorkspaceName)+" »")
	}
	var exported []string
	if !p.Source.ExportedAt.IsZero() {
		exported = append(exported, "exporté le "+p.Source.ExportedAt.In(loc).Format("02/01/2006"))
	}
	if p.Source.WatchooVersion != "" {
		exported = append(exported, "avec Watchoo "+textclean.Clean(p.Source.WatchooVersion))
	}
	if len(exported) > 0 {
		origin = append(origin, strings.Join(exported, " "))
	}
	if len(origin) > 0 {
		title += "  (" + strings.Join(origin, ", ") + ")"
	}
	b.WriteString(title + "\n")

	b.WriteString("Réglages\n")
	switch {
	case len(p.Settings.Provided) == 0:
		b.WriteString("  aucun dans le fichier\n")
	case len(p.Settings.Changes) == 0:
		b.WriteString(fmt.Sprintf("  inchangés (%s, identiques à ceux de l'espace)\n",
			countText(len(p.Settings.Provided), "réglage", "réglages")))
	default:
		for _, c := range p.Settings.Changes {
			line := fmt.Sprintf("  %-26s %s → %s", c.Key, textclean.Clean(c.Before), textclean.Clean(c.After))
			if c.EnvVar != "" {
				line += "   (masqué par " + c.EnvVar + ")"
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("Dépôts\n")
	if len(p.Repositories) == 0 {
		b.WriteString("  aucun dans le fichier\n")
	} else {
		var updates, reenables, inaccessible, errored []string
		for _, c := range p.Repositories {
			switch c.Action {
			case configshare.ActionUpdate:
				if c.Reenable {
					reenables = append(reenables, repoChangeText(c))
				} else {
					updates = append(updates, repoChangeText(c))
				}
			case configshare.ActionInaccessible:
				inaccessible = append(inaccessible, textclean.Clean(c.Repo))
			case configshare.ActionError:
				errored = append(errored, textclean.Clean(c.Repo))
			}
		}
		s := p.Summary
		line := func(label string, n int, items []string) {
			row := fmt.Sprintf("  %-16s %3d", label, n)
			if len(items) > 0 {
				row += "   " + items[0]
			}
			b.WriteString(row + "\n")
			for _, item := range items[min(1, len(items)):] {
				b.WriteString(strings.Repeat(" ", 2+16+1+3+3) + item + "\n")
			}
		}
		line("à ajouter", s.Added, nil)
		line("à mettre à jour", s.Updated-s.Reenabled, updates)
		if s.Reenabled > 0 {
			line("à reprendre", s.Reenabled, reenables)
		}
		line("inchangés", s.AlreadyTracked, nil)
		if p.AccessChecked || s.Inaccessible > 0 {
			line("inaccessibles", s.Inaccessible, joined(inaccessible))
		}
		if s.Errors > 0 {
			line("en erreur", s.Errors, joined(errored))
		}
		b.WriteString(failureDetails(p.Repositories))
	}
	for _, w := range p.Warnings {
		b.WriteString("! " + textclean.Clean(w.Message) + "\n")
	}
	if !p.AccessChecked && !dryRun && len(p.Repositories) > 0 {
		b.WriteString("(l'accès aux dépôts sera vérifié auprès de GitHub au moment d'appliquer)\n")
	}
	return b.String()
}

// joined renvoie les éléments de list réunis en un seul (« a, b »), ou rien si list est vide.
func joined(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	return []string{strings.Join(list, ", ")}
}

// repoChangeText décrit la mise à jour d'un dépôt : « bob/projet (branches : toutes → main, dev) ».
func repoChangeText(c configshare.RepoChange) string {
	text := textclean.Clean(c.Repo)
	if c.Before == nil {
		return text
	}
	var parts []string
	if c.Before.Label != c.After.Label {
		parts = append(parts, "nom : "+labelText(c.Before.Label)+" → "+labelText(c.After.Label))
	}
	if !equalStrings(c.Before.Branches, c.After.Branches) {
		parts = append(parts, "branches : "+patternsText(c.Before.Branches)+" → "+patternsText(c.After.Branches))
	}
	if !sameDays(c.Before.FlemmeAfterDays, c.After.FlemmeAfterDays) {
		parts = append(parts, "flemme : "+daysText(c.Before.FlemmeAfterDays)+" → "+daysText(c.After.FlemmeAfterDays))
	}
	if !sameDays(c.Before.DeadAfterDays, c.After.DeadAfterDays) {
		parts = append(parts, "dead : "+daysText(c.Before.DeadAfterDays)+" → "+daysText(c.After.DeadAfterDays))
	}
	if len(parts) > 0 {
		text += " (" + strings.Join(parts, " ; ") + ")"
	}
	return text
}

// daysText écrit un seuil propre (« 10 j »), ou « espace » pour le seuil de l'espace (nil).
func daysText(p *int) string {
	if p == nil {
		return "espace"
	}
	return strconv.Itoa(*p) + " j"
}

// sameDays compare deux seuils propres (nil = seuil de l'espace).
func sameDays(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// labelText écrit un nom affiché entre guillemets, ou « aucun ».
func labelText(label string) string {
	if label == "" {
		return "aucun"
	}
	return "« " + textclean.Clean(label) + " »"
}

// patternsText écrit des motifs de branches, ou « toutes » pour une liste vide.
func patternsText(branches []string) string {
	if len(branches) == 0 {
		return "toutes"
	}
	return textclean.Clean(strings.Join(branches, ", "))
}

// equalStrings compare deux listes (nil et vide sont égales).
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// failureDetails détaille les dépôts inaccessibles ou en erreur (précédé d'une ligne vide), ou "".
func failureDetails(changes []configshare.RepoChange) string {
	var details []string
	for _, c := range changes {
		if c.Action == configshare.ActionInaccessible || c.Action == configshare.ActionError {
			details = append(details, fmt.Sprintf("  dépôt n° %d  %s : %s", c.Index, textclean.Clean(c.Repo),
				textclean.Clean(c.Error)))
		}
	}
	if len(details) == 0 {
		return ""
	}
	return "\n" + strings.Join(details, "\n") + "\n"
}

// configResultText renvoie le bilan texte d'un import appliqué : réglages écrits, dépôts par
// résultat, détail des échecs.
func configResultText(res *configshare.Result) string {
	var b strings.Builder
	b.WriteString("BILAN\n")
	if res.Settings.Written {
		line := "  réglages écrits dans " + textclean.Clean(displayPath(res.Settings.Path))
		if res.Settings.BackupPath != "" {
			line += " (ancien fichier : " + textclean.Clean(displayPath(res.Settings.BackupPath)) + ")"
		}
		b.WriteString(line + "\n")
	} else {
		b.WriteString("  réglages inchangés : config.yaml n'a pas été modifié\n")
	}
	p := res.Preview
	if len(p.Repositories) > 0 {
		s := res.Summary
		var inaccessible, errored []string
		for _, c := range p.Repositories {
			switch c.Action {
			case configshare.ActionInaccessible:
				inaccessible = append(inaccessible, c.Repo)
			case configshare.ActionError:
				errored = append(errored, c.Repo)
			}
		}
		line := func(label string, n int, repos []string) {
			row := fmt.Sprintf("  %-16s %3d", label, n)
			if len(repos) > 0 {
				row += "   " + textclean.Clean(strings.Join(repos, ", "))
			}
			b.WriteString(row + "\n")
		}
		line("ajoutés", s.Added, nil)
		line("déjà suivis", s.AlreadyTracked, nil)
		line("mis à jour", s.Updated, nil)
		if s.Reenabled > 0 {
			line("dont repris", s.Reenabled, nil)
		}
		line("inaccessibles", s.Inaccessible, inaccessible)
		if s.Errors > 0 {
			line("en erreur", s.Errors, errored)
		}
		b.WriteString(failureDetails(p.Repositories))
		if s.Added+s.Updated > 0 {
			b.WriteString("\nLancez « watchoo sync » pour synchroniser les dépôts.\n")
		}
	}
	if res.Settings.Written {
		b.WriteString("Les réglages valent pour les commandes lancées ensuite ; un « watchoo serve » ou " +
			"« watchoo tui » déjà ouvert les prendra en compte à son prochain lancement.\n")
	}
	return b.String()
}
