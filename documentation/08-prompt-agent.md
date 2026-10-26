# 8. Prompt prêt à coller pour un agent de code

[← Sommaire](README.md)

À coller tel quel dans Claude Code (ou un autre agent), en le faisant avancer phase par phase
(voir [Roadmap](07-roadmap.md)).

````markdown
Tu es un développeur Go senior. Construis "Watchoo", un outil de supervision de dépôts GitHub.

## Objectif
Un binaire Go unique offrant une CLI et une interface web locale pour suivre une liste de dépôts GitHub :
nouveaux commits, auteurs/contributeurs, date de dernière activité, évolution dans le temps.

## Contraintes techniques
- Go, dernière version stable (fixée dans go.mod), modules, code dans internal/, point d'entrée cmd/watchoo.
- CLI : spf13/cobra + spf13/viper (config YAML + variables d'env).
- Authentification : la CLI GitHub `gh` est un prérequis obligatoire. Le token est obtenu uniquement avec
  `gh auth token`, redemandé à chaque synchro, jamais stocké (ni config, ni base, ni logs). init vérifie
  `gh auth status`. Dépôts majoritairement privés : aucune route GitHub en écriture n'est appelée.
- GitHub : google/go-github + oauth2. Pagination, ETag/If-None-Match, respect du rate limit
  (attendre le reset si le quota est épuisé).
- Stockage : SQLite via modernc.org/sqlite (pas de CGO), migrations avec pressly/goose,
  requêtes typées générées avec sqlc (schéma lu depuis les migrations goose, filtres optionnels via sqlc.narg).
- Web : net/http + routeur go-chi/chi (groupes de routes, middlewares), html/template, HTMX, DaisyUI (Tailwind) avec CSS généré par la CLI autonome
  Tailwind (pas de Node), assets embarqués avec embed. Écoute 127.0.0.1 par défaut.
- Logs : log/slog. Concurrence bornée avec errgroup (max 4 dépôts synchronisés en parallèle).
- Abstraction Provider (interface) pour ajouter GitLab/Gitea plus tard.

## Modèle de données
repositories, branches, contributors, contributor_emails, commits, commit_branches, sync_runs
(voir documentation/04-modele-donnees.md).
commits : clé (repo_id, sha), colonnes author_id, committer_id, message, authored_at, committed_at,
additions, deletions, url — un commit n'est stocké qu'une fois par dépôt.
commit_branches(repo_id, sha, branch_id) : sur quelles branches suivies chaque commit a été vu.
Index sur commits(repo_id, committed_at DESC), commits(author_id, committed_at DESC) et
commit_branches(branch_id).

## Espaces de supervision
Un répertoire = un groupe supervisé (ex. les projets des étudiants de licence). `watchoo init` crée
.watchoo/ (config.yaml, watchoo.db, .gitignore ; droits 0700/0600) dans le répertoire courant et refuse
s'il existe déjà. Les autres commandes cherchent .watchoo/ dans le répertoire courant puis les parents,
comme Git ; -C <répertoire> ou WATCHOO_DIR pour en désigner un autre. Une base SQLite par espace.
Voir documentation/10-espaces-de-supervision.md.

## Commandes CLI
init, repo import <fichier.csv> (colonnes repo, label, branches ; vérifie l'accès, bilan, idempotent, --dry-run),
repo add (toutes les branches par défaut, --branch pour restreindre par liste ou motif)|list|remove, sync, log (--repo --author --since -n), contributors, status,
serve (--addr --interval : 30 min par défaut, 15 min minimum, refus en dessous). Option --json globale.
Commande dédiée `tui` : mode interactif avec Bubble Tea (bubbletea + bubbles + lipgloss), construit en
phase 3. Les autres commandes ne sont jamais interactives : tableau texte (lipgloss/table) ou JSON.
Respect de NO_COLOR.

## Interface web
Tableau de bord (carte par dépôt + fil des derniers commits), détail d'un dépôt (historique,
filtres, graphique 90 jours), page contributeurs, page réglages (ajout de dépôt, synchro manuelle, quota API).

## Skills
Les skills du projet sont installés depuis skills.sh (npx skills add), dans .agents/skills/ avec des liens
dans .claude/skills/ et .vibe/skills/, verrouillés par skills-lock.json. Utilise goland pour le Go,
semver et semver-changelog pour les versions et le CHANGELOG. N'installe aucun skill sans accord.
Voir documentation/11-skills.md.

## Méthode de travail
- Avance par phases : 1) fondations, 2) provider GitHub, 3) synchro + CLI + TUI, 4) web, 5) finitions.
- À chaque phase : code, tests (httptest pour simuler l'API GitHub), go vet, puis résumé de ce qui marche.
- La CI n'utilise ni vrai gh ni vrai GitHub : gh est mocké (interface Go + faux binaire compilé depuis
  internal/auth/testdata/fakegh, chemin réglable via github.gh_path). Voir documentation/09-tests.md.
- Synchro incrémentale : ne jamais re-télécharger un commit déjà en base.
- Ne jamais écrire le token en clair dans les logs.

Commence par la phase 1 et propose l'arborescence avant d'écrire le code.
````

> Conseil : pointer aussi l'agent vers ce dossier `documentation/`, qui fait référence en cas de doute.
