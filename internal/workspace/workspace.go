// Package workspace gère les espaces de supervision : création par « watchoo init », recherche du
// répertoire .watchoo/ (comme Git), et chargement des réglages en trois niveaux avec viper.
//
// Voir documentation/10-espaces-de-supervision.md. Ce package ne crée pas la base SQLite : c'est le
// rôle de l'appelant, via internal/store (store.CreateDBFile puis store.Open et Store.Migrate).
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// Noms des éléments d'un espace de supervision.
const (
	DirName           = ".watchoo"    // répertoire de l'espace, à la racine de celui-ci
	ConfigFileName    = "config.yaml" // réglages de l'espace, dans DirName
	DBFileName        = "watchoo.db"  // base SQLite, dans DirName
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

// Erreurs du package (à tester avec errors.Is ; les fonctions les enveloppent avec le chemin concerné).
var (
	// ErrNotFound : aucun répertoire .watchoo/ dans le répertoire de départ ni dans ses parents.
	ErrNotFound = errors.New("aucun espace Watchoo ici, lancez « watchoo init »")
	// ErrAlreadyInitialized : le répertoire contient déjà un espace (.watchoo/ existe).
	ErrAlreadyInitialized = errors.New("ce répertoire est déjà un espace Watchoo")
	// ErrUntrusted : l'espace trouvé n'appartient pas à l'utilisateur courant ou est accessible à
	// d'autres (droits au-delà de 0700 pour .watchoo/, 0600 pour config.yaml). Unix seulement.
	ErrUntrusted = errors.New("espace Watchoo refusé")
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

// ConfigPath renvoie le chemin du fichier de réglages de l'espace : Dir/config.yaml.
func (w Workspace) ConfigPath() string {
	return filepath.Join(w.Dir, ConfigFileName)
}

// Find cherche un espace à partir du répertoire start (rendu absolu avec filepath.Abs), puis dans
// chacun de ses parents jusqu'à la racine du système de fichiers, comme Git. Le premier répertoire
// qui contient un sous-répertoire nommé DirName est l'espace (un fichier nommé .watchoo ne compte pas,
// la recherche continue au-dessus). Les liens symboliques ne sont pas résolus.
//
// Renvoie un *Workspace dont Root et Dir sont renseignés et Config est vide (pas de chargement des
// réglages), ou une erreur enveloppant ErrNotFound qui cite start. Une erreur d'accès inattendue
// (droits insuffisants…) arrête la recherche : continuer dans les parents risquerait de choisir un
// autre espace que celui de l'utilisateur.
func Find(start string) (*Workspace, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, fmt.Errorf("recherche de l'espace depuis %q : %w", start, err)
	}
	for dir := abs; ; {
		candidate := filepath.Join(dir, DirName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && info.IsDir():
			return &Workspace{Root: dir, Dir: candidate}, nil
		case err == nil, isNotExist(err):
			// Pas d'espace ici (absent, ou .watchoo n'est pas un répertoire) : on remonte.
		default:
			return nil, fmt.Errorf("recherche de l'espace : %w", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("%w (recherche depuis %s)", ErrNotFound, abs)
		}
		dir = parent
	}
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
		return nil, fmt.Errorf("répertoire courant inaccessible : %w", err)
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
		start, origin = flagDir, "option -C"
	case env != "":
		start, origin = env, "variable "+EnvDir
	}

	if origin != "" {
		if !filepath.IsAbs(start) {
			start = filepath.Join(cwd, start)
		}
		info, err := os.Stat(start)
		switch {
		case isNotExist(err):
			return nil, fmt.Errorf("%s : le répertoire %s n'existe pas", origin, start)
		case err != nil:
			return nil, fmt.Errorf("%s : %w", origin, err)
		case !info.IsDir():
			return nil, fmt.Errorf("%s : %s n'est pas un répertoire", origin, start)
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
//   - si root/.watchoo existe déjà (répertoire) : erreur enveloppant ErrAlreadyInitialized, sans rien
//     modifier ; si c'est un fichier ou autre chose : erreur claire, sans rien modifier ;
//   - cherche un espace parent avec Find(filepath.Dir(root)) et renseigne ParentWorkspace s'il en
//     trouve un (ErrNotFound n'est pas une erreur ici, une erreur d'accès non plus : seul
//     l'avertissement est perdu) ;
//   - crée root/.watchoo en DirMode (0700, forcé par chmod pour ne pas dépendre du umask) ;
//   - y écrit config.yaml (contenu ConfigTemplate(nom du répertoire racine)) et .gitignore
//     (GitignoreContent) en FileMode (0600, forcé par chmod), sans jamais écraser un fichier
//     existant (O_EXCL) ;
//   - ne crée PAS la base SQLite (voir store.CreateDBFile / store.Open) et ne vérifie pas gh : la CLI
//     vérifie gh avant d'appeler Init, puis crée et migre la base après ;
//   - en cas d'échec après la création de .watchoo/, supprime le répertoire qu'il vient de créer.
func Init(root string) (result *InitResult, err error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("création de l'espace %q : %w", root, err)
	}
	info, err := os.Stat(abs)
	switch {
	case isNotExist(err):
		return nil, fmt.Errorf("création de l'espace : le répertoire %s n'existe pas", abs)
	case err != nil:
		return nil, fmt.Errorf("création de l'espace : %w", err)
	case !info.IsDir():
		return nil, fmt.Errorf("création de l'espace : %s n'est pas un répertoire", abs)
	}

	dir := filepath.Join(abs, DirName)
	if _, err := os.Lstat(dir); err == nil {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return nil, fmt.Errorf("%s : %w", abs, ErrAlreadyInitialized)
		}
		return nil, fmt.Errorf("création de l'espace : %s existe déjà et n'est pas un répertoire, "+
			"il n'a pas été modifié", dir)
	} else if !isNotExist(err) {
		return nil, fmt.Errorf("création de l'espace : %w", err)
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
		return nil, fmt.Errorf("création de %s : %w", dir, err)
	}
	// À partir d'ici, le répertoire est le nôtre (Mkdir a réussi) : on le supprime en cas d'échec pour
	// ne pas laisser un espace à moitié créé.
	defer func() {
		if err != nil {
			if rmErr := os.RemoveAll(dir); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("nettoyage de %s : %w", dir, rmErr))
			}
		}
	}()
	if err := os.Chmod(dir, DirMode); err != nil {
		return nil, fmt.Errorf("droits de %s : %w", dir, err)
	}

	name := filepath.Base(abs)
	ws := &Workspace{Root: abs, Dir: dir, Config: DefaultConfig(name)}
	if err := createFile(ws.ConfigPath(), []byte(ConfigTemplate(name))); err != nil {
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
		return fmt.Errorf("création de %s : %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("écriture de %s : %w", path, cerr)
		}
	}()
	if err := f.Chmod(FileMode); err != nil {
		return fmt.Errorf("droits de %s : %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		return fmt.Errorf("écriture de %s : %w", path, err)
	}
	return nil
}
