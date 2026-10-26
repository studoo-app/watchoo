# AGENTS.md

Instructions pour les agents de code qui travaillent sur Watchoo (format [agents.md](https://agents.md/)).

## Projet

Watchoo est un binaire Go unique qui surveille des dépôts GitHub, **majoritairement privés**, et répond à
trois questions : qu'est-ce qui a bougé, quand, et qui l'a fait. Il propose une CLI, un mode interactif
(`watchoo tui`) et une interface web locale (`watchoo serve`). Chaque groupe supervisé (ex. les projets des
étudiants d'une promotion) vit dans son propre répertoire, initialisé par `watchoo init`.

**État actuel : conception.** Aucun code n'est encore écrit. La prochaine étape est la phase 1 de la
[roadmap](documentation/07-roadmap.md).

## Documentation de référence

`documentation/` est la **source de vérité**. La lire avant de coder ; en cas de doute ou de
contradiction, elle l'emporte sur toute autre source, skills compris.

| Sujet | Fichier |
| --- | --- |
| Vision, périmètre | [01-vision.md](documentation/01-vision.md) |
| Fonctionnalités et règles (états Actif / Flemme / Dead…) | [02-fonctionnalites.md](documentation/02-fonctionnalites.md) |
| Architecture, stack, décisions techniques | [03-architecture.md](documentation/03-architecture.md) |
| Modèle de données (7 tables) | [04-modele-donnees.md](documentation/04-modele-donnees.md) |
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

Go (dernière version stable, fixée dans `go.mod`) · cobra + viper · go-github · SQLite via
`modernc.org/sqlite` (sans CGO) · migrations goose · requêtes sqlc · net/http + go-chi/chi ·
html/template + HTMX + DaisyUI (CSS généré par la CLI autonome Tailwind) · Chart.js embarqué ·
Bubble Tea + bubbles + lipgloss · log/slog · errgroup. Détail et justifications :
[03-architecture.md](documentation/03-architecture.md).

## Organisation du code (cible)

```
cmd/watchoo/          point d'entrée
internal/cli/         commandes cobra (affichage seulement)
internal/tui/         mode interactif Bubble Tea (affichage seulement)
internal/web/         handlers chi, templates, static (embed)
internal/workspace/   espaces de supervision : init, recherche de .watchoo/, réglages
internal/auth/        accès à gh (token, état de connexion)
internal/provider/    interface Provider + github/
internal/sync/        moteur de synchronisation (cœur métier)
internal/store/       SQLite : migrations/, queries/ (sqlc), db/ (généré)
internal/model/       structs métier
```

## Commandes (prévues, à créer en phase 1)

```bash
go build ./cmd/watchoo
go test -race ./...
go vet ./...
sqlc generate
make css
```

- `sqlc generate` après toute modification de `internal/store/queries/` ou des migrations ; le code
  généré (`internal/store/db/`) est commité et **jamais modifié à la main**.
- `make css` après toute modification des templates ; `internal/web/static/app.css` est commité.

## Règles d'architecture

- **Aucune logique métier** dans `cli`, `tui` ou `web` : ces couches appellent le cœur métier.
- Tout accès à GitHub passe par l'interface `Provider` ; rien de spécifique à GitHub hors de
  `internal/provider/github/`.
- Les types générés par sqlc ne sortent pas de `internal/store/` ; ils sont convertis vers `internal/model`.
- Les handlers web restent des `http.Handler` standard.
- Synchro **incrémentale** : ne jamais re-télécharger un commit déjà en base ; utiliser les ETag ;
  respecter le rate limit ; au plus 4 dépôts synchronisés en parallèle.
- Dates stockées en texte ISO 8601 UTC, format fixe `2006-01-02T15:04:05Z` ; conversion en heure locale
  à l'affichage seulement.
- Une base SQLite par espace de supervision (`.watchoo/watchoo.db`).

## Sécurité (non négociable)

- Le token GitHub vient **uniquement** de `gh auth token`. Ne jamais lire `GITHUB_TOKEN` ni stocker le
  token (config, base, fichier, trousseau).
- Le token n'apparaît **jamais** dans les logs, les erreurs, la sortie CLI, le TUI ou le web.
- **Lecture seule** : aucun appel GitHub en `POST`, `PATCH`, `PUT` ou `DELETE`.
- Les données viennent de dépôts privés et contiennent des données personnelles d'étudiants :
  `.watchoo/` en `0700`, fichiers en `0600` ; **ni message de commit ni e-mail dans les logs**.
- Web : écoute sur `127.0.0.1` par défaut ; authentification obligatoire sur toute autre adresse.

## Tests

- Tout code nouveau ou modifié a ses tests.
- **Aucun service extérieur** dans les tests ni en CI : `gh` est mocké (interface Go + faux binaire dans
  `internal/auth/testdata/fakegh/`, chemin réglable par `github.gh_path`), l'API GitHub est simulée avec
  `httptest`, la base est une SQLite temporaire.
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
