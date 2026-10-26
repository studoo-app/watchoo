// Package store persiste les données de Watchoo dans SQLite (une base par espace de supervision,
// .watchoo/watchoo.db).
//
// Organisation :
//   - migrations/ : migrations goose embarquées (schéma de référence, lu aussi par sqlc) ;
//   - queries/    : requêtes SQL nommées pour sqlc ;
//   - db/         : code Go généré par « sqlc generate » (jamais modifié à la main).
//
// Les types générés par sqlc (package db) ne sortent pas de store : les autres packages utilisent les
// méthodes de Store, qui reçoivent et renvoient des types de internal/model. Aucune méthode exportée
// ne renvoie de type du package db (le code de store utilise le champ queries).
//
// Driver : modernc.org/sqlite (nom de driver « sqlite », sans CGO).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"modernc.org/sqlite" // enregistre aussi le driver database/sql « sqlite »
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/studoo-app/watchoo/internal/store/db"
	"github.com/studoo-app/watchoo/internal/store/migrations"
)

// TimeLayout est le format fixe des dates en base : ISO 8601, UTC, à la seconde, suffixe « Z » littéral
// (ex. « 2026-10-26T14:03:00Z »). Il se trie correctement comme du texte.
const TimeLayout = "2006-01-02T15:04:05Z"

// DriverName est le nom du driver database/sql enregistré par modernc.org/sqlite.
const DriverName = "sqlite"

// DSNPragmas sont les pragmas appliqués par le driver à chaque nouvelle connexion :
// clés étrangères actives, attente de 5 s sur une base verrouillée, journal WAL.
const DSNPragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

// FileMode est le mode du fichier de base (lisible par l'utilisateur seul) ; les fichiers -wal et -shm
// créés par SQLite héritent des droits du fichier de base.
const FileMode = 0o600

// MigrationDialect est le dialecte goose utilisé pour appliquer les migrations.
const MigrationDialect = goose.DialectSQLite3

// Erreurs renvoyées par les méthodes de Store (à tester avec errors.Is).
var (
	// ErrNotFound : l'élément demandé n'existe pas (remplace sql.ErrNoRows hors de store).
	ErrNotFound = errors.New("élément introuvable")
	// ErrAlreadyExists : une contrainte d'unicité est violée (ex. dépôt déjà suivi).
	ErrAlreadyExists = errors.New("élément déjà existant")
)

// Store donne accès à une base SQLite Watchoo ouverte. Il est sûr pour un usage concurrent
// (comme *sql.DB).
type Store struct {
	db      *sql.DB
	queries *db.Queries
}

// Open ouvre la base SQLite située à path et renvoie un Store prêt à l'emploi. Il n'applique pas les
// migrations : l'appelant appelle Migrate ensuite.
//
// Comportement :
//   - appelle d'abord CreateDBFile(path), pour que le fichier existe en 0600 avant que SQLite ne
//     l'ouvre (les fichiers -wal / -shm héritent alors de ces droits) ;
//   - ouvre la base avec sql.Open(DriverName, dsn), le DSN étant « file: » + chemin + « ? » + DSNPragmas,
//     le chemin étant échappé pour que les espaces, « ? », « # » et « % » qu'il contient ne soient pas
//     interprétés ;
//   - vérifie la connexion (PingContext) et que foreign_keys vaut bien 1 ; en cas d'échec, ferme la
//     base et renvoie une erreur enveloppée qui mentionne path ;
//   - crée le *db.Queries associé.
func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("ouverture de la base : chemin vide")
	}
	if err := CreateDBFile(path); err != nil {
		return nil, fmt.Errorf("ouverture de la base %s : %w", path, err)
	}

	sqlDB, err := sql.Open(DriverName, dsn(path))
	if err != nil {
		return nil, fmt.Errorf("ouverture de la base %s : %w", path, err)
	}
	if err := checkConnection(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ouverture de la base %s : %w", path, err)
	}

	return &Store{db: sqlDB, queries: db.New(sqlDB)}, nil
}

// checkConnection ouvre une première connexion (ce qui applique les pragmas du DSN) et vérifie que les
// clés étrangères sont bien actives : sans elles, les cascades du schéma seraient ignorées en silence.
func checkConnection(ctx context.Context, sqlDB *sql.DB) error {
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("connexion impossible : %w", err)
	}
	var foreignKeys int
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("lecture du pragma foreign_keys : %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("les clés étrangères ne sont pas actives (foreign_keys = %d)", foreignKeys)
	}
	return nil
}

