package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// usageTemplate est le modèle d'usage de cobra, traduit en français (les groupes de commandes, que
// Watchoo n'utilise pas, en sont retirés).
const usageTemplate = `Utilisation :{{if .Runnable}}
  {{useLineFR .UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [commande]{{end}}{{if gt (len .Aliases) 0}}

Alias :
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Exemples :
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Commandes :{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Options :
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Options globales :
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Lancez « {{.CommandPath}} [commande] --help » pour l'aide d'une commande.{{end}}
`

func init() {
	// cobra ajoute « [flags] » à la ligne d'usage de toute commande qui a des options.
	cobra.AddTemplateFunc("useLineFR", func(line string) string {
		return strings.Replace(line, "[flags]", "[options]", 1)
	})
}

// localize traduit en français ce que cobra affiche par défaut en anglais : modèle d'usage, option
// --help, commande help et erreurs d'options. La commande completion que cobra ajoute d'office est
// désactivée : son aide et ses sous-commandes sont en anglais, et elle n'est pas documentée
// (documentation/05-cli.md).
func localize(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplate)
	root.PersistentFlags().BoolP("help", "h", false, "affiche l'aide de la commande")
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [commande]",
		Short: "Affiche l'aide d'une commande",
		Long:  "Affiche l'aide d'une commande : « watchoo help init », par exemple.",
		RunE: func(c *cobra.Command, args []string) error {
			cmd, rest, err := c.Root().Find(args)
			if err != nil || cmd == nil || len(rest) > 0 {
				return fmt.Errorf("aide : commande inconnue « %s » (liste des commandes : « %s --help »)",
					strings.Join(args, " "), c.Root().CommandPath())
			}
			return cmd.Help()
		},
	})
	root.CompletionOptions.DisableDefaultCmd = true
	root.SuggestionsMinimumDistance = 2
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &hintError{
			msg: fmt.Sprintf("%s (aide : « %s --help »)", translateFlagError(err), c.CommandPath()),
			err: err,
		}
	})
}

// translateFlagError traduit en français une erreur d'analyse des options renvoyée par pflag, à partir
// de son type (et non de son texte, en anglais) : seuls les éléments donnés par l'utilisateur (nom de
// l'option, valeur) sont repris. Une erreur d'un autre type est renvoyée telle quelle.
func translateFlagError(err error) string {
	var (
		notExist      *pflag.NotExistError
		valueRequired *pflag.ValueRequiredError
		invalidValue  *pflag.InvalidValueError
		invalidSyntax *pflag.InvalidSyntaxError
	)
	switch {
	case errors.As(err, &notExist):
		// Pour une option courte, pflag donne sa lettre et ce qui restait du groupe (« -vZx ») :
		// seule la lettre est reprise.
		if notExist.GetSpecifiedShortnames() != "" {
			return "option courte inconnue : -" + notExist.GetSpecifiedName()
		}
		return "option inconnue : --" + notExist.GetSpecifiedName()
	case errors.As(err, &valueRequired):
		dashes := "--"
		if valueRequired.GetSpecifiedShortnames() != "" {
			dashes = "-"
		}
		return fmt.Sprintf("l'option %s%s attend une valeur", dashes, valueRequired.GetSpecifiedName())
	case errors.As(err, &invalidValue):
		msg := fmt.Sprintf("valeur invalide %q pour --%s", invalidValue.GetValue(), invalidValue.GetFlag().Name)
		if expected := expectedValue(invalidValue.GetFlag()); expected != "" {
			msg += " : " + expected
		}
		return msg
	case errors.As(err, &invalidSyntax):
		return "option mal écrite : " + invalidSyntax.GetSpecifiedFlag()
	}
	return err.Error()
}

// expectedValue décrit en français la valeur attendue par une option, selon son type pflag ; chaîne
// vide si le type n'a pas de description.
func expectedValue(f *pflag.Flag) string {
	switch f.Value.Type() {
	case "bool":
		return "true ou false attendu"
	case "duration":
		return "durée attendue, ex. 30m ou 1h"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "nombre entier attendu"
	}
	return ""
}
