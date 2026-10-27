package cli

// « watchoo repo import fichier [--dry-run] » : repoimport.Parse, puis wsync.Engine.AddRepositories,
// bilan en texte ou JSON ; code de sortie non nul s'il reste des dépôts inaccessibles.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// importSummary compte les résultats d'un import.
type importSummary struct {
	Added          int `json:"added"`
	AlreadyTracked int `json:"already_tracked"`
	Updated        int `json:"updated"`
	Inaccessible   int `json:"inaccessible"`
	Errors         int `json:"errors"`
}

// importEntry est le résultat d'une ligne du fichier.
type importEntry struct {
	Line       int              `json:"line"`
	Repo       string           `json:"repo"`    // dépôt demandé (owner/repo tel qu'écrit)
	Outcome    wsync.AddOutcome `json:"outcome"` // added, already, updated, inaccessible ou error
	Repository *repositoryJSON  `json:"repository"`
	Error      string           `json:"error"`
}

// importReport est le bilan de « watchoo repo import » (sortie JSON).
type importReport struct {
	File    string            `json:"file"`
	Format  repoimport.Format `json:"format"` // csv ou list
	DryRun  bool              `json:"dry_run"`
	Summary importSummary     `json:"summary"`
	Entries []importEntry     `json:"entries"`
}

// newRepoImportCmd construit « watchoo repo import fichier [--dry-run] ».
func newRepoImportCmd(deps Deps, opts *options) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "import fichier",
		Short: "Ajoute d'un coup une liste de dépôts (CSV ou un dépôt par ligne)",
		Long: `Ajoute d'un coup une liste de dépôts, par exemple ceux d'une promotion.

Le fichier est un CSV avec une ligne d'en-tête repo,label,branches (seule la colonne
repo est obligatoire ; flemme et dead, facultatives, donnent les seuils d'activité du
dépôt en jours, 0 pour revenir aux seuils de l'espace), ou un simple texte avec un dépôt par ligne. Les lignes vides et
celles qui commencent par # sont ignorées. « - » lit la liste sur l'entrée standard.

Tout le fichier est validé avant d'ajouter quoi que ce soit : une ligne mal formée
arrête l'import en indiquant son numéro. L'accès à chaque dépôt est ensuite vérifié
auprès de GitHub ; un dépôt inaccessible est signalé sans bloquer les autres. Un dépôt
déjà suivi n'est pas ajouté en double : son label, ses branches et ses seuils sont mis à
jour si le fichier en donne (une cellule vide garde la valeur existante). L'import peut donc être
relancé sans risque.

Le code de retour est non nul s'il reste des dépôts inaccessibles. L'import ne lance
pas de synchro : enchaînez avec « watchoo sync ».`,
		Example: `  watchoo repo import promo-licence.csv --dry-run
  watchoo repo import promo-licence.csv`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoImport(cmd.Context(), deps, opts, args[0], dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"valide le fichier et vérifie l'accès aux dépôts, sans rien enregistrer")
	return cmd
}

// runRepoImport lit et valide le fichier, vérifie l'accès aux dépôts et les enregistre (sauf dryRun),
// puis affiche le bilan.
func runRepoImport(ctx context.Context, deps Deps, opts *options, file string, dryRun bool) (err error) {
	entries, format, err := readImportFile(deps, file)
	if err != nil {
		return err
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	engine, err := s.engine()
	if err != nil {
		return err
	}
	reqs := make([]wsync.AddRequest, len(entries))
	for i, e := range entries {
		reqs[i] = wsync.AddRequest{Line: e.Line, Ref: e.Ref, Label: e.Label, BranchPatterns: e.Branches,
			FlemmeAfterDays: e.FlemmeAfterDays, DeadAfterDays: e.DeadAfterDays}
	}
	results, err := engine.AddRepositories(ctx, reqs, wsync.AddOptions{DryRun: dryRun})
	if err != nil {
		return err
	}

	report := importReport{File: file, Format: format, DryRun: dryRun, Entries: make([]importEntry, 0, len(results))}
	var failed []string
	for _, res := range results {
		entry := importEntry{Line: res.Request.Line, Repo: res.Request.Ref.String(), Outcome: res.Outcome, Error: res.Error}
		switch res.Outcome {
		case wsync.OutcomeAdded:
			report.Summary.Added++
		case wsync.OutcomeAlreadyTracked:
			report.Summary.AlreadyTracked++
		case wsync.OutcomeUpdated:
			report.Summary.Updated++
		case wsync.OutcomeInaccessible:
			report.Summary.Inaccessible++
			failed = append(failed, entry.Repo)
		default:
			report.Summary.Errors++
			failed = append(failed, entry.Repo)
		}
		if res.Outcome != wsync.OutcomeInaccessible && res.Outcome != wsync.OutcomeError {
			repo := newRepositoryJSON(res.Repo)
			entry.Repository = &repo
		}
		report.Entries = append(report.Entries, entry)
	}

	if opts.json {
		err = writeJSON(deps.Stdout, report)
	} else {
		err = writeText(deps.Stdout, importText(report))
	}
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		return importFailure(report.Summary, failed)
	}
	return nil
}

