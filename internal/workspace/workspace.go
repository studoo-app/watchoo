// Package workspace gère les espaces de supervision : création par « watchoo init », recherche du
// répertoire .watchoo/ (comme Git), et chargement des réglages en trois niveaux avec viper.
//
// Voir documentation/fr/10-espaces-de-supervision.md. Ce package ne crée pas la base SQLite : c'est le
// rôle de l'appelant, via internal/store (store.CreateDBFile puis store.Open et Store.Migrate).
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// Noms des éléments d'un espace de supervision.
const (
	DirName           = ".watchoo"    // répertoire de l'espace, à la racine de celui-ci
	ConfigFileName    = "config.yaml" // réglages de l'espace, dans DirName
	DBFileName        = "watchoo.db"  // base SQLite, dans DirName
	ReposDirName      = "repos"       // clones Git locaux (V2), à la racine de l'espace ; créé au premier clone, en 0700
	GitignoreFileName = ".gitignore"  // contient « * », dans DirName
	GitignoreContent  = "*\n"         // contenu exact du .gitignore interne
	EnvDir            = "WATCHOO_DIR" // variable désignant un espace, même effet que -C
)

// Droits des éléments créés par Init : seul l'utilisateur peut lire les données (dépôts privés,
// données personnelles d'étudiants).
const (
	DirMode  = 0o700 // répertoire .watchoo/
	FileMode = 0o600 // config.yaml, .gitignore (et watchoo.db, créé par store)
)

// i18nError est une erreur sentinelle dont le message vient du catalogue i18n : le texte est traduit
// au moment de l'affichage (Error), dans la langue du processus. La valeur reste comparable avec ==
// (une structure d'un champ) : errors.Is retrouve l'erreur sentinelle à travers les emballages.
type i18nError struct{ key string }

func (e i18nError) Error() string { return i18n.T(e.key) }

// errWrapped compose un texte traduit et une erreur enveloppée, avec la ponctuation de la langue
// (« texte : err » en français, « texte: err » en anglais).
func errWrapped(text string, err error) error {
	return fmt.Errorf("%s%s%w", text, i18n.T("workspace.err.sep"), err)
}

// Erreurs du package (à tester avec errors.Is ; les fonctions les enveloppent avec le chemin concerné).
var (
	// ErrNotFound : aucun répertoire .watchoo/ dans le répertoire de départ ni dans ses parents.
	ErrNotFound = i18nError{"workspace.err.not_found"}
	// ErrAlreadyInitialized : le répertoire contient déjà un espace (.watchoo/ existe).
	ErrAlreadyInitialized = i18nError{"workspace.err.already_initialized"}
	// ErrUntrusted : l'espace trouvé n'appartient pas à l'utilisateur courant ou est accessible à
	// d'autres (droits au-delà de 0700 pour .watchoo/, 0600 pour config.yaml). Unix seulement.
	ErrUntrusted = i18nError{"workspace.err.untrusted"}
)

// createFile crée un fichier qui ne doit pas exister encore. C'est une variable pour que les tests
// puissent simuler un échec d'écriture (nettoyage de Init).
var createFile = writeNewFile

// Workspace est un espace de supervision trouvé ou créé.
type Workspace struct {
	Root string // chemin absolu et nettoyé (filepath.Clean) du répertoire qui contient .watchoo/
	Dir  string // chemin absolu de .watchoo/ : filepath.Join(Root, DirName)
	// Config contient les réglages : la valeur zéro après Find, les réglages fusionnés et validés après
	// Load ou Resolve, DefaultConfig(nom) après Init.
	Config Config
}

// DBPath renvoie le chemin de la base SQLite : Dir/watchoo.db.
func (w Workspace) DBPath() string {
	return filepath.Join(w.Dir, DBFileName)
}

