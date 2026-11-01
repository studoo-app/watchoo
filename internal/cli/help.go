package cli

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// usageTemplate est le modèle d'usage de cobra. Les textes affichés passent par internal/i18n (tr et
// trf, évalués au moment du rendu) ; les groupes de commandes, que Watchoo n'utilise pas, en sont
// retirés.
const usageTemplate = `{{tr "cli.core.usage"}}{{if .Runnable}}
  {{useLine .UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} {{tr "cli.core.usage_command"}}{{end}}{{if gt (len .Aliases) 0}}

{{tr "cli.core.aliases"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{tr "cli.core.examples"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

{{tr "cli.core.commands"}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{tr "cli.core.options"}}
{{flagUsages .LocalFlags | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{tr "cli.core.global_options"}}
{{flagUsages .InheritedFlags | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

{{trf "cli.core.usage_more" .CommandPath}}{{end}}
`

func init() {
	// cobra ajoute « [flags] » à la ligne d'usage de toute commande qui a des options.
	cobra.AddTemplateFunc("useLine", func(line string) string {
		return strings.Replace(line, "[flags]", "[options]", 1)
	})
	cobra.AddTemplateFunc("flagUsages", flagUsages)
	cobra.AddTemplateFunc("tr", func(key string) string { return i18n.T(key) })
	cobra.AddTemplateFunc("trf", func(key string, args ...any) string { return i18n.T(key, args...) })
}

// flagUsages remplace pflag.FlagSet.FlagUsages dans l'aide, pour deux raisons : pflag écrit la valeur
// par défaut « (default …) » dans sa langue, et il aligne la colonne des descriptions en comptant des
// octets, ce qui décale les options dont le nom de valeur a des accents (« dépôt », « période »). Ici
// la valeur par défaut est écrite avec cli.core.flag.default et l'alignement compte les caractères
// (runes). Une valeur par défaut vide, nulle ou fausse n'est pas affichée, comme avec pflag. Les
// options cachées sont omises.
func flagUsages(fs *pflag.FlagSet) string {
	type row struct{ left, usage string }
	var rows []row
	width := 0
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		left := "      --" + f.Name
		if f.Shorthand != "" && f.ShorthandDeprecated == "" {
			left = "  -" + f.Shorthand + ", --" + f.Name
		}
		varname, usage := pflag.UnquoteUsage(f)
		if varname != "" {
			left += " " + varname
		}
		if f.NoOptDefVal != "" && f.Value.Type() != "bool" {
			left += "[=" + f.NoOptDefVal + "]"
		}
		if def := f.DefValue; def != "" && def != "0" && def != "false" && def != "[]" && def != "0s" {
			if f.Value.Type() == "string" {
				def = i18n.T("cli.core.flag.default_string", def)
			}
			usage += " " + i18n.T("cli.core.flag.default", def)
		}
		if f.Deprecated != "" {
			usage += " " + i18n.T("cli.core.flag.deprecated", f.Deprecated)
		}
		rows = append(rows, row{left, usage})
		width = max(width, utf8.RuneCountInString(left))
	})
	var b strings.Builder
	indent := "\n" + strings.Repeat(" ", width+3)
	for _, r := range rows {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(r.left))
		fmt.Fprintf(&b, "%s%s   %s\n", r.left, pad, strings.ReplaceAll(r.usage, "\n", indent))
	}
	return b.String()
}

// localize traduit ce que cobra affiche par défaut dans sa langue à lui (anglais) : modèle d'usage,
// option --help, commande help et erreurs d'options passent par internal/i18n. La commande completion
// que cobra ajoute d'office est désactivée : son aide et ses sous-commandes sont en anglais, et elle
// n'est pas documentée (documentation/fr/05-cli.md).
func localize(root *cobra.Command) {
	root.SetUsageTemplate(usageTemplate)
	root.PersistentFlags().BoolP("help", "h", false, i18n.T("cli.core.flag.help"))
	root.SetHelpCommand(&cobra.Command{
		Use:   i18n.T("cli.core.help.use"),
		Short: i18n.T("cli.core.help.short"),
		Long:  i18n.T("cli.core.help.long"),
		RunE: func(c *cobra.Command, args []string) error {
			cmd, rest, err := c.Root().Find(args)
			if err != nil || cmd == nil || len(rest) > 0 {
				return fmt.Errorf(i18n.T("cli.core.help.unknown"),
					strings.Join(args, " "), c.Root().CommandPath())
			}
			return cmd.Help()
		},
	})
	root.CompletionOptions.DisableDefaultCmd = true
	root.SuggestionsMinimumDistance = 2
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &hintError{
			msg: translateFlagError(err) + i18n.T("cli.core.flagerr.hint", c.CommandPath()),
			err: err,
		}
	})
}

// translateFlagError traduit une erreur d'analyse des options renvoyée par pflag, à partir de son
// type (et non de son texte, en anglais) : seuls les éléments donnés par l'utilisateur (nom de
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
			return i18n.T("cli.core.flagerr.unknown_short", notExist.GetSpecifiedName())
		}
		return i18n.T("cli.core.flagerr.unknown", notExist.GetSpecifiedName())
	case errors.As(err, &valueRequired):
		dashes := "--"
		if valueRequired.GetSpecifiedShortnames() != "" {
			dashes = "-"
		}
		return i18n.T("cli.core.flagerr.value_required", dashes, valueRequired.GetSpecifiedName())
	case errors.As(err, &invalidValue):
		msg := i18n.T("cli.core.flagerr.invalid_value", invalidValue.GetValue(), invalidValue.GetFlag().Name)
		if expected := expectedValue(invalidValue.GetFlag()); expected != "" {
			msg += i18n.T("cli.core.err.sep") + expected
		}
		return msg
	case errors.As(err, &invalidSyntax):
		return i18n.T("cli.core.flagerr.malformed", invalidSyntax.GetSpecifiedFlag())
	}
	return err.Error()
}

// expectedValue décrit la valeur attendue par une option, selon son type pflag ; chaîne vide si le
// type n'a pas de description.
func expectedValue(f *pflag.Flag) string {
	switch f.Value.Type() {
	case "bool":
		return i18n.T("cli.core.expected.bool")
	case "duration":
		return i18n.T("cli.core.expected.duration")
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return i18n.T("cli.core.expected.int")
	}
	return ""
}