// readImportFile lit et valide le fichier d'import (« - » : entrée standard). Un chemin relatif l'est
// au répertoire courant de deps.
func readImportFile(deps Deps, file string) ([]repoimport.Entry, repoimport.Format, error) {
	var r io.Reader
	if file == "-" {
		r = deps.Stdin
	} else {
		path := file
		if !filepath.IsAbs(path) {
			cwd, err := deps.Getwd()
			if err != nil {
				return nil, "", fmt.Errorf("répertoire courant inaccessible : %w", err)
			}
			path = filepath.Join(cwd, path)
		}
		f, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, "", fmt.Errorf("fichier d'import %s introuvable", file)
			}
			return nil, "", fmt.Errorf("ouverture du fichier d'import : %w", err)
		}
		defer f.Close()
		r = f
	}
	entries, format, err := repoimport.Parse(r)
	if err != nil {
		var perr *repoimport.ParseError
		if errors.As(err, &perr) {
			return nil, "", fmt.Errorf("%s : %w ; rien n'a été importé", file, err)
		}
		return nil, "", fmt.Errorf("lecture du fichier d'import %s : %w", file, err)
	}
	return entries, format, nil
}

// importText renvoie le bilan texte de l'import (voir documentation/05-cli.md, « Import groupé »),
// suivi du détail des dépôts inaccessibles ou en erreur.
func importText(r importReport) string {
	var b strings.Builder
	title := "IMPORT  " + textclean.Clean(r.File)
	if r.DryRun {
		title += "  (simulation : rien n'a été enregistré)"
	}
	b.WriteString(title + "\n")
	var inaccessible, errored []string
	for _, e := range r.Entries {
		switch e.Outcome {
		case wsync.OutcomeInaccessible:
			inaccessible = append(inaccessible, e.Repo)
		case wsync.OutcomeError:
			errored = append(errored, e.Repo)
		}
	}
	line := func(label string, n int, repos []string) {
		s := fmt.Sprintf("  %-16s %3d", label, n)
		if len(repos) > 0 {
			s += "   " + textclean.Clean(strings.Join(repos, ", "))
		}
		b.WriteString(s + "\n")
	}
	added := "ajoutés"
	if r.DryRun {
		added = "à ajouter"
	}
	line(added, r.Summary.Added, nil)
	line("déjà suivis", r.Summary.AlreadyTracked, nil)
	line("mis à jour", r.Summary.Updated, nil)
	line("inaccessibles", r.Summary.Inaccessible, inaccessible)
	if r.Summary.Errors > 0 {
		line("en erreur", r.Summary.Errors, errored)
	}
	line("lignes en erreur", 0, nil) // une ligne mal formée arrête l'import avant le bilan

	var details []string
	for _, e := range r.Entries {
		if e.Outcome == wsync.OutcomeInaccessible || e.Outcome == wsync.OutcomeError {
			details = append(details, fmt.Sprintf("  ligne %d  %s : %s", e.Line, textclean.Clean(e.Repo),
				textclean.Clean(e.Error)))
		}
	}
	if len(details) > 0 {
		b.WriteString("\n" + strings.Join(details, "\n") + "\n")
	}
	if !r.DryRun && r.Summary.Added+r.Summary.Updated > 0 {
		b.WriteString("\nLancez « watchoo sync » pour synchroniser les dépôts.\n")
	}
	return b.String()
}

// importFailure renvoie l'erreur (code de retour non nul) d'un import qui laisse des dépôts
// inaccessibles ou en erreur.
func importFailure(sum importSummary, repos []string) error {
	var parts []string
	if sum.Inaccessible > 0 {
		parts = append(parts, fmt.Sprintf("%d dépôt%s inaccessible%s", sum.Inaccessible, plural(sum.Inaccessible),
			plural(sum.Inaccessible)))
	}
	if sum.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d dépôt%s en erreur", sum.Errors, plural(sum.Errors)))
	}
	return fmt.Errorf("import incomplet : %s (%s) ; relancez l'import une fois l'accès donné",
		strings.Join(parts, ", "), strings.Join(repos, ", "))
}