// ReposDir renvoie le répertoire des clones Git locaux (V2) : Root/repos, à côté de .watchoo/ (les
// clones ont une copie de travail, à lire par l'enseignant). Il est créé en 0700 par le provider Git
// au premier clone. Hors de .watchoo/, il n'est pas couvert par son .gitignore : si la racine de
// l'espace est un dépôt Git, à l'utilisateur de l'ignorer. Find ne retient jamais un .watchoo/ placé
// dans ce répertoire (clone d'un dépôt d'étudiant).
func (w Workspace) ReposDir() string {
	return filepath.Join(w.Root, ReposDirName)
}

// ConfigPath renvoie le chemin du fichier de réglages de l'espace : Dir/config.yaml.
func (w Workspace) ConfigPath() string {
	return filepath.Join(w.Dir, ConfigFileName)
}

// Find cherche un espace à partir du répertoire start (rendu absolu avec filepath.Abs), puis dans
// chacun de ses parents jusqu'à la racine du système de fichiers, comme Git. Le premier répertoire
// qui contient un sous-répertoire nommé DirName est l'espace (un fichier nommé .watchoo ne compte pas,
// la recherche continue au-dessus). Les liens symboliques ne sont pas résolus.
//
// Exception : un .watchoo/ situé sous le répertoire des clones (ReposDirName) d'un autre espace
// appartient au dépôt d'un étudiant cloné par Watchoo (clone avec copie de travail, V2) ; il est
// ignoré et la recherche reprend à cet autre espace, qui le contient.
//
// Renvoie un *Workspace dont Root et Dir sont renseignés et Config est vide (pas de chargement des
// réglages), ou une erreur enveloppant ErrNotFound qui cite start. Une erreur d'accès inattendue
// (droits insuffisants…) arrête la recherche : continuer dans les parents risquerait de choisir un
// autre espace que celui de l'utilisateur.
func Find(start string) (*Workspace, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, errWrapped(i18n.T("workspace.find.from", start), err)
	}
	for dir := abs; ; {
		candidate := filepath.Join(dir, DirName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.IsDir():
			outer, err := managingWorkspace(dir)
			if err != nil {
				return nil, errWrapped(i18n.T("workspace.find.err"), err)
			}
			if outer != "" {
				dir = outer // .watchoo/ d'un clone : l'espace est celui qui contient le clone
				continue
			}
			return &Workspace{Root: dir, Dir: candidate}, nil
		case err == nil, isNotExist(err):
			// Pas d'espace ici (absent, ou .watchoo n'est pas un répertoire) : on remonte.
		default:
			return nil, errWrapped(i18n.T("workspace.find.err"), err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("%w (%s)", ErrNotFound, i18n.T("workspace.find.searched_from", abs))
		}
		dir = parent
	}
}

// managingWorkspace renvoie la racine de l'espace dont le répertoire des clones (ReposDirName)
// contient dir, ou "" : dir est alors dans un clone de Watchoo, pas un espace. La comparaison du nom
// « repos » ignore la casse (systèmes de fichiers de macOS et Windows).
func managingWorkspace(dir string) (string, error) {
	for child, parent := dir, filepath.Dir(dir); parent != child; child, parent = parent, filepath.Dir(parent) {
		if !strings.EqualFold(filepath.Base(child), ReposDirName) {
			continue
		}
		info, err := os.Stat(filepath.Join(parent, DirName))
		switch {
		case err == nil && info.IsDir():
			return parent, nil
		case err != nil && !isNotExist(err):
			return "", err
		}
	}
	return "", nil
}

// isNotExist indique si err signifie que le chemin n'existe pas, y compris quand un élément du chemin
// est un fichier et non un répertoire (ENOTDIR).
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// Resolve trouve l'espace sur lequel une commande (autre que init) doit travailler, puis charge ses
// réglages (Load). Il équivaut à ResolveFrom(flagDir, os.Getwd()).
func Resolve(flagDir string) (*Workspace, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errWrapped(i18n.T("workspace.resolve.cwd"), err)
	}
	return ResolveFrom(flagDir, cwd)
}

