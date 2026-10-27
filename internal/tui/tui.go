// Package tui est le mode interactif de Watchoo (« watchoo tui »), construit avec Bubble Tea v2
// (charm.land/bubbletea/v2), ses composants (charm.land/bubbles/v2) et lipgloss v2.
//
// Couche d'affichage seulement : les données viennent de DataSource (internal/store), les états
// d'activité et formats courts de internal/activity, les synchros de Syncer (internal/sync). Aucune
// règle métier ici.
//
// Vues : dépôts (nom, état, dernier commit, auteur), commits, contributeurs ; détail d'un dépôt ;
// filtre ; aide des touches. En-tête : hibou mini (internal/logo) aux pupilles vertes.
//
// Touches (voir keys.go) : tab / maj+tab ou 1, 2, 3 changent de vue ; ↑ ↓ (j k) déplacent la
// sélection ; entrée ouvre le détail du dépôt, échap le ferme ; « / » filtre ; « s » synchronise tout ;
// « S » synchronise le dépôt sélectionné ; « r » relit la base ; « ? » affiche l'aide ; « q » ou
// ctrl+c quitte.
//
// Synchro automatique : toutes les Deps.Interval (0 = manuelle seule), par une commande tea.Tick ;
// la synchro tourne dans une commande Bubble Tea (goroutine), jamais dans Update, et une seule à la
// fois côté TUI (une demande pendant une synchro est ignorée et signalée) ; les événements de
// progression arrivent par messages (canal lu par une commande,
// voir syncer.go). Une synchro manuelle ne décale pas la prochaine synchro automatique.
//
// Tests : en appelant Init / Update / View du modèle directement (pas de teatest), avec un store
// temporaire et un faux Syncer ; Run est testé avec le vrai moteur (quitter pendant une synchro).
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// DataSource est la lecture des données affichées ; *store.Store la satisfait.
type DataSource interface {
	RepositorySummaries(ctx context.Context) ([]store.RepositorySummary, error)
	ListRecentCommits(ctx context.Context, f store.CommitFilter) ([]store.CommitView, error)
	ContributorRanking(ctx context.Context, f store.ContributorFilter) ([]store.ContributorStats, error)
	AuthorCommitTimes(ctx context.Context, repoID int64, since time.Time) ([]model.AuthorCommitTime, error)
}

// Syncer lance les synchros ; *wsync.Engine le satisfait.
type Syncer interface {
	SyncAll(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error)
	SyncRepo(ctx context.Context, repoID int64, opts wsync.SyncOptions) (wsync.Report, error)
}

// Vérifie à la compilation que les types du cœur satisfont les interfaces.
var (
	_ DataSource = (*store.Store)(nil)
	_ Syncer     = (*wsync.Engine)(nil)
)

// Deps regroupe les dépendances du TUI.
type Deps struct {
	Data   DataSource
	Syncer Syncer
	// Config est la configuration de l'espace (nom affiché dans l'en-tête, seuils d'activité).
	Config workspace.Config
	// Location est le fuseau d'affichage (Config.Location) ; nil = time.Local.
	Location *time.Location
	// Interval est l'intervalle de synchro automatique déjà validé (wsync.AutoInterval) ; 0 = manuelle.
	Interval time.Duration
	Now      func() time.Time // horloge ; nil = time.Now
	Version  string           // version affichée dans l'en-tête (cli.Version)
	Unicode  bool             // terminal Unicode (sinon hibou ascii-mini-pur, symboles ASCII)
	Color    bool             // couleurs permises (faux si NO_COLOR ou TERM=dumb)
	// CommitLimit est le nombre de commits chargés dans la vue commits ; 0 = 200.
	CommitLimit int
}

// IO décrit le terminal où tourne le TUI.
type IO struct {
	In  io.Reader // entrée clavier (os.Stdin)
	Out io.Writer // sortie (os.Stdout)
}

// ErrNotTerminal est renvoyée par la CLI quand « watchoo tui » n'est pas lancé sur un terminal.
var ErrNotTerminal = errors.New("« watchoo tui » doit être lancé dans un terminal interactif " +
	"(utilisez les autres commandes, comme « watchoo repo list », dans un script)")

// Run lance le TUI (écran alternatif) jusqu'à « q », ctrl+c ou l'annulation de ctx, et renvoie
// l'erreur éventuelle de Bubble Tea. La vérification « est-ce un terminal » est faite par l'appelant.
// Pendant le TUI, l'appelant redirige les logs slog hors du terminal (io.Discard par défaut).
//
// En quittant, Run annule la synchro en cours et l'attend (au plus quitSyncTimeout) avant de rendre
// la main : Bubble Tea n'attend pas ses commandes, et l'appelant ferme la base juste après ; le
// moteur doit d'abord clore ses sync_runs (« synchro annulée »), sinon ils resteraient « running » et
// verrouilleraient les dépôts.
func Run(ctx context.Context, deps Deps, term IO) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m := NewModel(ctx, deps)
	_, err := tea.NewProgram(m, tea.WithContext(ctx),
		tea.WithInput(term.In), tea.WithOutput(term.Out)).Run()
	cancel()
	if !m.tracker.closeAndWait(quitSyncTimeout) && err == nil {
		err = errors.New("la synchro en cours ne s'est pas arrêtée à temps")
	}
	if err != nil {
		return fmt.Errorf("mode interactif : %w", err)
	}
	return nil
}
