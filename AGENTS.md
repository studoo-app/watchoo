# AGENTS.md

Instructions pour les agents de code qui travaillent sur Watchoo (format [agents.md](https://agents.md/)).

## Projet

Watchoo est un binaire Go unique qui surveille des dépôts GitHub, **majoritairement privés**, et répond à
trois questions : qu'est-ce qui a bougé, quand, et qui l'a fait. Il propose une CLI, un mode interactif
(`watchoo tui`) et une interface web locale (`watchoo serve`). Chaque groupe supervisé (ex. les projets des
étudiants d'une promotion) vit dans son propre répertoire, initialisé par `watchoo init`.

**État actuel : v0.4.0 (MVP terminé, phases 1 à 6 de la [roadmap](documentation/07-roadmap.md) en place : MVP, partage de la configuration et V2).**
`watchoo init` crée un espace de supervision ; `repo add / import / list / remove`, `sync`, `log`,
`contributors`, `prs`, `releases`, `status` et `version` fonctionnent en texte et en `--json` (schéma stable, documenté dans
[05-cli.md](documentation/05-cli.md#sortie-json) et vérifié par `internal/cli/json_schema_test.go`) ;
`watchoo tui` ouvre le mode interactif ; `watchoo serve` sert l'interface web locale (tableau de bord et
contributeurs avec graphiques d'activité, détail d'un dépôt, réglages) et synchronise en arrière-plan. Le
cœur métier est dans `internal/sync` (moteur : synchro incrémentale, verrou, quota, ajout de dépôts ;
`Coordinator` : une seule synchro à la fois dans un processus long), `internal/activity` (états Actif /
Flemme / Dead et gris, séries d'activité par jour et par semaine) et `internal/repoimport` (références,
motifs `--branch`, fichiers d'import) ; `internal/cli`, `internal/tui` et `internal/web` ne font
qu'afficher. Deux tests de bout en bout enchaînent les commandes (`internal/cli/e2e_test.go`) et les
pages web (`internal/web/e2e_test.go`) avec le faux `gh` et `githubtest`. Distribution : GoReleaser
(`.goreleaser.yaml`), image Docker (`Dockerfile`, `docker-compose.yml`), workflow de release sur tag
(`.github/workflows/release.yml`) ; historique dans [CHANGELOG.md](CHANGELOG.md). V2 : seuils d'activité
par dépôt, activité par contributeur dans un dépôt, pull requests / releases / tags, et dépôts Git locaux
(`internal/provider/git` : clones nus dans `.watchoo/repos/`, utilisés quand `gh` est absent ou non
connecté). Le critère de fin du MVP (usage réel d'une semaine) est validé : le MVP est terminé.

## Documentation de référence

`documentation/` est la **source de vérité**. La lire avant de coder ; en cas de doute ou de
contradiction, elle l'emporte sur toute autre source, skills compris.

| Sujet | Fichier |
| --- | --- |
| Vision, périmètre | [01-vision.md](documentation/01-vision.md) |
| Fonctionnalités et règles (états Actif / Flemme / Dead…) | [02-fonctionnalites.md](documentation/02-fonctionnalites.md) |
| Architecture, stack, décisions techniques | [03-architecture.md](documentation/03-architecture.md) |
| Modèle de données (10 tables) | [04-modele-donnees.md](documentation/04-modele-donnees.md) |
| CLI, TUI, import groupé | [05-cli.md](documentation/05-cli.md) |
| Interface web | [06-interface-web.md](documentation/06-interface-web.md) |
| Roadmap par phases | [07-roadmap.md](documentation/07-roadmap.md) |
| Tests | [09-tests.md](documentation/09-tests.md) |
| Espaces de supervision (`watchoo init`) | [10-espaces-de-supervision.md](documentation/10-espaces-de-supervision.md) |
| Skills | [11-skills.md](documentation/11-skills.md) |

Une décision qui change ce qui est documenté se fait **d'abord dans la documentation**, avec l'accord du
mainteneur, puis dans le code.

## Méthode de travail

- Avancer **phase par phase** ; ne passer à la suivante que lorsque la précédente marche de bout en bout.
- Avant d'écrire le code d'une phase, proposer l'arborescence et le plan.
- À la fin de chaque phase : tests verts, `go vet` propre, résumé de ce qui marche.
- Ne jamais ajouter une dépendance hors de la stack documentée sans accord.

## Stack (résumé)

Go (dernière version stable, fixée dans `go.mod`) · cobra + viper · go-github (sans x/oauth2) · SQLite via
`modernc.org/sqlite` (sans CGO) · migrations goose · requêtes sqlc · net/http + go-chi/chi ·
html/template + HTMX + DaisyUI (CSS généré par la CLI autonome Tailwind) · Chart.js embarqué ·
Bubble Tea + bubbles + lipgloss · log/slog · errgroup · binaire `git` (dépôts Git locaux, lancé par
`os/exec`). Détail et justifications :
[03-architecture.md](documentation/03-architecture.md).

## Organisation du code (cible)

```
cmd/watchoo/          point d'entrée
internal/cli/         commandes cobra (affichage seulement)
internal/tui/         mode interactif Bubble Tea (affichage seulement)
internal/web/         handlers chi, templates, static (embed), tailwind/ (CSS DaisyUI, VERSIONS)
internal/workspace/   espaces de supervision : init, recherche de .watchoo/, réglages
internal/auth/        accès à gh (token, état de connexion)
internal/provider/    interface Provider, types, erreurs ; providertest/ (faux Provider)
                      github/ : implémentation GitHub ; github/githubtest/ (faux serveur GitHub)
                      git/ : dépôts Git locaux (binaire git) ; git/gittest/ (dépôts de test en file://)
internal/sync/        moteur de synchronisation (cœur métier)
internal/activity/    règles d'activité (Actif / Flemme / Dead, états gris), séries, formats courts
internal/repoimport/  références de dépôts, motifs --branch, fichiers d'import
internal/configshare/ partage de la configuration (export JSON, validation, aperçu, import)
internal/textclean/   nettoyage des textes affichés (séquences d'échappement), CLI et TUI
internal/store/       SQLite : migrations/, queries/ (sqlc), db/ (généré)
internal/model/       structs métier
internal/logo/        hibou en versions texte (copie de assets/logo/) et carte d'en-tête
```

## Commandes

```bash
make build        # bin/watchoo, sans CGO, version injectée par -ldflags
make test         # go test -race ./...
make vet          # go vet ./... (et le faux gh des tests, ignoré par ./...)
make fmt-check    # échoue si gofmt -l trouve un fichier
make sqlc         # sqlc generate
make sqlc-check   # échoue si internal/store/db n'est pas à jour
make check        # tous les contrôles, comme la CI (.github/workflows/ci.yml)
make css          # CSS DaisyUI généré par la CLI autonome Tailwind (make css-check : à jour ?)
make logo         # recopie assets/logo dans internal/logo/art/ et internal/web/static/ (copies embarquées)
make release-check    # goreleaser check (.goreleaser.yaml)
make release-snapshot # goreleaser release --snapshot --clean : release locale dans dist/, rien de publié
make docker           # image Docker locale (IMAGE=watchoo:dev), jamais poussée
```

- `sqlc generate` après toute modification de `internal/store/queries/` ou des migrations ; le code
  généré (`internal/store/db/`) est commité et **jamais modifié à la main**.
- `make css` après toute modification des templates ; `internal/web/static/app.css` est commité.
- `make logo` après toute modification de `assets/logo/` ; des tests vérifient que les copies sont à jour.
- Les `-ldflags` (`internal/cli.Version`, `Commit`, `Date`) sont écrits dans le `Makefile`, le
  `.goreleaser.yaml` et le `Dockerfile` : les garder identiques.
- Toute modification visible va dans la section `[Unreleased]` de `CHANGELOG.md` (skills `semver` et
  `semver-changelog`, voir [11-skills.md](documentation/11-skills.md)). Aucun tag, release ni
  `docker push` sans demande explicite.

## Règles d'architecture

- **Aucune logique métier** dans `cli`, `tui` ou `web` : ces couches appellent le cœur métier.
- Tout accès à GitHub passe par l'interface `Provider` ; rien de spécifique à GitHub hors de
  `internal/provider/github/` (seul package qui importe go-github) et de `internal/provider/git/` (URL de
  clone et de page des dépôts github.com).
- Les types générés par sqlc ne sortent pas de `internal/store/` ; ils sont convertis vers `internal/model`.
- Les handlers web restent des `http.Handler` standard.
- Synchro **incrémentale** : ne jamais re-télécharger un commit déjà en base ; utiliser les ETag ;
  respecter le rate limit ; au plus 4 dépôts synchronisés en parallèle.
- Dates stockées en texte ISO 8601 UTC, format fixe `2006-01-02T15:04:05Z` ; conversion en heure locale
  à l'affichage seulement.
- Une base SQLite par espace de supervision (`.watchoo/watchoo.db`).

## Sécurité (non négociable)

- `gh` est facultatif : sans `gh` installé et connecté, les dépôts sont suivis par clone local. Quand
  l'API est utilisée, le token GitHub vient **uniquement** de `gh auth token`. Ne jamais lire
  `GITHUB_TOKEN` ni stocker le token (config, base, fichier, trousseau).
- Dépôts Git locaux : Watchoo ne lit, ne demande ni ne stocke aucun identifiant (git utilise la clé SSH
  ou le gestionnaire d'identifiants de l'utilisateur, sans invite interactive).
- Le token n'apparaît **jamais** dans les logs, les erreurs, la sortie CLI, le TUI ou le web.
- **Lecture seule** : aucun appel GitHub en `POST`, `PATCH`, `PUT` ou `DELETE` (le transport du provider
  les refuse avant l'envoi) ; en mode Git local, seulement `clone`, `fetch` et des commandes de lecture,
  jamais `push`.
- Les données viennent de dépôts privés et contiennent des données personnelles d'étudiants :
  `.watchoo/` en `0700`, fichiers en `0600` ; **ni message de commit ni e-mail dans les logs**.
- Web : écoute sur `127.0.0.1` par défaut ; authentification obligatoire sur toute autre adresse.

## Tests

- Tout code nouveau ou modifié a ses tests.
- **Aucun service extérieur** dans les tests ni en CI : `gh` est mocké (interface Go + faux binaire dans
  `internal/auth/testdata/fakegh/`, chemin réglable par `github.gh_path`), l'API GitHub est simulée par le
  faux serveur `githubtest` (sur `httptest`), le provider par `providertest`, les dépôts Git locaux par
  `gittest` (vrais dépôts créés dans un répertoire temporaire, servis en `file://` ; le binaire `git` est
  requis), la base est une SQLite temporaire.
- Détail des scénarios : [09-tests.md](documentation/09-tests.md).

## Style

- Code formaté avec `gofmt` ; `go vet` sans avertissement.
- Identifiants Go en anglais, selon les conventions Go.
- Documentation, messages affichés à l'utilisateur (CLI, TUI, web) et messages de commit en **français**.
- Erreurs enveloppées avec du contexte (`fmt.Errorf("…: %w", err)`) ; logs structurés avec `log/slog`.

## Skills

Les skills du projet viennent de [skills.sh](https://www.skills.sh/), installés dans `.agents/skills/`
(liens dans `.claude/skills/` et `.vibe/skills/`), verrouillés par `skills-lock.json`.

- `goland` pour le développement Go ; `semver` et `semver-changelog` pour les versions et `CHANGELOG.md`.
- N'installer ni mettre à jour aucun skill sans accord. Voir [11-skills.md](documentation/11-skills.md).

## Git

- Messages de commit au format **Conventional Commits**, en français : `feat: …`, `fix: …`, `docs: …`,
  `test: …`, `chore: …`.
- Ne pas commiter ni pousser sans demande explicite.
- Ne jamais commiter de base `.watchoo/`, de token ni de données réelles d'étudiants (les tests utilisent
  des données factices).