// dsnPathEscaper échappe les caractères qu'une URI SQLite (« file:… ») interpréterait : « % » (début
// d'un échappement), « ? » (début des paramètres), « # » (début du fragment), ainsi que l'espace. Le
// remplacement se fait en une seule passe : un « % » déjà présent n'est jamais échappé deux fois.
var dsnPathEscaper = strings.NewReplacer(
	"%", "%25",
	"?", "%3F",
	"#", "%23",
	" ", "%20",
)

// dsn construit le DSN modernc.org/sqlite de la base située à path : URI « file: » (le driver transmet
// alors les paramètres à SQLite, qui ignore ceux qu'il ne connaît pas) suivie de DSNPragmas.
//
// Un chemin absolu reçoit une autorité vide (« file:///chemin ») : sans elle, un chemin commençant par
// « // » serait lu comme un nom d'hôte.
func dsn(path string) string {
	p := filepath.ToSlash(path)
	if strings.HasPrefix(p, "/") {
		p = "//" + p
	}
	return "file:" + dsnPathEscaper.Replace(p) + "?" + DSNPragmas
}

// Migrate applique toutes les migrations goose embarquées (migrations.FS) qui ne le sont pas encore.
// Il est idempotent : sur une base à jour, il ne fait rien et renvoie nil.
//
// Il utilise l'API sans état global de goose (goose.NewProvider, sans registre global des migrations
// Go). goose reste silencieux ; le résultat est journalisé avec log/slog au niveau Debug.
func (s *Store) Migrate(ctx context.Context) error {
	provider, err := goose.NewProvider(MigrationDialect, s.db, migrations.FS,
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return fmt.Errorf("préparation des migrations : %w", err)
	}
	// Ne pas appeler provider.Close() : il fermerait la connexion du Store.
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("application des migrations : %w", err)
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("lecture de la version du schéma : %w", err)
	}
	slog.DebugContext(ctx, "migrations de la base appliquées",
		slog.Int("applied", len(results)),
		slog.Int64("version", version),
	)
	return nil
}

// Close ferme la base. Après Close, le Store n'est plus utilisable.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("fermeture de la base : %w", err)
	}
	return nil
}

// DB renvoie la connexion database/sql sous-jacente (migrations, transactions, requêtes écrites à la
// main dans store).
func (s *Store) DB() *sql.DB {
	return s.db
}

// FormatTime convertit t au format de stockage TimeLayout : conversion en UTC puis troncature à la
// seconde (les fractions de seconde sont abandonnées, pas arrondies).
// Exemple : 2026-10-26 15:03:00.9 +01:00 → « 2026-10-26T14:03:00Z ».
//
// L'appelant traite lui-même les dates absentes : la valeur zéro de time.Time correspond à NULL en base
// et ne doit pas être passée à FormatTime. Les années hors de 0000-9999 ne respectent pas le format
// (refusées par les contraintes CHECK du schéma).
func FormatTime(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(TimeLayout)
}

// ParseTime analyse une date stockée au format TimeLayout et renvoie un time.Time en UTC.
// Toute autre forme (chaîne vide, fraction de seconde, décalage horaire « +02:00 », date invalide) est
// refusée avec une erreur qui cite la valeur reçue.
func ParseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("date vide : format attendu %s", TimeLayout)
	}
	t, err := time.Parse(TimeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("date %q invalide (format attendu %s) : %w", s, TimeLayout, err)
	}
	// time.Parse tolère des variantes que le format de stockage interdit (fraction de seconde après
	// les secondes, heure sur un seul chiffre…) : seule la forme canonique est acceptée.
	if t.Format(TimeLayout) != s {
		return time.Time{}, fmt.Errorf("date %q invalide : format attendu %s (UTC, à la seconde)", s, TimeLayout)
	}
	return t.UTC(), nil
}

// CreateDBFile crée à path un fichier vide en mode 0600 s'il n'existe pas encore. Si le fichier existe
// déjà, il n'est ni modifié ni tronqué, et ses droits ne sont pas changés. Le répertoire parent doit
// exister.
//
// Le mode est forcé à 0600 après création (os.Chmod), pour ne pas dépendre du umask. La création
// exclusive (O_EXCL) refuse aussi de suivre un lien symbolique placé à path. Un chemin existant qui
// n'est pas un fichier ordinaire (un répertoire, par exemple) est refusé.
func CreateDBFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, FileMode)
	if errors.Is(err, fs.ErrExist) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return fmt.Errorf("fichier de base %s : %w", path, statErr)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("fichier de base %s : ce n'est pas un fichier ordinaire", path)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("création du fichier de base %s : %w", path, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("création du fichier de base %s : %w", path, err)
	}
	if err := os.Chmod(path, FileMode); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("droits du fichier de base %s : %w", path, err)
	}
	return nil
}

// isUniqueViolation indique si err provient d'une violation de contrainte UNIQUE ou PRIMARY KEY.
func isUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}
