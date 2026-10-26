package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version est la version de Watchoo, injectée à la compilation (voir la cible build du Makefile) :
//
//	go build -ldflags "-X github.com/studoo-app/watchoo/internal/cli.Version=v0.1.0" ./cmd/watchoo
//
// Elle vaut « dev » pour un binaire compilé sans cette option.
var Version = "dev"

// versionReport est la sortie de « watchoo version ».
type versionReport struct {
	Version string `json:"version"` // version de Watchoo
	Go      string `json:"go"`      // version de Go utilisée pour la compilation
	OS      string `json:"os"`      // système cible
	Arch    string `json:"arch"`    // architecture cible
}

// newVersionCmd construit « watchoo version », qui n'a besoin ni d'un espace ni de gh.
func newVersionCmd(deps Deps, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Affiche la version de Watchoo",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := versionReport{
				Version: Version,
				Go:      runtime.Version(),
				OS:      runtime.GOOS,
				Arch:    runtime.GOARCH,
			}
			if opts.json {
				return writeJSON(deps.Stdout, report)
			}
			_, err := fmt.Fprintf(deps.Stdout, "watchoo %s (%s, %s/%s)\n",
				report.Version, report.Go, report.OS, report.Arch)
			return err
		},
	}
}
