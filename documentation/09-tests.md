# 9. Stratégie de tests

[← Sommaire](README.md)

Principe : **la CI ne dépend d'aucun service extérieur.** Pas de vrai `gh`, pas d'appel à l'API GitHub,
pas de token. Tout ce qui sort du binaire est simulé.

## Vue d'ensemble

| Ce qui est testé | Comment | Où |
| --- | --- | --- |
| Commande `gh` (token, état de connexion) | **Mock de `gh`** : interface + faux binaire (voir ci-dessous) | `internal/auth/` |
| API GitHub | Faux serveur GitHub programmable (`githubtest`, sur `httptest`) | `internal/provider/github/` |
| Dépôts Git locaux (V2) | Vrais dépôts Git créés avec `git` dans un répertoire temporaire et servis par `file://` (`gittest`) ; tests sautés seulement si `git` est absent (la CI l'a) | `internal/provider/git/`, `internal/sync/`, `internal/cli/` |
| Espaces de supervision | Répertoires temporaires : `init`, refus si déjà initialisé, recherche dans les parents, `-C`, `WATCHOO_DIR`, espace imbriqué, droits `0700`/`0600` | `internal/workspace/` |
| Base SQLite et requêtes sqlc | Base SQLite temporaire par test, migrations goose appliquées | `internal/store/` |
| Import groupé | Fichiers CSV et texte de test : ligne mal formée, doublons, dépôt inaccessible (`404`), relance idempotente, `--dry-run` sans écriture | `internal/cli/` + `internal/sync/` |
| Moteur de synchro | Faux Provider en mémoire (`providertest`) + base temporaire | `internal/sync/` |
| CLI | Exécution des commandes cobra, vérification de la sortie texte et JSON | `internal/cli/` |
| Web | `httptest` sur le routeur chi, vérification des pages et fragments | `internal/web/` |
| TUI | Tests du modèle Bubble Tea : messages envoyés, état et rendu obtenus | `internal/tui/` |
| Bout en bout | La CLI complète avec le faux `gh`, le vrai provider contre `githubtest` et une base temporaire | `internal/cli/e2e_test.go` |

## Mock de la commande `gh`

Watchoo dépend de `gh` pour obtenir le token, et pour choisir entre l'API GitHub et les clones Git
locaux (voir
[Architecture](03-architecture.md#authentification--gh-imposé)). Pour que la CI n'ait besoin ni de `gh`
ni d'un compte GitHub, `gh` est simulé à deux niveaux.

### Niveau 1 — interface Go (tests unitaires)

L'accès à `gh` passe par une petite interface dans `internal/auth/`, par exemple :

- obtenir le token ;
- obtenir l'état de connexion (compte, droits).

L'implémentation réelle exécute `gh` (`os/exec`). Les tests des autres packages (synchro, CLI, web, TUI)
utilisent une **fausse implémentation en mémoire** qui renvoie la réponse voulue. Aucun processus n'est
lancé.

### Niveau 2 — faux binaire `gh` (tests de l'implémentation réelle)

Pour tester le code qui lance vraiment `gh` (arguments, lecture de la sortie, codes de retour,
erreurs), on utilise un **faux exécutable `gh`** :

- un petit programme Go dans `internal/auth/testdata/fakegh/`, compilé au début des tests ;
- le chemin de `gh` est réglable dans la config (`github.gh_path`, `gh` par défaut), ce qui permet aux
  tests de pointer vers le faux binaire sans toucher au `PATH` ;
- son comportement est choisi par une variable d'environnement propre aux tests (par exemple
  `FAKEGH_SCENARIO`).

Scénarios couverts :

| Scénario | Comportement du faux `gh` | Ce que Watchoo doit faire |
| --- | --- | --- |
| Connecté | `auth status` réussit, `auth token` renvoie un faux token | Utiliser le token |
| Non connecté | `auth status` échoue avec le message de `gh` | `init` et `status` indiquent `gh auth login` |
| `gh` absent | Chemin inexistant | Message clair : installer `gh` |
| Plusieurs comptes | `auth token --user X` renvoie un token différent | Respecter `github.user` ; refuser un `github.user` écrit avec une autre casse (`--user` distingue les majuscules) |
| `GH_TOKEN` seul (Docker, CI) | `auth status` ne connaît le compte que par `GH_TOKEN` ; `auth token` sans `--user` renvoie ce token, `auth token --user X` échoue | Utiliser le token sans `--user` ; refuser un `github.user` que `gh` ne connaît que par `GH_TOKEN` |
| Token refusé | `auth status` donne l'état `error` avec `HTTP 401: Bad credentials` | `init` et `status` indiquent `gh auth login` |
| GitHub injoignable | `auth status` donne l'état `error` (erreur réseau) ou `timeout` | Erreur claire, sans conseiller `gh auth login` |
| Token changé entre deux synchros | Deux appels renvoient deux tokens différents | Utiliser le nouveau token sans redémarrer |
| Sortie inattendue / délai dépassé | Sortie vide, ou `gh` qui ne répond pas | Erreur claire, délai maximum appliqué |

Règles :

- le faux token est une valeur factice (ex. `gho_fake_for_tests`) ;
- un test vérifie que ce faux token **n'apparaît jamais dans les logs** ni dans les sorties CLI, web ou
  TUI.

## API GitHub simulée

Deux faux, écrits en phase 2 et réutilisés par les phases suivantes :

| Faux | Package | Sert à tester |
| --- | --- | --- |
| Serveur GitHub programmable | `internal/provider/github/githubtest` | Le vrai provider GitHub (transport, cache ETag, quota, erreurs) ; en phase 3, le moteur de synchro de bout en bout |
| `Provider` en mémoire | `internal/provider/providertest` | Le moteur de synchro, la CLI, le web et le TUI, sans HTTP |

- **`githubtest`** : serveur `httptest` sur `127.0.0.1` qui simule les routes utilisées (dépôt, branches,
  commits, commit avec statistiques, compare, pull requests, releases, tags, `/rate_limit`) à partir d'un état préparé par le test :
  dépôts, graphe de commits (merges compris), branches, force-push, renommage ou transfert (`301`), accès
  refusé (`404`) ou SSO (`403`), dépôt vide (`409`). Il reproduit le format JSON de GitHub, la pagination
  (en-tête `Link`), les ETag et les `304` (sans quota), le quota (en-têtes `X-RateLimit-*`, épuisement,
  remise à zéro), les limites secondaires (`Retry-After`), et tient un journal des requêtes reçues (sans
  la valeur de `Authorization`). Une réponse peut aussi être imposée pour un chemin donné.
- **`providertest`** : implémente `provider.Provider` avec la même sémantique que le provider GitHub
  (validation, ordre des commits, `Stop`, `Since`, `Limit`, dépôt vide, `ErrUnknownBase`, renommages),
  plus l'injection d'erreurs, le comptage des appels et un quota simulé.
- `internal/provider/github/testdata/` : quelques réponses JSON factices au format exact de GitHub, pour
  les tests de conversion des types.
- **Scénarios** (le vrai provider contre `githubtest`, `internal/provider/github/scenario_test.go`) :
  pagination des branches et des commits ; synchro incrémentale (premier import par la liste des
  commits, nouvelle branche par compare depuis la tête de la branche par défaut, y compris une branche
  qui a fusionné `main`, compare entre l'ancienne et la nouvelle tête, y compris un merge qui apporte des
  commits plus anciens que l'ancienne tête, et la perte de commits que provoquerait l'arrêt au premier
  SHA connu) ; force-push (`ErrUnknownBase`, puis repli) ; réponses `304` avec ETag (quota non décompté, cache
  partagé entre deux providers du même token et vidé par un autre token) ; quota épuisé ; limite
  secondaire ; `404` sur un dépôt privé sans accès ; `403` lié au SSO ; `401` ; dépôt renommé ou
  transféré ; dépôt vide ; dépôt archivé ; 4 dépôts synchronisés en parallèle (sous `-race`).
- **Test de lecture seule** : le serveur simulé fait échouer le test dès qu'il reçoit une requête
  `POST`, `PATCH`, `PUT` ou `DELETE`, et un test vérifie que le transport du provider les refuse avant
  l'envoi.
- **Test de fuite** : dans tous ces scénarios, les logs (capturés au niveau Debug) et les erreurs ne
  contiennent ni le faux token, ni en-tête `Authorization`, ni message de commit, nom ou e-mail.

## Phase 3 : moteur, CLI et TUI

- **Store** (`internal/store/`, base temporaire) : verrou de synchro (8 goroutines concurrentes, une
  seule le prend ; `running` périmé clos « synchro interrompue »), écriture d'une synchro en une
  transaction (annulée en entier sur erreur), `INSERT … ON CONFLICT DO NOTHING`, fusion des identités,
  `last_commit_at` sur les branches suivies, vues de lecture (filtres dépôt, auteur, branche, période,
  limite ; classement ; résumés).
- **Activité et import** (`internal/activity/`, `internal/repoimport/`, sans base) : seuils globaux et par
  dépôt, bornes 5/6 et 15/16 jours, jours calendaires (passage de minuit à Paris, heure d'été), priorité
  des états gris ; CSV (BOM, CRLF, guillemets, colonnes dans le désordre), liste simple, commentaires,
  doublons, chaque erreur avec son numéro de ligne, aucun identifiant de connexion dans les messages ;
  colonnes `flemme` et `dead` (cellule vide, `0`, valeur illisible, Dead inférieur ou égal à Flemme).
- **Seuils par dépôt** : création, mise à jour, seuil gardé s'il n'est pas fourni et remise au seuil de
  l'espace avec `0` (`internal/sync`, `DryRun` compris), `repo add --flemme / --dead` et colonnes de
  l'import (`internal/cli`, état recalculé par `repo list`), export, aperçu et import du partage
  (`internal/configshare`), formulaire de la page d'un dépôt (`internal/web` : enregistrement, saisie
  refusée, `404`, CSRF).
- **Activité par contributeur dans un dépôt** (V2) : commits par semaine et par contributeur
  (`activity.CountContributorWeeks` : fuseau, bornes de la fenêtre, contributeur absent) et courbe d'une
  ligne (`activity.Sparkline`, Unicode et ASCII) ; `store.AuthorCommitTimes` filtré par dépôt (commit
  sur deux branches compté une fois, autres dépôts exclus) ; graphique de la page d'un dépôt
  (`TestRepoContributorsChart` : séries du seul dépôt, absent des fragments ; dépôt sans commit sur la
  période) ; détail du TUI (colonnes « 12 SEM. » et « PAR SEMAINE », faux `DataSource` et vraie base) ;
  `watchoo contributors --weeks` (texte, `--repo`, JSON `weekly`, `[]` sans l'option, bornes et refus
  avec `--since`, `TestContributorsWeeks`).
- **Pull requests, releases et tags** (V2) : provider GitHub contre `githubtest` (conversion, ordre,
  pagination et plafonds de 1 000, arrêt anticipé `UpdatedSince` sur une seule page, `Limit`, `304`
  gratuits, dépôt vide, `409`, réponse inexploitable, `5xx`, `404`, arguments invalides sans requête ;
  `pulls_test.go`), routes de `githubtest` (`state`, `sort`, `direction`, pagination) et faux
  `providertest` (tri, `UpdatedSince`, erreur injectée `ErrUnsupported`, quota) ; schéma `00003`
  (unicité, `CHECK`, cascades), écriture par `ApplyRepoSync` (création, mise à jour, remplacement des
  releases et tags, conservation quand la liste n'est pas relue, refus d'une donnée invalide sans rien
  écrire) et lectures (filtres d'état, tags datés par leur commit, résumé des dépôts) ; moteur
  (`extras_test.go` : premier passage puis incrémental, échec isolé en avertissement avec commits gardés
  et reprise au même point, `ErrUnsupported` sans avertissement ni effacement, quota bas et épuisé,
  annulation) ; CLI (`watchoo prs` et `watchoo releases` en texte et en JSON, filtres, options
  invalides, titres piégés neutralisés, avertissements de `watchoo sync`) ; web (cartes et fragment
  `repo-pulls`, échappement d'un titre piégé, états vides, PR ouvertes sur le tableau de bord) ; TUI
  (ligne « Pull requests » du détail, avertissements du bilan). Les deux tests de bout en bout lisent
  aussi PR, releases et tags (`304` quand rien n'a changé).
- **Moteur** (`internal/sync/`, `providertest` + base temporaire) : premier import, synchro
  incrémentale, tête inchangée (aucune requête de commits), nouvelle branche, merge qui apporte des
  commits plus anciens, force-push (branche et branche de référence), branche supprimée,
  `branch_patterns` et `branches.exclude`, branche par défaut changée ou renommée (ancienne supprimée),
  motifs qui excluent après coup la branche par défaut (nouvelle référence relue en entier,
  `last_commit_at` et état d'activité conservés), label invalide refusé, dépôt renommé, transféré ou vide,
  `404` et SSO, quota bas avant et pendant la synchro, `RateLimitError`, limite secondaire, verrou (deux
  moteurs sur la même base), parallélisme borné, règles des statistiques, token redemandé à chaque
  synchro et masqué, annulation. Un scénario passe par le vrai provider et `githubtest`.
- **CLI** (`internal/cli/`, `authtest` ou faux `gh`, `providertest`) : chaque commande en texte et en
  `--json` (jeu de clés figé par les tests), hors espace, dépôt inconnu, arguments invalides, `--since`
  et `-n`, import (`--dry-run`, bilan, code de retour, fichier invalide), dépôts verrouillés ou reportés,
  `gh` déconnecté, carte de `status`, quota bas signalé par `status` (texte et `quota_low`),
  confirmation de `repo remove` (`--yes` hors terminal), `repo add --label` avec un caractère de
  contrôle refusé, sortie texte de `log`, `contributors` et `repo list` sans aucune séquence
  d'échappement quand messages et noms Git en contiennent (`internal/textclean` testé à part).
- **Bout en bout** (`internal/cli/e2e_test.go`) : vrai `auth.GH` avec le faux `gh`, vrai provider GitHub
  contre `githubtest`, vrai moteur, base temporaire : `init` → `repo add` → `repo import` (avec
  `--dry-run`, un dépôt déjà suivi et un inaccessible) → `sync` → `log` (filtres) → `contributors` →
  `repo list` (Actif / Flemme / Dead) → `status` → nouveaux commits et synchro incrémentale (compare,
  jamais la liste complète ; rien quand rien n'a changé ; uniquement des `GET`) → `repo remove`
  (`--keep-history`, puis suppression). Ni token ni message de commit ni e-mail dans les logs.
- **TUI** (`internal/tui/`) : `Init` / `Update` / `View` appelés directement (sans `teatest`), avec un
  faux `Syncer` qu'on peut suspendre et une vraie base temporaire : vues, modes ASCII et couleur,
  défilement, filtre, détail, synchro (progression, refus pendant une synchro, erreurs, report en heure
  de Paris), synchro automatique, quitter pendant une synchro, aide. Un test lance `tui.Run` avec le
  vrai moteur, une base temporaire et un provider bloquant : quitter pendant la synchro attend sa fin,
  le `sync_run` n'est plus `running` et le dépôt n'est pas verrouillé. `watchoo tui` : refus hors
  terminal, `--json`, valeurs de `--interval`.
- **Fuites** : à chaque commande de la CLI, le harness vérifie que le faux token n'apparaît ni sur les
  sorties ni dans les logs ; les tests du moteur vérifient aussi l'absence de nom, d'e-mail, de login et
  de message de commit dans les logs et les rapports.

## Phase 4 : interface web

- **Store et activité** : historique paginé avec total (`CommitPage` : bornes, filtres), dates de commit
  depuis une date (`CommitTimes`), commits par contributeur et par dépôt, fin de la dernière synchro ;
  série d'activité par jour (`activity.DailyCounts` : 90 jours, fuseau, changement d'heure, dates hors
  fenêtre).
- **Coordinateur** (`internal/sync`, faux `Syncer` suspendu, minuterie simulée) : synchro au démarrage
  seulement si la dernière date de plus d'un intervalle, synchro périodique, synchro manuelle refusée
  pendant une synchro, progression, erreur globale, changement d'intervalle (bornes), arrêt qui annule
  et attend la synchro, délai d'arrêt dépassé ; un scénario avec le vrai moteur.
- **Web** (`internal/web/`, `httptest` sur le routeur complet, base temporaire remplie de données
  factices, vrai moteur sur `providertest` et `authtest`) : chaque page (200, contenu clé, message de
  commit contenant `<script>` échappé), 404 et 405 en français, pagination et filtres, fragments HTMX
  (en-tête `HX-Request`), `POST` sans jeton CSRF, avec un mauvais jeton ou d'une origine étrangère
  refusés (`403`, rien n'est modifié), aucun `GET` qui modifie, `Host` étranger refusé (`421`, page qui
  ne montre rien de l'espace), y compris derrière `web.trusted_proxy` (noms de `web.allowed_hosts`
  acceptés), authentification basique (bon et mauvais identifiants, statiques
  compris), en-têtes de sécurité (CSP sans `unsafe-inline` ni `unsafe-eval`), journal d'accès sans
  query string, corps trop gros, panique rattrapée, synchro manuelle concurrente refusée, progression et
  `HX-Trigger: sync-finished` (aussi pour une synchro finie avant la réponse au `POST`), boutons de
  synchro jamais désactivés au rendu, arrêt propre de `Serve` (synchro en cours annulée), fichiers statiques
  (Content-Type, cache, `ETag` et `304`), copies des logos identiques à `assets/logo`, `app.css` présent
  et à jour, empreintes de `tailwind/VERSIONS`, et ni token ni e-mail dans aucune réponse ni dans le
  journal.
- **Pages** : tableau de bord (tri Dead, Flemme, Actif puis gris, compteurs, fil limité à 20, URL de
  commit en `javascript:` neutralisée, espace vide) ; détail (chaque état, suivi arrêté sans bouton de
  synchro, pagination sur 120 commits avec bornes, filtres branche et auteur gardés d'une page à l'autre,
  filtre contenant un e-mail ignoré, `HX-Push-Url`, données du graphique dans le fuseau d'affichage) ;
  contributeurs (classement, commits par dépôt, avatar seulement depuis `avatars.githubusercontent.com`
  avec `referrerpolicy` et `loading`, échappement) ; réglages (ajout, déjà suivi, saisies refusées sans
  rien créer, dépôt inaccessible `422`, erreur GitHub `502` sans détail, modification, confirmation de
  retrait en `GET` qui ne modifie rien, arrêt du suivi, reprise, suppression refusée pendant une
  synchro, intervalle et ses bornes, fragment de quota, messages `?msg=` jamais recopiés).
- **Bout en bout** (`internal/web/e2e_test.go`) : routeur complet, vrai moteur et vrai coordinateur,
  vrai provider GitHub sur `githubtest`, faux `gh` (`authtest`) ; ajout de dépôts depuis les réglages
  (`POST` avec CSRF ; dépôt caché refusé ; sans jeton : `403`), synchro manuelle par le bouton,
  `HX-Trigger: sync-finished`, tableau de bord avec les trois états dans le bon ordre, détail avec
  pagination et filtres, nouvelle synchro sans commit re-téléchargé, contributeurs, arrêt du suivi puis
  suppression depuis les réglages ; uniquement des `GET` vers GitHub, et ni token ni e-mail dans les
  réponses ou le journal.
- **CLI** (`watchoo serve`) : `--json` et arguments refusés, `--interval` et `--addr` invalides, refus
  hors boucle locale sans authentification (aucune écoute ouverte), règles du mot de passe, port déjà
  pris, démarrage réel sur un port libre de `127.0.0.1` (fragment et CSS servis), carte d'en-tête,
  avertissement du reverse proxy, authentification basique, synchro au démarrage puis pas de nouvelle
  synchro après une synchro récente, arrêt propre (code 0).

## Phase 5 : finitions du MVP

- **Activité** (`internal/activity/series_weeks_test.go`) : commits par semaine du lundi au dimanche
  (`WeeklyCounts`) dans le fuseau d'affichage (Paris, passages à l'heure d'hiver et d'été, fin d'année,
  UTC+14, nombre de semaines invalide), résumé d'une série par jour (total, jour le plus chargé).
- **Store** (`TestAuthorCommitTimes`) : auteur et date de chaque commit, auteur inconnu exclu, dates en
  UTC, borne `since`, filtre par dépôt.
- **Graphiques web** (`internal/web/charts_test.go`) : séries des 8 contributeurs les plus actifs et
  regroupement des autres (égalités, auteur hors classement, noms nettoyés et tronqués, `[]` plutôt que
  `null`) ; JSON des pages tableau de bord et contributeurs, absent des fragments et d'un espace vide ;
  nom contenant `</script>` échappé.
- **Retouches web** : `HEAD` accepté sur toutes les routes `GET`, `405` sur les routes `POST`, `404` sur
  une page inconnue (`routes_test.go`) ; cookie CSRF posé sur les pages HTML et jamais sur les fichiers
  statiques (`TestCSRFCookieOnlyOnHTML`) ; lecture du quota interrompue par un contexte annulé journalisée
  en debug, autres échecs en avertissement (`TestSettingsQuotaCanceled`).
- **Sortie `--json`** (`internal/cli/json_schema_test.go`) : les 10 commandes qui produisent des données
  écrivent un seul objet JSON avec exactement les clés attendues ; ces clés sont comparées au tableau de
  [Sortie JSON](05-cli.md#sortie-json) (`TestJSONSchemaDocumented` lit `05-cli.md` : garder le format de
  ses lignes) ; espace vide (`[]`, `null`), trois issues de `repo remove`, entrées en erreur.
- **Version** (`TestVersionBuildMetadata`) : `-ldflags` complets, `-ldflags` prioritaires sur les
  informations de compilation, `go install …@v0.1.0`, `go build` local, arbre modifié (`dev`), commit hors
  tag (pseudo-version), date illisible ou hors UTC, rien de connu ; `--version --json` identique à
  `version --json`, carte d'en-tête avec la même version.
- **Distribution** (hors `go test`, vérifiée à la main avant la release, sans token réel) :
  `make release-check`, `make release-snapshot` (6 archives et leur contenu, fichier de sommes,
  `version --json` du binaire), `make docker` (utilisateur 65532, `WATCHOO_DIR=/data`, `gh --version`) et
  un essai du conteneur avec le faux `gh` compilé pour Linux (`init`, refus de `serve` sans
  authentification hors boucle locale, `401` sans identifiants puis `200`, arrêt propre).

## Dépôts Git locaux (V2)

Aucun service extérieur : le paquet `internal/provider/git/gittest` crée de vrais dépôts nus
(`<temp>/<owner>/<name>.git`) et une copie de travail pour les remplir (commits datés, d'une
identité factice, branches, merges, tags légers et annotés, force-push, suppression de branche, changement
de branche par défaut), avec une configuration Git isolée de celle du développeur (`GIT_CONFIG_GLOBAL`
vide, `GIT_CONFIG_NOSYSTEM`, pas d'entretien automatique). Le provider s'y branche par
`git.Options.BaseURL` (`file://…`).

- **Provider** (`internal/provider/git`) : `Repository` par `ls-remote` sans rien cloner (branche par
  défaut, URL GitHub, `Partial`) ; clone nu en `0700` ; branches, `ListCommits` (HEAD, branche, SHA,
  `Limit`, `Since`, `Stop`, branche inconnue `ErrNotFound`), message, identités, dates UTC et parents ;
  `CompareCommits` avec un merge, base égale à la tête, base inconnue (`ErrUnknownBase`) ; statistiques
  (`diff-tree --numstat` : commit simple, racine, merge par rapport au premier parent, commit inconnu) ;
  tags (légers et annotés déréférencés) ; `fetch` qui suit une branche supprimée, un force-push, un tag
  ajouté et une nouvelle branche par défaut (une seule mise à jour par Provider) ; dépôt vide (listes
  vides non nil) ; références invalides refusées sans lancer `git` (`git` introuvable dans ce test) ;
  dépôt absent : `ErrAccessDenied` sans invite, aucun reste de clone, URL absente du message ; port fermé :
  `ErrUnavailable` ; 16 appels concurrents sur 4 dépôts ; `RemoveClone` (casse, répertoire du
  propriétaire vidé) ; `Check` ; aucun nom, e-mail, message ni URL dans les logs ; `ErrUnsupported` pour
  pull requests, releases, `RepositoryByID` et quota.
- **Migration `00004`** (`internal/store/migration_provider_id_test.go`) : base remplie au schéma
  `00003` (toutes les tables filles), migration, aucune ligne perdue, `foreign_key_check` et
  `integrity_check` propres, clés étrangères réactivées, cascade toujours active, casse ignorée ;
  plusieurs `provider_id` `NULL`, `0` refusé, écriture `Partial` qui garde URL, étoiles, issues, archivé et
  identifiant, identifiant résolu par une synchro « API ».
- **Moteur** (`internal/sync/gitsource_test.go`) : choix de la source (gh connecté, absent, non connecté,
  `github.user` inutilisable : clones ; délai et réponse illisible : erreur) ; ajout sans `gh`
  (`--dry-run` qui planifie une fois un dépôt écrit avec une autre casse, dépôt absent « accès refusé »,
  second ajout retrouvé par nom) ; premier import (branches, commits propres à une branche, tags,
  contributeur sans login), synchro incrémentale avec statistiques, dépôt devenu inaccessible, retour de
  `gh` qui résout `provider_id` et les métadonnées ; dépôt connu par l'API synchronisé sans `gh` (étoiles,
  issues, archivé, URL et identifiant gardés, aucun appel à l'API).
- **CLI** (`internal/cli/gitsource_test.go` et tests d'`init`, `status`, `sync`, `repo`) : `repo add`
  sans clone, `sync` (texte et `--json`, `source: "git"`, pas de quota), `log`, `status` (source),
  `repo remove --keep-history` qui garde le clone, `repo remove` qui le supprime ; `init` sans `gh`
  (faux client, faux binaire, vrai `auth.GH` vers un chemin absent) qui crée l'espace et prévient, sans
  `git` qui s'arrête, token refusé qui s'arrête ; clé `source` des sorties JSON (`init`, `sync`,
  `status`).
- **Web et TUI** : page d'un dépôt sans identifiant de forge (étoiles, issues, pull requests et releases
  masquées), fragment du quota « sans objet » sans `gh` ; détail du TUI correspondant.

## Partage de la configuration

- **Cœur** (`internal/configshare`) : aller-retour export → import dans un autre espace (dépôts, labels,
  branches, réglages) ; export sans dépôt arrêté, trié, sans token, e-mail, chemin ni clé `github` /
  `web` ; chaque refus de validation (JSON invalide ou suivi d'un autre texte, plus de 1 Mio, `format`
  absent ou différent, version plus récente ou nulle, clé inconnue à chaque niveau, `github` / `web`
  dans `settings`, plus de 1 000 dépôts, doublon sans tenir compte de la casse, `repo` invalide,
  `provider` autre que `github`, label invalide, motif invalide ou avec virgule, durée illisible,
  réglages incohérents une fois fusionnés) ; aperçu : ajout, mise à jour (avant → après), inchangé,
  dépôt arrêté à reprendre, réglages modifiés seulement, avertissements (variable `WATCHOO_*`,
  commentaires perdus) ; aperçu sans écriture (base et `config.yaml` identiques octet pour octet) ;
  application : dépôt inaccessible signalé sans bloquer les autres, réglages identiques non réécrits.
- **Réécriture de `config.yaml`** (`internal/workspace`) : clés `name`, `github.*`, `git.*`, `web.*` et réglages
  non importés conservés, `.bak` identique à l'ancien fichier, droits `0600`, idempotence (deux imports
  donnent le même fichier), injection YAML impossible (fuseau et motifs avec guillemets, `#`, `:`, sauts
  de ligne, `\n`), fichier relu égal aux valeurs attendues, échec sans modification, gabarit de
  `watchoo init` inchangé.
- **CLI** (`watchoo config`) : export sur la sortie standard et avec `-o` (`0600`, refus d'écraser sans
  `--force`), import en texte et en `--json` (clés du tableau [Sortie JSON](05-cli.md#sortie-json)),
  `--dry-run`, confirmation au terminal, `--yes` obligatoire sinon, code de retour non nul s'il reste des
  dépôts inaccessibles.
- **Web** : export (en-têtes, nom de fichier sûr, contenu, `GET` sans effet), aperçu sans écriture,
  `apply` sans jeton CSRF refusé (`403`), document modifié entre l'aperçu et `apply` revalidé, taille
  dépassée (`413`), aucun token dans les réponses.
- **Bout en bout** (`internal/cli/e2e_config_test.go`, faux `gh` et `githubtest`) : espace A (`repo
  add`, `config export -o`), espace B (`config import --dry-run` sans aucune écriture, refus hors
  terminal sans `--yes`, `--yes` avec un dépôt inaccessible et code `1`, dépôts et `config.yaml`
  vérifiés, `.bak`, synchro, réimport sans changement), espace C (aperçu puis « Appliquer » dans un vrai
  `watchoo serve`, jeton CSRF et cookie compris) ; ni token ni e-mail dans les réponses et les logs.

## Logo et carte d'en-tête

- Les copies embarquées dans `internal/logo/art/` sont identiques octet pour octet à `assets/logo/*.txt`
  (sinon : `make logo`).
- Carte : toutes les lignes ont la même largeur affichée (accents, « », caractères de bloc), coupure de
  l'astuce, borne de 80 colonnes, pas de carte sous 40, repli 100 % ASCII, couleurs seulement si permises.
- CLI : carte affichée seulement sur un terminal simulé, jamais avec `--json`, sur stderr ni dans une sortie
  redirigée (la sortie de `watchoo version` hors terminal ne change pas).

## En CI

À chaque push et chaque pull request :

1. `go vet` et les tests (`go test ./...`, avec le détecteur de concurrence `-race`).
2. `sqlc generate` puis vérification qu'aucun fichier généré ne change (code commité à jour).
3. `make css-check` (CLI autonome Tailwind de la version fixée dans `internal/web/tailwind/VERSIONS`,
   installée avec vérification SHA-256) : `app.css` commité est à jour.
4. Aucune variable `GH_TOKEN` / `GITHUB_TOKEN` n'est fournie aux tests : un test qui aurait besoin du vrai
   GitHub échoue au lieu de passer en silence.
5. `git` est installé sur les machines de CI : les tests des dépôts Git locaux y tournent toujours (ils
   ne sont sautés que si `git` est absent, en local).

Sur un tag `vX.Y.Z`, le workflow de release relance les mêmes contrôles (`make check`) avant GoReleaser
et l'image Docker ([Distribution](03-architecture.md#distribution)).

## Script d'installation

`cmd/watchoo/install_test.go` lance `install.sh` avec `sh` contre un faux serveur de releases
(`httptest`, variable `WATCHOO_RELEASES_URL`, seule à autoriser HTTP), sans réseau : dernière version lue
dans la redirection de `/latest`, version fixée sans `v`, répertoire par défaut `~/.local/bin`, binaire
installé en `0755` sans fichier temporaire oublié ; somme SHA-256 fausse ou archive absente du fichier de
sommes refusées sans rien installer ; version inexistante, version invalide, `HOME` vide. Les tests sont
sautés hors Linux / macOS ou sans `sh`, `curl` et `tar`. En CI (Ubuntu), `sh` est `dash` : le script reste
POSIX.
