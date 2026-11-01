package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// Informations de compilation, injectées par -ldflags (voir la cible build du Makefile et
// .goreleaser.yaml) :
//
//	go build -ldflags "-X github.com/studoo-app/watchoo/internal/cli.Version=v0.1.0 \
//	  -X github.com/studoo-app/watchoo/internal/cli.Commit=<sha> \
//	  -X github.com/studoo-app/watchoo/internal/cli.Date=2026-10-27T10:00:00Z" ./cmd/watchoo
//
// Sans ces options, elles viennent des informations de compilation de Go quand elles existent
// (« go install …@v0.1.0 » donne la version du module ; « go build » dans un dépôt Git, la version
// du module, le commit et sa date) ; sinon Version vaut « dev », Commit et Date restent vides.
var (
	// Version est la version de Watchoo (« v0.1.0 », « dev »).
	Version = "dev"
	// Commit est le SHA complet du commit compilé.
	Commit = ""
	// Date est la date du commit compilé, en RFC 3339 UTC (« 2026-10-27T10:00:00Z ») : la même pour
	// deux compilations du même commit, comme le veut une compilation reproductible.
	Date = ""
)

// readBuildInfo lit les informations de compilation de Go ; remplacée dans les tests.
var readBuildInfo = debug.ReadBuildInfo

// versionReport est la sortie de « watchoo version ».
type versionReport struct {
	Version string     `json:"version"` // version de Watchoo
	Commit  string     `json:"commit"`  // SHA complet du commit compilé ; vide si inconnu
	Date    *time.Time `json:"date"`    // date du commit compilé (UTC) ; null si inconnue
	Go      string     `json:"go"`      // version de Go utilisée pour la compilation
	OS      string     `json:"os"`      // système cible
	Arch    string     `json:"arch"`    // architecture cible
}

// buildVersion rassemble la version, le commit et sa date : valeurs injectées par -ldflags d'abord,
// informations de compilation de Go ensuite. Une date illisible est ignorée (null en JSON).
func buildVersion() versionReport {
	report := versionReport{
		Version: Version,
		Commit:  Commit,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
	date := Date
	if info, ok := readBuildInfo(); ok && info != nil {
		if report.Version == "dev" {
			// « v0.1.0 » pour « go install …@v0.1.0 » ou un « go build » sur un tag, une pseudo-version
			// (« v0.0.0-20261027…-9412b2f170d8 ») sur un autre commit. « (devel) » (pas de Git) et une
			// version suffixée « +dirty » (modifications non commitées) gardent « dev ».
			if v := info.Main.Version; v != "" && v != "(devel)" && !strings.HasSuffix(v, "+dirty") {
				report.Version = v
			}
		}
		for _, s := range info.Settings {
			switch {
			case s.Key == "vcs.revision" && report.Commit == "":
				report.Commit = s.Value
			case s.Key == "vcs.time" && date == "":
				date = s.Value
			}
		}
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(date)); err == nil {
		report.Date = timeJSON(t)
	}
	return report
}

// newVersionCmd construit « watchoo version », qui n'a besoin ni d'un espace ni de gh.
func newVersionCmd(deps Deps, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: i18n.T("cli.core.version.short"),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(deps, opts)
		},
	}
}

// versionDetails renvoie le texte entre parenthèses de « watchoo version » : commit court et date
// (jour UTC) quand ils sont connus, puis Go et la plateforme.
func versionDetails(r versionReport) string {
	var parts []string
	if r.Commit != "" {
		parts = append(parts, "commit "+shortCommit(r.Commit))
	}
	if r.Date != nil {
		parts = append(parts, r.Date.Format(time.DateOnly))
	}
	parts = append(parts, r.Go, r.OS+"/"+r.Arch)
	return strings.Join(parts, ", ")
}

// shortCommit abrège un SHA à 7 caractères, comme Git ; une valeur plus courte reste telle quelle.
func shortCommit(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// runVersion affiche la version : un objet JSON avec --json ; sur un terminal, la carte d'en-tête
// (version de Watchoo, puis celles de Go et de la plateforme en sous-titre) et rien d'autre ; sinon
// (sortie redirigée, ou terminal trop étroit pour la carte), une seule ligne de texte. C'est aussi
// l'effet de « watchoo --version ».
func runVersion(deps Deps, opts *options) error {
	report := buildVersion()
	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	if term, ok := cardTerminal(deps, opts); ok {
		subtitle := fmt.Sprintf("%s · %s/%s", report.Go, report.OS, report.Arch)
		if card := renderCard(term, subtitle, ""); card != "" {
			if _, err := io.WriteString(deps.Stdout, card); err != nil {
				return fmt.Errorf("%s%s%w", i18n.T("cli.core.err.write_version"),
					i18n.T("cli.core.err.sep"), err)
			}
			return nil
		}
	}
	if _, err := fmt.Fprintf(deps.Stdout, "watchoo %s (%s)\n", report.Version, versionDetails(report)); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.core.err.write_version"),
			i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// appVersion renvoie la version affichée partout (carte d'en-tête, TUI, web, User-Agent) : celle de
// « watchoo version ».
func appVersion() string {
	return buildVersion().Version
}