// ResolveFrom est la version de Resolve qui reçoit le répertoire courant cwd (pour les tests et la CLI,
// qui injecte son propre Getwd). Ordre de priorité :
//  1. flagDir (option -C / --workspace) s'il n'est pas vide ;
//  2. sinon la variable d'environnement WATCHOO_DIR si elle n'est pas vide ;
//  3. sinon cwd.
//
// Un chemin relatif (flagDir ou WATCHOO_DIR) est interprété par rapport à cwd. La recherche part du
// répertoire retenu avec Find (donc remonte aussi dans ses parents). Si le répertoire désigné par
// flagDir ou WATCHOO_DIR n'existe pas, l'erreur le dit clairement (en citant l'origine : option -C ou
// variable WATCHOO_DIR). Renvoie l'espace avec ses réglages chargés et validés, ou une erreur
// enveloppant ErrNotFound, ou l'erreur de Load.
func ResolveFrom(flagDir, cwd string) (*Workspace, error) {
	start, origin := cwd, ""
	switch env := os.Getenv(EnvDir); {
	case flagDir != "":
		start, origin = flagDir, i18n.T("workspace.resolve.origin_flag")
	case env != "":
		start, origin = env, i18n.T("workspace.resolve.origin_env")
	}

	if origin != "" {
		if !filepath.IsAbs(start) {
			start = filepath.Join(cwd, start)
		}
		info, err := os.Stat(start)
		switch {
		case isNotExist(err):
			return nil, fmt.Errorf("%s%s%s", origin, i18n.T("workspace.err.sep"),
				i18n.T("workspace.err.dir_missing", start))
		case err != nil:
			return nil, errWrapped(origin, err)
		case !info.IsDir():
			return nil, fmt.Errorf("%s%s%s", origin, i18n.T("workspace.err.sep"),
				i18n.T("workspace.err.not_a_dir", start))
		}
	}

	ws, err := Find(start)
	if err != nil {
		if origin != "" {
			return nil, fmt.Errorf("%s : %w", origin, err)
		}
		return nil, err
	}
	if err := Load(ws); err != nil {
		return nil, err
	}
	return ws, nil
}

// InitResult est le résultat de Init.
type InitResult struct {
	// Workspace est l'espace créé, avec Config = DefaultConfig(nom du répertoire racine).
	Workspace *Workspace
	// ParentWorkspace est la racine (Root) d'un espace parent déjà existant, si root se trouve dans un
	// espace ; vide sinon. La CLI affiche alors un avertissement : l'espace créé est imbriqué et
	// indépendant du parent.
	ParentWorkspace string
}

