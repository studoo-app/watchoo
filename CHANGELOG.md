# Journal des modifications

Toutes les modifications notables de Watchoo sont consignées dans ce fichier.

Le format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/), et le projet respecte le
[versionnage sémantique](https://semver.org/lang/fr/spec/v2.0.0.html). Tant que Watchoo est en `0.y.z`,
un changement incompatible fait monter MINOR et tout changement compatible fait monter PATCH (voir
[Skills](documentation/11-skills.md#versions-et-changelogmd)).

## [Unreleased]

## [0.4.0] - 2026-10-27

V2 : dépôts Git locaux sans `gh`, pull requests, releases et tags, seuils d'activité par dépôt et
activité par contributeur dans un dépôt. Fin du MVP.

### Ajouté

- **Dépôts Git locaux** : `gh` devient facultatif. Sans `gh` installé et connecté, Watchoo suit les
  dépôts par clones Git **avec copie de travail** dans `repos/<name>`, à la racine de l'espace (homonymes :
  `repos/<owner>-<name>`), mis à jour à chaque synchro par `git fetch` puis remis sur la branche par
  défaut (une modification locale d'un fichier suivi est écrasée, un fichier non suivi gardé ; ni hook,
  ni sous-module, ni téléchargement LFS), avec la clé SSH ou les identifiants Git de la machine, sans aucune invite (`git` doit être
  installé) : commits, branches, statistiques et tags, sans quota, mais sans pull requests, releases,
  étoiles ni login GitHub (contributeurs fusionnés par e-mail). La source est choisie à chaque synchro,
  seulement selon `gh` : le connecter (`gh auth login`) fait repasser par l'API, qui complète alors
  l'identifiant et les métadonnées des dépôts ajoutés sans `gh`. Nouveau réglage `git.protocol`
  (`ssh` par défaut, ou `https`), propre à la machine et jamais partagé. `watchoo init` sans `gh` crée
  l'espace et le signale au lieu de s'arrêter (il s'arrête si `git` manque aussi) ; `repo add` et
  `repo import` vérifient l'accès par `git ls-remote` (« accès refusé » sinon) ; `repo remove` supprime
  le clone, `--keep-history` le garde. Lancée depuis un clone, une commande ignore un `.watchoo/` commité
  par l'étudiant et travaille sur l'espace qui contient `repos/`. `watchoo status`, le bilan de `watchoo sync` et la page Réglages
  (quota « sans objet ») indiquent la source ; les sorties `--json` de `init`, `sync` et `status`
  gagnent la clé `source`. La page d'un dépôt et le détail du TUI masquent étoiles, issues, pull requests
  et releases d'un dépôt jamais lu par l'API. La base rend `repositories.provider_id` facultatif
  (migration `00004`, appliquée automatiquement, sans perte de données). L'image Docker contient
  désormais `git` et `ssh` : elle passe d'une base distroless à un système Alpine minimal installé pour
  l'architecture cible (toujours sans émulation, utilisateur non root).
- **Pull requests et releases** : chaque synchro lit aussi les pull requests (de façon incrémentale :
  seulement celles modifiées depuis la synchro précédente), les releases et les tags de chaque dépôt ;
  quand rien n'a changé, GitHub répond `304` et ces lectures ne coûtent rien. Nouvelles commandes
  `watchoo prs [--repo X] [--state open|merged|closed|all] [-n 20]` et `watchoo releases [--repo X]
  [-n 20]` (releases puis tags, avec la date du commit visé), en texte et en `--json` (clés
  `pull_requests`, `releases` et `tags`). La page d'un dépôt de l'interface web gagne les cartes « Pull
  requests » (ouvertes, dernières mergées, liens vers GitHub) et « Releases et tags » ; le tableau de
  bord indique les PR ouvertes de chaque dépôt, et le détail d'un dépôt du TUI ses PR ouvertes et sa
  dernière release. Un échec sur ces listes ne fait pas échouer la synchro des commits : il devient un
  avertissement (colonne DÉTAIL de `watchoo sync`, clé `warnings` de chaque dépôt en `--json`, bilan du
  TUI). La base passe à 10 tables (migration `00003`, appliquée automatiquement).

- **Seuils d'activité par dépôt** : un dépôt peut avoir ses propres seuils Flemme et Dead à la place de
  ceux de l'espace (`activity.*`). `watchoo repo add owner/repo --flemme 3 --dead 10` (`0` revient au
  seuil de l'espace), colonnes facultatives `flemme` et `dead` du fichier de `watchoo repo import`
  (cellule vide = seuil gardé), clés `flemme_after_days` et `dead_after_days` des dépôts dans le
  partage de la configuration (écrites seulement pour un dépôt qui en a), et formulaire « Seuils
  d'activité de ce dépôt » sur la page d'un dépôt de l'interface web (champ vide = seuil de l'espace).
  L'en-tête de la page et le détail du TUI disent si les seuils appliqués sont propres au dépôt ou ceux
  de l'espace. Les sorties `--json` gagnent les clés `flemme_after_days` et `dead_after_days` (`null` =
  seuil de l'espace) dans l'objet dépôt et dans l'aperçu de `config import`.
- **Activité par contributeur dans un dépôt** : la page d'un dépôt de l'interface web montre les
  commits par semaine sur 12 semaines de ses 8 contributeurs les plus actifs (barres empilées, les
  autres regroupés, table des données repliable), comme la page des contributeurs mais pour ce seul
  dépôt (toutes les branches suivies, chaque commit une fois). Le détail d'un dépôt du TUI donne, pour
  ses principaux contributeurs, leurs commits sur 12 semaines et une courbe semaine par semaine.
  `watchoo contributors --weeks N` (avec ou sans `--repo`) limite le classement aux N dernières semaines
  et affiche les commits de chacun semaine par semaine ; la sortie `--json` gagne la clé `weekly`
  (objets `week` / `count`, `[]` sans `--weeks`) dans chaque contributeur.

## [0.2.0] - 2026-10-27

Partage de la configuration entre superviseurs, script d'installation et nouveau logo.

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
- **Nouveau logo** : hibou pixelisé aux yeux magenta et pupilles vertes, avec le nom « WATCHOO »
  (`assets/logo/watchoo-logo-final*.svg`, variantes pour fond sombre). L'en-tête de l'interface web
  affiche le hibou et le nom, en version claire ou sombre selon le thème ; favicon, icônes du manifeste
  et de l'écran d'accueil iOS refaits sur fond blanc ; README mis à jour. Les versions texte du hibou
  (CLI, TUI, `config.yaml`) ne changent pas.

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

[Unreleased]: https://github.com/studoo-app/watchoo/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/studoo-app/watchoo/compare/v0.2.0...v0.4.0
[0.2.0]: https://github.com/studoo-app/watchoo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/studoo-app/watchoo/releases/tag/v0.1.0
