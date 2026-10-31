package cli

// Mise à jour de Watchoo (0.6.0) : avis de nouvelle version à chaque commande, et
// « watchoo update [--check] [--yes] ». La logique (dernière version, comparaison, téléchargement
// vérifié, remplacement du binaire) est dans internal/selfupdate ; ici, seulement l'affichage.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/tui"
)

// Updater lit les releases de Watchoo et installe une version ; *selfupdate.Client le satisfait.
type Updater interface {
	// Check lit la dernière version publiée et la compare à current.
	Check(ctx context.Context, current string) (selfupdate.Status, error)
	// Install télécharge la release tag, vérifie sa somme SHA-256 et remplace exe.
	Install(ctx context.Context, tag, exe string) error
}

var _ Updater = (*selfupdate.Client)(nil)

// Délais de la mise à jour.
var (
	// updateNoticeTimeout est le délai total de la vérification de l'avis, compté depuis le début de
	// la commande : une commande n'est jamais retardée de plus (variable pour les tests).
	updateNoticeTimeout = 1500 * time.Millisecond
	// updateCheckTimeout borne la lecture de la dernière version par « watchoo update ».
	updateCheckTimeout = 30 * time.Second
	// updateInstallTimeout borne le téléchargement et l'installation.
	updateInstallTimeout = 10 * time.Minute
)

// noticeSlot reçoit l'avis lancé par le PersistentPreRun de la commande racine, pour que Run le
// termine après la commande.
type noticeSlot struct {
	notice *updateNotice
}

// updateNotice est une vérification de nouvelle version lancée en arrière-plan au début d'une
// commande. Une valeur nil (vérification sautée) est valide : finish et abandon n'y font rien.
type updateNotice struct {
	done     chan struct{} // fermé quand status / err sont prêts
	status   selfupdate.Status
	err      error
	deadline time.Time
	cancel   context.CancelFunc
	logger   *slog.Logger
	docker   bool
}

// noNoticeCommands n'ont jamais d'avis : update vérifie de toute façon, serve et tui ont leur propre
// vérification (tui l'affiche dans son en-tête), l'aide et la complétion ne sont pas des commandes de
// travail.
var noNoticeCommands = map[string]bool{
	"update": true, "serve": true, "tui": true, "help": true, "completion": true,
	cobra.ShellCompRequestCmd: true, cobra.ShellCompNoDescRequestCmd: true,
}

// updateCheckEnabled indique si les nouvelles versions de current sont vérifiées : pas quand
// WATCHOO_NO_UPDATE_CHECK la coupe, ni pour une version qui n'est pas une version de release. Décision
// commune à l'avis des commandes, à « watchoo serve » et à « watchoo tui ».
func updateCheckEnabled(current string) bool {
	return !selfupdate.CheckDisabled(os.Getenv) && selfupdate.IsRelease(current)
}