// Init fait de root (rendu absolu ; il doit exister et être un répertoire) un espace de supervision.
//
// Comportement :
//   - si root est sous le répertoire des clones (ReposDirName) d'un espace : erreur, rien n'est créé
//     (Find ignorerait cet espace) ;
//   - si root/.watchoo existe déjà (répertoire) : erreur enveloppant ErrAlreadyInitialized, sans rien
//     modifier ; si c'est un fichier ou autre chose : erreur claire, sans rien modifier ;
//   - cherche un espace parent avec Find(filepath.Dir(root)) et renseigne ParentWorkspace s'il en
//     trouve un (ErrNotFound n'est pas une erreur ici, une erreur d'accès non plus : seul
//     l'avertissement est perdu) ;
//   - crée root/.watchoo en DirMode (0700, forcé par chmod pour ne pas dépendre du umask) ;
//   - y écrit config.yaml (contenu ConfigTemplate(i18n.Process().Lang(), nom du répertoire racine)) et
//     .gitignore (GitignoreContent) en FileMode (0600, forcé par chmod), sans jamais écraser un fichier
//     existant (O_EXCL) ;
//   - ne crée PAS la base SQLite (voir store.CreateDBFile / store.Open) et ne vérifie pas gh : la CLI
//     vérifie gh avant d'appeler Init, puis crée et migre la base après ;
//   - en cas d'échec après la création de .watchoo/, supprime le répertoire qu'il vient de créer.
func Init(root string) (result *InitResult, err error) {
	prefix := func() string { return i18n.T("workspace.init.prefix") }
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, errWrapped(i18n.T("workspace.init.create_q", root), err)
	}
	info, err := os.Stat(abs)
	switch {
	case isNotExist(err):
		return nil, fmt.Errorf("%s%s%s", prefix(), i18n.T("workspace.err.sep"),
			i18n.T("workspace.err.dir_missing", abs))
	case err != nil:
		return nil, errWrapped(prefix(), err)
	case !info.IsDir():
		return nil, fmt.Errorf("%s%s%s", prefix(), i18n.T("workspace.err.sep"),
			i18n.T("workspace.err.not_a_dir", abs))
	}

	switch outer, err := managingWorkspace(abs); {
	case err != nil:
		return nil, errWrapped(prefix(), err)
	case outer != "":
		return nil, errors.New(i18n.T("workspace.init.in_clones", abs, ReposDirName, outer))
	}

	dir := filepath.Join(abs, DirName)
	if _, err := os.Lstat(dir); err == nil {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return nil, fmt.Errorf("%s : %w", abs, ErrAlreadyInitialized)
		}
		return nil, errors.New(i18n.T("workspace.init.watchoo_not_dir", dir))
	} else if !isNotExist(err) {
		return nil, errWrapped(prefix(), err)
	}

	result = &InitResult{}
	if parent := filepath.Dir(abs); parent != abs {
		parentWS, err := Find(parent)
		switch {
		case err == nil:
			result.ParentWorkspace = parentWS.Root
		case !errors.Is(err, ErrNotFound):
			slog.Debug("recherche d'un espace parent impossible", "racine", abs, "erreur", err)
		}
	}

	if err := os.Mkdir(dir, DirMode); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Créé entre-temps par un autre processus : on ne touche à rien.
			return nil, fmt.Errorf("%s : %w", abs, ErrAlreadyInitialized)
		}
		return nil, errWrapped(i18n.T("workspace.fs.create", dir), err)
	}
	// À partir d'ici, le répertoire est le nôtre (Mkdir a réussi) : on le supprime en cas d'échec pour
	// ne pas laisser un espace à moitié créé.
	defer func() {
		if err != nil {
			if rmErr := os.RemoveAll(dir); rmErr != nil {
				err = errors.Join(err, errWrapped(i18n.T("workspace.init.cleanup", dir), rmErr))
			}
		}
	}()
	if err := os.Chmod(dir, DirMode); err != nil {
		return nil, errWrapped(i18n.T("workspace.fs.perms", dir), err)
	}

	name := filepath.Base(abs)
	ws := &Workspace{Root: abs, Dir: dir, Config: DefaultConfig(name)}
	if err := createFile(ws.ConfigPath(), []byte(ConfigTemplate(i18n.Process().Lang(), name))); err != nil {
		return nil, err
	}
	if err := createFile(filepath.Join(dir, GitignoreFileName), []byte(GitignoreContent)); err != nil {
		return nil, err
	}

	result.Workspace = ws
	return result, nil
}

// writeNewFile crée path en FileMode (O_EXCL : jamais d'écrasement), force ces droits quel que soit le
// umask, puis y écrit content.
func writeNewFile(path string, content []byte) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, FileMode)
	if err != nil {
		return errWrapped(i18n.T("workspace.fs.create", path), err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = errWrapped(i18n.T("workspace.fs.write", path), cerr)
		}
	}()
	if err := f.Chmod(FileMode); err != nil {
		return errWrapped(i18n.T("workspace.fs.perms", path), err)
	}
	if _, err := f.Write(content); err != nil {
		return errWrapped(i18n.T("workspace.fs.write", path), err)
	}
	return nil
}
