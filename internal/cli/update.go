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

	"github.com/studoo-app/watchoo/internal/selfupdate"
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

// noNoticeCommands n'ont jamais d'avis : update vérifie de toute façon, serve a sa propre
// vérification, l'aide et la complétion ne sont pas des commandes de travail.
var noNoticeCommands = map[string]bool{
	"update": true, "serve": true, "help": true, "completion": true,
	cobra.ShellCompRequestCmd: true, cobra.ShellCompNoDescRequestCmd: true,
}

// startUpdateNotice lance la vérification de l'avis pour la commande cmd, sauf si elle doit être
// sautée (voir 05-cli.md, « Avis de nouvelle version ») : --json, sortie d'erreur hors terminal,
// version qui n'est pas une version de release, commandes de noNoticeCommands, « watchoo » seul (aide),
// variable WATCHOO_NO_UPDATE_CHECK. Elle renvoie nil si la vérification est sautée.
func startUpdateNotice(ctx context.Context, deps Deps, opts *options, cmd, root *cobra.Command) *updateNotice {
	current := appVersion()
	switch {
	case opts.json,
		noNoticeCommands[cmd.Name()],
		cmd == root && !opts.version,
		!cmd.Runnable(), // « watchoo repo » seul : son aide
		selfupdate.CheckDisabled(os.Getenv),
		!selfupdate.IsRelease(current),
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
// est publiée. Échec, délai dépassé ou commande interrompue : rien (log debug avec --verbose).
func (n *updateNotice) finish(ctx context.Context, deps Deps) {
	if n == nil {
		return
	}
	defer n.cancel()
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

// updateNoticeText est la phrase de l'avis de nouvelle version (CLI et « watchoo serve »).
func updateNoticeText(st selfupdate.Status, docker bool) string {
	how := "Mettez-la à jour avec « watchoo update »."
	if docker {
		how = "Changez le tag de l'image Docker pour la mettre à jour."
	}
	return fmt.Sprintf("Nouvelle version de Watchoo disponible : %s → %s. %s", st.Current, st.Latest, how)
}

// newUpdateMonitor renvoie la vérification périodique de « watchoo serve » (au démarrage puis toutes
// les 24 h), ou nil si elle est coupée (WATCHOO_NO_UPDATE_CHECK) ou si la version installée n'est pas
// une version de release. Une nouvelle version est annoncée une fois sur la sortie d'erreur (même
// phrase que l'avis des commandes) et journalisée (info).
func newUpdateMonitor(deps Deps, logger *slog.Logger) *selfupdate.Monitor {
	current := appVersion()
	if selfupdate.CheckDisabled(os.Getenv) || !selfupdate.IsRelease(current) {
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
		Short: "Met Watchoo à jour avec la dernière version publiée",
		Long: `Met Watchoo à jour avec la dernière version publiée sur GitHub : affiche la version
installée et la dernière, demande confirmation, télécharge l'archive de la release pour
ce système, vérifie sa somme SHA-256 et remplace le binaire d'un bloc.

--check vérifie seulement, sans rien installer (--json possible). Seules les pages
publiques des releases de Watchoo sont lues : ni token, ni API GitHub.

Refusé dans l'image Docker (changez le tag de l'image), pour une version compilée
depuis les sources (go install …@latest, ou git pull && make build) et si le
répertoire du binaire n'est pas accessible en écriture.

Un « watchoo serve » déjà lancé garde l'ancienne version jusqu'à son redémarrage ; la
base de chaque espace est mise à niveau à la prochaine commande.`,
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
	cmd.Flags().BoolVar(&check, "check", false, "vérifie seulement si une nouvelle version est publiée, sans rien installer")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "installe sans poser de question (obligatoire hors terminal)")
	return cmd
}

// checkLatest lit la dernière version, avec updateCheckTimeout.
func checkLatest(ctx context.Context, deps Deps) (selfupdate.Status, error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	st, err := deps.Updater.Check(ctx, appVersion())
	if err != nil {
		return selfupdate.Status{}, fmt.Errorf("impossible de lire la dernière version de Watchoo : %w", err)
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
	out := fmt.Sprintf("Version installée : %s\nDernière version : %s\n", st.Current, st.Latest)
	switch {
	case st.UpdateAvailable:
		out += "Nouvelle version disponible : " + st.ReleaseURL + "\n"
		if deps.InDocker() {
			out += "Changez le tag de l'image Docker pour la mettre à jour.\n"
		} else {
			out += "Mettez-la à jour avec « watchoo update ».\n"
		}
	case !selfupdate.IsRelease(st.Current):
		out += "Version de développement (compilée depuis les sources) : comparaison impossible.\n"
	default:
		out += "Watchoo est à jour.\n"
	}
	if _, err := io.WriteString(deps.Stdout, out); err != nil {
		return fmt.Errorf("affichage de la vérification : %w", err)
	}
	return nil
}

// runUpdate installe la dernière version après confirmation (sauf yes), ou refuse avec un message qui
// dit quoi faire.
func runUpdate(ctx context.Context, deps Deps, opts *options, yes bool) error {
	if opts.json {
		return errors.New("« watchoo update » n'a de sortie JSON qu'avec --check : utilisez « watchoo update --check --json »")
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
		_, err := fmt.Fprintf(deps.Stdout, "Watchoo %s est à jour (dernière version publiée : %s).\n", st.Current, st.Latest)
		return err
	}
	if _, err := fmt.Fprintf(deps.Stdout, "Mise à jour de Watchoo : %s → %s\nNotes de version : %s\n"+
		"Journal des modifications : %s\nBinaire : %s\n", st.Current, st.Latest, st.ReleaseURL, st.ChangelogURL, exe); err != nil {
		return fmt.Errorf("affichage de la mise à jour : %w", err)
	}
	if !yes {
		ok, err := confirm(ctx, deps, fmt.Sprintf("Installer Watchoo %s ?", st.Latest))
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("mise à jour annulée : rien n'a été modifié")
		}
	}

	fmt.Fprintf(deps.Stdout, "Téléchargement de Watchoo %s (%s/%s)…\n", st.Latest, runtime.GOOS, runtime.GOARCH)
	installCtx, cancel := context.WithTimeout(ctx, updateInstallTimeout)
	defer cancel()
	if err := deps.Updater.Install(installCtx, st.Latest, exe); err != nil {
		if errors.Is(err, selfupdate.ErrNotWritable) {
			return notWritableError(err, dir)
		}
		return fmt.Errorf("mise à jour impossible (binaire inchangé) : %w", err)
	}
	_, err = fmt.Fprintf(deps.Stdout, "Somme SHA-256 vérifiée. Watchoo %s installé dans %s.\n"+
		"Un « watchoo serve » déjà lancé garde l'ancienne version jusqu'à son redémarrage ; la base de chaque\n"+
		"espace est mise à niveau (migrations) à la prochaine commande.\n", st.Latest, exe)
	return err
}

// installableError traduit un refus de selfupdate.CheckInstallable en message qui dit quoi faire.
func installableError(err error, current string) error {
	switch {
	case errors.Is(err, selfupdate.ErrDocker):
		return &hintError{err: err, msg: "« watchoo update » ne s'utilise pas dans l'image Docker : changez le tag " +
			"de l'image (ghcr.io/studoo-app/watchoo:<version>) puis recréez le conteneur, ou, avec compose, " +
			"changez le tag dans docker-compose.yml puis « docker compose pull && docker compose up -d »"}
	case errors.Is(err, selfupdate.ErrDevBuild):
		return &hintError{err: err, msg: fmt.Sprintf("la version %s a été compilée depuis les sources : "+
			"« watchoo update » ne la remplace pas ; utilisez « go install github.com/studoo-app/watchoo/cmd/watchoo@latest », "+
			"ou « git pull && make build » dans le clone", current)}
	}
	return err
}

// notWritableError explique un répertoire du binaire non accessible en écriture.
func notWritableError(err error, dir string) error {
	return &hintError{err: err, msg: fmt.Sprintf("le répertoire du binaire, %s, n'est pas accessible en écriture : "+
		"vérifiez ses droits, relancez avec sudo, ou réinstallez Watchoo dans un répertoire à vous avec le script "+
		"d'installation (WATCHOO_INSTALL_DIR, voir le README, « Mise à jour »)", dir)}
}