// startUpdateNotice lance la vérification de l'avis pour la commande cmd (arguments args), sauf si
// elle doit être sautée (voir 05-cli.md, « Avis de nouvelle version ») : --json, sortie d'erreur hors
// terminal, vérification coupée (updateCheckEnabled), commandes de noNoticeCommands, aide d'un groupe,
// mot inconnu après « watchoo ». « watchoo » seul (carte et aide) a son avis. Elle renvoie nil si la
// vérification est sautée.
func startUpdateNotice(ctx context.Context, deps Deps, opts *options, cmd, root *cobra.Command, args []string) *updateNotice {
	current := appVersion()
	switch {
	case opts.json,
		noNoticeCommands[cmd.Name()],
		cmd == root && len(args) > 0, // « watchoo inconnu » : une erreur
		!cmd.Runnable(),              // « watchoo repo » seul : son aide
		!updateCheckEnabled(current),
		!deps.StderrIsTerminal():
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logger := slog.New(slog.DiscardHandler)
	if opts.verbose {
		logger = deps.Logger
		if logger == nil {
			logger = newLogger(deps.Stderr, "", true)
		}
	}
	n := &updateNotice{
		done:     make(chan struct{}),
		deadline: time.Now().Add(updateNoticeTimeout),
		logger:   logger,
		docker:   deps.InDocker(),
	}
	checkCtx, cancel := context.WithDeadline(ctx, n.deadline)
	n.cancel = cancel
	go func() {
		defer close(n.done)
		n.status, n.err = deps.Updater.Check(checkCtx, current)
	}()
	return n
}

// finish attend la vérification jusqu'à son échéance au plus (jamais au-delà de updateNoticeTimeout
// après le début de la commande), puis écrit l'avis sur la sortie d'erreur si une version plus récente
// est publiée. Une réponse déjà arrivée l'emporte toujours : après une commande plus longue que le
// délai, l'échéance est passée mais la réponse est là, et select choisirait au hasard entre les deux.
// Échec, délai dépassé ou commande interrompue : rien (log debug avec --verbose).
func (n *updateNotice) finish(ctx context.Context, deps Deps) {
	if n == nil {
		return
	}
	defer n.cancel()
	select {
	case <-n.done:
	default:
		timer := time.NewTimer(time.Until(n.deadline))
		defer timer.Stop()
		select {
		case <-n.done:
		case <-timer.C:
			n.logger.Debug("vérification de la dernière version de Watchoo trop lente : avis abandonné")
			return
		case <-ctx.Done():
			return
		}
	}
	if n.err != nil {
		n.logger.Debug("vérification de la dernière version de Watchoo impossible", "erreur", n.err)
		return
	}
	if n.status.UpdateAvailable {
		fmt.Fprintln(deps.Stderr, updateNoticeText(n.status, n.docker))
	}
}

// abandon arrête la vérification sans rien afficher (commande interrompue).
func (n *updateNotice) abandon() {
	if n != nil {
		n.cancel()
	}
}

// updateNoticeText est la phrase de l'avis de nouvelle version (CLI et « watchoo serve ») ; celle de
// « watchoo tui » est écrite par le TUI (tui.UpdateInfo).
func updateNoticeText(st selfupdate.Status, docker bool) string {
	how := i18n.T("cli.sync.update_how")
	if docker {
		how = i18n.T("cli.sync.update_docker_how")
	}
	return i18n.T("cli.sync.update_notice", st.Current, st.Latest, how)
}

// tuiUpdateCheck renvoie la vérification passée à « watchoo tui » (tui.Deps.UpdateCheck) : la nouvelle
// version si une version plus récente est publiée, une UpdateInfo vide sinon. nil si la vérification
// est coupée (updateCheckEnabled). Le délai est celui du TUI.
func tuiUpdateCheck(deps Deps) func(ctx context.Context) (tui.UpdateInfo, error) {
	current := appVersion()
	if !updateCheckEnabled(current) {
		return nil
	}
	docker := deps.InDocker()
	return func(ctx context.Context) (tui.UpdateInfo, error) {
		st, err := deps.Updater.Check(ctx, current)
		if err != nil || !st.UpdateAvailable {
			return tui.UpdateInfo{}, err
		}
		return tui.UpdateInfo{Current: st.Current, Latest: st.Latest, Docker: docker}, nil
	}
}

// newUpdateMonitor renvoie la vérification périodique de « watchoo serve » (au démarrage puis toutes
// les 30 min), ou nil si elle est coupée (WATCHOO_NO_UPDATE_CHECK) ou si la version installée n'est pas
// une version de release. Une nouvelle version est annoncée une fois sur la sortie d'erreur (même
// phrase que l'avis des commandes) et journalisée (info).
func newUpdateMonitor(deps Deps, logger *slog.Logger) *selfupdate.Monitor {
	current := appVersion()
	if !updateCheckEnabled(current) {
		logger.Debug("vérification des nouvelles versions de Watchoo désactivée", "version", current)
		return nil
	}
	docker := deps.InDocker()
	return selfupdate.NewMonitor(selfupdate.MonitorOptions{
		Checker: deps.Updater,
		Current: current,
		Logger:  logger,
		OnUpdate: func(st selfupdate.Status) {
			logger.Info("nouvelle version de Watchoo disponible", "installée", st.Current, "publiée", st.Latest)
			fmt.Fprintln(deps.Stderr, updateNoticeText(st, docker))
		},
	})
}

// updateReport est la sortie JSON de « watchoo update --check ».
type updateReport struct {
	Current         string `json:"current"`          // version installée
	Latest          string `json:"latest"`           // dernière version publiée
	UpdateAvailable bool   `json:"update_available"` // une version plus récente est publiée
	ReleaseURL      string `json:"release_url"`      // page de la release de latest
}

// newUpdateCmd construit « watchoo update [--check] [--yes] », qui n'a besoin ni d'un espace ni de gh.
func newUpdateCmd(deps Deps, opts *options) *cobra.Command {
	var check, yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: i18n.T("cli.sync.update_short"),
		Long:  i18n.T("cli.sync.update_long"),
		Example: `  watchoo update --check
  watchoo update
  watchoo update --yes`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				return runUpdateCheck(cmd.Context(), deps, opts)
			}
			return runUpdate(cmd.Context(), deps, opts, yes)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, i18n.T("cli.sync.flag_check"))
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("cli.sync.flag_yes"))
	return cmd
}

