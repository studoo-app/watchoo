package git

// Langages d'un dépôt (1.1.0, documentation/fr/03-architecture.md, « Langages d'un dépôt ») : taille en
// octets de chaque langage sur la branche par défaut, lue dans le clone, sans requête ni quota. La
// détection se fait par nom de fichier et extension, sans dépendance : les chiffres sont proches de
// ceux de GitHub, sans être identiques.

import (
	"bytes"
	"cmp"
	"context"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// LanguageRulesVersion est la version des règles de langages (table, exclusions, commits de fusion) avec
// laquelle CommitStatsBatch lit les langages des commits (1.2.0, commits.languages_version) : toute
// modification de ces règles l'augmente, ce qui fait relire les langages de tous les commits à la
// synchro suivante.
const LanguageRulesVersion = 1

// languageExcludedDirs sont les répertoires ignorés où qu'ils soient : dépendances et fichiers générés,
// souvent commités par erreur.
var languageExcludedDirs = map[string]bool{
	"node_modules": true, "vendor": true, "bower_components": true, "dist": true, "build": true,
	"target": true, "__pycache__": true, ".venv": true, "venv": true, ".idea": true, ".vscode": true,
	".gradle": true, ".next": true, ".nuxt": true,
}

// languageExcludedFiles sont les fichiers de verrouillage ignorés (noms en minuscules).
var languageExcludedFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "composer.lock": true,
	"go.sum": true, "cargo.lock": true, "gemfile.lock": true, "poetry.lock": true,
}

// languageByName donne le langage d'un fichier d'après son nom (en minuscules).
var languageByName = map[string]string{
	"dockerfile": "Dockerfile", "containerfile": "Dockerfile", "makefile": "Makefile",
	"gnumakefile": "Makefile", "cmakelists.txt": "CMake", "jenkinsfile": "Groovy", "rakefile": "Ruby",
	"gemfile": "Ruby", "vagrantfile": "Ruby",
}

// languageByExt donne le langage d'un fichier d'après son extension (en minuscules). Comme GitHub, les
// données et la prose (JSON, YAML, Markdown…) ne comptent pas ; le SQL, si.
var languageByExt = map[string]string{
	".asm": "Assembly", ".s": "Assembly", ".astro": "Astro", ".bat": "Batchfile", ".cmd": "Batchfile",
	".c": "C", ".h": "C", ".cs": "C#", ".cpp": "C++", ".cc": "C++", ".cxx": "C++", ".hpp": "C++",
	".hh": "C++", ".clj": "Clojure", ".cljs": "Clojure", ".cmake": "CMake", ".css": "CSS", ".dart": "Dart",
	".ex": "Elixir", ".exs": "Elixir", ".ejs": "EJS", ".erl": "Erlang", ".fs": "F#", ".fsx": "F#",
	".gd": "GDScript", ".go": "Go", ".groovy": "Groovy", ".gradle": "Groovy", ".hbs": "Handlebars",
	".hs": "Haskell", ".tf": "HCL", ".html": "HTML", ".htm": "HTML", ".java": "Java",
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".jl": "Julia", ".ipynb": "Jupyter Notebook", ".kt": "Kotlin", ".kts": "Kotlin", ".less": "Less",
	".liquid": "Liquid", ".lua": "Lua", ".nix": "Nix", ".m": "Objective-C", ".mm": "Objective-C++",
	".ml": "OCaml", ".pl": "Perl", ".pm": "Perl", ".php": "PHP", ".ps1": "PowerShell", ".pug": "Pug",
	".py": "Python", ".r": "R", ".rb": "Ruby", ".rs": "Rust", ".sass": "Sass", ".scala": "Scala",
	".scss": "SCSS", ".sh": "Shell", ".bash": "Shell", ".zsh": "Shell", ".sol": "Solidity", ".sql": "SQL",
	".svelte": "Svelte", ".swift": "Swift", ".tex": "TeX", ".twig": "Twig", ".ts": "TypeScript",
	".tsx": "TypeScript", ".vb": "Visual Basic", ".vue": "Vue", ".zig": "Zig",
}

// Languages renvoie les langages de la branche par défaut du clone de owner/name, du plus gros au plus
// petit (puis par nom). Le clone est d'abord créé ou mis à jour (voir Prepare). Dépôt vide : liste vide
// non nil. Une seule commande, sans lire le contenu des fichiers :
//
//	git ls-tree -r -l -z <sha>
func (p *Provider) Languages(ctx context.Context, owner, name string) ([]provider.Language, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	clone, err := p.ensure(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	op := i18n.T("provider.git.op.languages", owner, name)
	branch, err := p.defaultBranch(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	if branch == "" || provider.ValidateBranch(branch) != nil {
		return []provider.Language{}, nil
	}
	sha, found, err := p.resolve(ctx, op, clone, remotePrefix+branch)
	if err != nil {
		return nil, err
	}
	if !found {
		return []provider.Language{}, nil
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(clone), "ls-tree", "ls-tree", "-r", "-l", "-z", sha)
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return nil, localError(op, res)
	}
	return parseLanguages(res.stdout), nil
}

// parseLanguages lit la sortie de « ls-tree -r -l -z » : un enregistrement
// « mode type sha taille<TAB>chemin<NUL> » par entrée. Seuls les fichiers ordinaires comptent : ni lien
// symbolique (mode 120000), ni sous-module (type commit).
func parseLanguages(raw []byte) []provider.Language {
	sizes := make(map[string]int64)
	for rec := range bytes.SplitSeq(raw, []byte{0}) {
		meta, file, ok := strings.Cut(string(rec), "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 4 || fields[1] != "blob" || fields[0] == "120000" {
			continue
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil || size <= 0 {
			continue
		}
		if lang := classifyLanguage(file); lang != "" {
			sizes[lang] += size
		}
	}
	langs := make([]provider.Language, 0, len(sizes))
	for lang, size := range sizes {
		langs = append(langs, provider.Language{Name: lang, Bytes: size})
	}
	slices.SortFunc(langs, func(a, b provider.Language) int {
		return cmp.Or(cmp.Compare(b.Bytes, a.Bytes), cmp.Compare(a.Name, b.Name))
	})
	return langs
}

// classifyLanguage renvoie le langage du fichier file (chemin dans le dépôt, séparé par « / ») ; vide
// s'il est exclu ou inconnu.
func classifyLanguage(file string) string {
	dir, base := path.Split(file)
	for part := range strings.SplitSeq(strings.TrimSuffix(dir, "/"), "/") {
		if languageExcludedDirs[part] {
			return ""
		}
	}
	lower := strings.ToLower(base)
	if languageExcludedFiles[lower] || strings.HasSuffix(lower, ".min.js") || strings.HasSuffix(lower, ".min.css") {
		return ""
	}
	if lang, ok := languageByName[lower]; ok {
		return lang
	}
	if strings.HasPrefix(lower, "dockerfile.") {
		return "Dockerfile"
	}
	return languageByExt[path.Ext(lower)]
}
