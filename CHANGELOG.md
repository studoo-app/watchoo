# Journal des modifications

Toutes les modifications notables de Watchoo sont consignées dans ce fichier.

Le format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/), et le projet respecte le
[versionnage sémantique](https://semver.org/lang/fr/spec/v2.0.0.html). Tant que Watchoo est en `0.y.z`,
un changement incompatible fait monter MINOR et tout changement compatible fait monter PATCH (voir
[Skills](documentation/11-skills.md#versions-et-changelogmd)).

## [Unreleased]

### Ajouté

- **Script d'installation** `install.sh` (Linux, macOS) : `curl -fsSL
  https://raw.githubusercontent.com/studoo-app/watchoo/main/install.sh | sh` télécharge la dernière
  release, vérifie sa somme SHA-256 et installe `watchoo` dans `~/.local/bin`, sans sudo
  (`WATCHOO_VERSION`, `WATCHOO_INSTALL_DIR`).
- **Partage de la configuration** entre superviseurs d'un même groupe : `watchoo config export [-o
  fichier] [--force]` écrit un fichier JSON versionné (`format: watchoo-config`, `version: 1`) avec les
  dépôts suivis (nom affiché, branches) et les réglages communs (`sync.*`, `activity.*`,
  `branches.exclude`, `timezone`), jamais `github.*`, `web.*`, token, e-mail ni commit ;
  `watchoo config import fichier [--dry-run] [--yes]` le valide entièrement, affiche un aperçu, puis
  fusionne sans rien retirer (dépôts ajoutés ou mis à jour comme avec `repo import`, `config.yaml`
  réécrit depuis le modèle commenté avec copie `config.yaml.bak`). Même fonction sur la page Réglages
  de l'interface web (export, aperçu, application).
- **README** : section « Mise à jour » (script, binaires, Docker, sources ; migrations automatiques,
  sauvegarde de la base avant de changer de version).

### Modifié

- Interface web : un formulaire dont le corps dépasse la limite reçoit maintenant `413` (corps trop
  gros) au lieu d'un `403` trompeur ; rien n'est appliqué dans les deux cas.

## [0.1.0] - 2026-10-27

Première version publiée : le MVP (phases 1 à 5 de la [roadmap](documentation/07-roadmap.md)).

### Ajouté

- **Espaces de supervision** : `watchoo init` transforme un répertoire en espace indépendant
  (`.watchoo/` en `0700`, config et base SQLite en `0600`, `.gitignore`) après avoir vérifié `gh` ;
  recherche de l'espace dans les répertoires parents, option `-C` et variable `WATCHOO_DIR` ; réglages
  en trois niveaux (utilisateur, espace, variables `WATCHOO_<SECTION>_<CLÉ>`) ; `config.example.yaml`.
- **Base SQLite** sans CGO (`modernc.org/sqlite`), migrations goose embarquées, requêtes sqlc, dates
  stockées en UTC au format fixe.
- **Accès à GitHub** par la CLI `gh` uniquement (token de `gh auth token`, compte choisi par
  `github.user`, cas `GH_TOKEN` pour Docker et la CI), et provider GitHub (go-github) : pagination,
  arrêt au premier commit connu, comparaisons pour l'incrémental, cache ETag, suivi du quota, dépôts
  vides, renommés ou transférés, SSO.
- **Moteur de synchronisation incrémentale** : toutes les branches suivies (motifs `--branch`, branches
  exclues), aucun commit re-téléchargé, statistiques bornées, verrou par synchro, garde-fou de quota,
  au plus 4 dépôts en parallèle.
- **Commandes** `repo add`, `repo import` (fichier CSV ou liste, vérification d'accès, `--dry-run`,
  bilan), `repo list`, `repo remove`, `sync`, `log`, `contributors`, `status` et `version`.
- **États d'activité** de chaque dépôt : Actif, Flemme (plus de 5 jours sans commit), Dead (plus de
  15 jours), seuils réglables par espace (pas encore par dépôt), et états gris (jamais synchronisé, en
  erreur, archivé, suivi arrêté, vide).
- **Mode interactif** `watchoo tui` (Bubble Tea) : dépôts, commits, contributeurs, filtre et synchro au
  clavier.
- **Interface web locale** `watchoo serve` (HTMX, DaisyUI, Chart.js embarqués) : tableau de bord, détail
  d'un dépôt (historique paginé, filtres par branche et par auteur, activité sur 90 jours),
  contributeurs, réglages (ajout et retrait de dépôts, nom affiché, intervalle, quota) ; synchro
  périodique en arrière-plan et synchro à la demande avec progression.
- **Graphiques d'activité** : commits par jour sur 90 jours de tout l'espace sur le tableau de bord, et
  commits par semaine sur 12 semaines des 8 contributeurs les plus actifs sur la page contributeurs,
  calculés dans le fuseau d'affichage, lisibles en thème clair et sombre.
- **Sortie `--json`** sur toutes les commandes qui produisent des données, avec un schéma stable
  documenté dans [05-cli.md](documentation/05-cli.md#sortie-json) et vérifié par les tests.
- `watchoo version` et `watchoo version --json` donnent la version, le commit et sa date, la version de
  Go, le système et l'architecture.
- **Carte d'en-tête** avec le hibou dans le terminal (seulement sur un terminal, jamais avec `--json`),
  logo et icônes de l'interface web.
- **Distribution** : binaires linux, macOS et Windows en amd64 et arm64 (GoReleaser), archives avec
  sommes SHA-256 ; image Docker `ghcr.io/studoo-app/watchoo` (linux/amd64 et linux/arm64) avec `gh` ;
  exemple `docker-compose.yml` ; cibles `make release-check`, `make release-snapshot` et `make docker`.
- README complet, documentation de conception dans `documentation/`, licence Apache 2.0.

### Sécurité

- Le token GitHub n'est jamais lu ailleurs que dans `gh`, jamais stocké, et n'apparaît dans aucun
  journal, erreur ni sortie.
- Lecture seule : le transport du provider refuse tout appel GitHub autre que `GET` ou `HEAD` avant
  l'envoi, et n'envoie le token qu'à l'origine de l'API.
- Seuls les espaces de l'utilisateur sont acceptés (Linux, macOS) : un `.watchoo/` qui ne lui
  appartient pas ou qui dépasse les droits `0700` (`config.yaml` : `0600`), comme celui qu'un étudiant
  aurait commité dans son dépôt cloné, est refusé avant toute lecture de ses réglages ;
  `github.gh_path` doit être un nom cherché dans le `PATH` ou un chemin absolu.
- Ni message de commit ni e-mail dans les journaux ; textes venus des dépôts nettoyés des séquences
  d'échappement du terminal et échappés dans le web.
- Interface web : écoute sur `127.0.0.1` par défaut, authentification obligatoire sur toute autre
  adresse (authentification basique avec `WATCHOO_WEB_PASSWORD`, ou reverse proxy de confiance avec
  `web.allowed_hosts`), protection CSRF (cookie posé seulement avec les pages HTML), contrôle de l'en-tête
  `Host` contre le DNS rebinding, CSP stricte sans script en ligne, en-têtes de sécurité.
- Image Docker sur une base distroless, utilisateur non root (uid 65532), sans token dans l'image ;
  `gh` est le binaire officiel vérifié par SHA-256 ; images de base et actions GitHub épinglées par
  empreinte.

[Unreleased]: https://github.com/studoo-app/watchoo/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/studoo-app/watchoo/releases/tag/v0.1.0