// checkLatest lit la dernière version, avec updateCheckTimeout.
func checkLatest(ctx context.Context, deps Deps) (selfupdate.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	st, err := deps.Updater.Check(ctx, appVersion())
	if err != nil {
		return selfupdate.Status{}, fmt.Errorf("%s%s%w", i18n.T("cli.sync.update_check_failed"),
			i18n.T("cli.core.err.sep"), err)
	}
	return st, nil
}

// runUpdateCheck affiche la version installée, la dernière publiée et, s'il y a lieu, la marche à
// suivre.
func runUpdateCheck(ctx context.Context, deps Deps, opts *options) error {
	st, err := checkLatest(ctx, deps)
	if err != nil {
		return err
	}
	if opts.json {
		return writeJSON(deps.Stdout, updateReport{Current: st.Current, Latest: st.Latest,
			UpdateAvailable: st.UpdateAvailable, ReleaseURL: st.ReleaseURL})
	}
	out := i18n.T("cli.sync.update_check_versions", st.Current, st.Latest)
	switch {
	case st.UpdateAvailable:
		out += i18n.T("cli.sync.update_check_available", st.ReleaseURL)
		if deps.InDocker() {
			out += i18n.T("cli.sync.update_check_docker")
		} else {
			out += i18n.T("cli.sync.update_check_how")
		}
	case !selfupdate.IsRelease(st.Current):
		out += i18n.T("cli.sync.update_check_dev")
	default:
		out += i18n.T("cli.sync.update_check_uptodate")
	}
	if _, err := io.WriteString(deps.Stdout, out); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.sync.update_print_check_err"),
			i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// runUpdate installe la dernière version après confirmation (sauf yes), ou refuse avec un message qui
// dit quoi faire.
func runUpdate(ctx context.Context, deps Deps, opts *options, yes bool) error {
	if opts.json {
		return errors.New(i18n.T("cli.sync.update_no_json"))
	}
	current := appVersion()
	if err := selfupdate.CheckInstallable(current, deps.InDocker()); err != nil {
		return installableError(err, current)
	}
	exe, err := deps.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	if err := selfupdate.CheckWritable(dir); err != nil {
		return notWritableError(err, dir)
	}

	st, err := checkLatest(ctx, deps)
	if err != nil {
		return err
	}
	if !st.UpdateAvailable {
		_, err := fmt.Fprintf(deps.Stdout, "%s", i18n.T("cli.sync.update_uptodate", st.Current, st.Latest))
		return err
	}
	if _, err := fmt.Fprintf(deps.Stdout, "%s", i18n.T("cli.sync.update_plan", st.Current, st.Latest,
		st.ReleaseURL, st.ChangelogURL, exe)); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.sync.update_print_err"),
			i18n.T("cli.core.err.sep"), err)
	}
	if !yes {
		ok, err := confirm(ctx, deps, i18n.T("cli.sync.update_confirm", st.Latest))
		if err != nil {
			return err
		}
		if !ok {
			return errors.New(i18n.T("cli.sync.update_cancelled"))
		}
	}

	fmt.Fprintf(deps.Stdout, "%s", i18n.T("cli.sync.update_downloading", st.Latest, runtime.GOOS, runtime.GOARCH))
	installCtx, cancel := context.WithTimeout(ctx, updateInstallTimeout)
	defer cancel()
	if err := deps.Updater.Install(installCtx, st.Latest, exe); err != nil {
		if errors.Is(err, selfupdate.ErrNotWritable) {
			return notWritableError(err, dir)
		}
		return fmt.Errorf("%s%s%w", i18n.T("cli.sync.update_failed"),
			i18n.T("cli.core.err.sep"), err)
	}
	_, err = fmt.Fprintf(deps.Stdout, "%s", i18n.T("cli.sync.update_done", st.Latest, exe))
	return err
}

// installableError traduit un refus de selfupdate.CheckInstallable en message qui dit quoi faire.
func installableError(err error, current string) error {
	switch {
	case errors.Is(err, selfupdate.ErrDocker):
		return &hintError{err: err, msg: i18n.T("cli.sync.update_err_docker")}
	case errors.Is(err, selfupdate.ErrDevBuild):
		return &hintError{err: err, msg: i18n.T("cli.sync.update_err_devbuild", current)}
	}
	return err
}

// notWritableError explique un répertoire du binaire non accessible en écriture.
func notWritableError(err error, dir string) error {
	return &hintError{err: err, msg: i18n.T("cli.sync.update_err_not_writable", dir)}
}
