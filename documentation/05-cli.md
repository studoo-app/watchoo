# 5. CLI

[← Sommaire](README.md)

## Options globales

| Option | Effet |
| --- | --- |
| `--json` | Sortie JSON pour rendre toutes les commandes scriptables |
| `-C <répertoire>` / `--workspace <répertoire>` | Travaille sur l'espace de ce répertoire au lieu de celui trouvé à partir du répertoire courant |
| `--verbose` | Logs détaillés (niveau debug) sur la sortie d'erreur. Sans elle, le niveau vient de la variable `WATCHOO_LOG_LEVEL` (`debug`, `info`, `warn` par défaut, `error`). Pas de lettre courte : `-v` serait confondu avec `--version` |

Toutes les commandes sauf `init` et `version` travaillent sur un **espace de supervision** : le répertoire
`.watchoo/` trouvé dans le répertoire courant ou ses parents (voir
[Espaces de supervision](10-espaces-de-supervision.md)).

**Prérequis : la CLI GitHub `gh`, installée et connectée** (`gh auth login`). Watchoo obtient le token
uniquement avec `gh auth token` et ne le stocke jamais (voir
[Architecture](03-architecture.md#authentification--gh-imposé)).

## Commandes

| Commande | Effet | Version |
| --- | --- | --- |
| `watchoo init` | Fait du répertoire courant un espace de supervision : vérifie `gh`, puis crée `.watchoo/` avec la config et la base SQLite. Refuse si l'espace existe déjà | MVP |
| `watchoo version` | Affiche la version de Watchoo, le commit compilé et sa date, la version de Go et la plateforme (`watchoo --version` fait de même). Fonctionne hors d'un espace et sans `gh` | MVP |
| `watchoo repo add owner/repo [--branch main,dev,release/*] [--label "Alice Martin"]` | Ajoute un dépôt à surveiller (toutes ses branches par défaut), avec un nom affiché facultatif | MVP |
| `watchoo repo import fichier.csv [--dry-run]` | Ajoute d'un coup une liste de dépôts (ex. ceux d'une promotion), vérifie l'accès à chacun et affiche un bilan | MVP |
| `watchoo repo add-org mon-org` | Ajoute tous les dépôts d'une organisation | V2 |
| `watchoo repo list` | Tableau : dépôt, dernier commit, auteur, état (Actif / Flemme / Dead) | MVP |
| `watchoo repo remove owner/repo [--keep-history]` | Arrête le suivi et supprime l'historique du dépôt après confirmation (`--keep-history` pour le garder) | MVP |
| `watchoo sync [owner/repo]` | Synchronise tout ou un seul dépôt | MVP |
| `watchoo log [--repo X] [--author Y] [--branch B] [--since 7d] [-n 20]` | Derniers commits filtrés | MVP |
| `watchoo contributors [--repo X] [--since 30d]` | Classement des contributeurs | MVP |
| `watchoo status` | Résumé : dépôts à jour, inactifs, en erreur, quota API restant, compte `gh` utilisé et ses droits (sans afficher le token) | MVP |
| `watchoo serve [--addr 127.0.0.1:8080] [--interval 30m]` | Lance l'interface web et la synchro périodique (30 min par défaut, 15 min minimum) | MVP |
| `watchoo tui [--interval 30m]` | Ouvre le mode interactif dans le terminal | MVP |

### Détails

- **`repo add`** accepte `owner/repo` ou une URL GitHub complète. Sans `--branch`, **toutes les branches**
  du dépôt sont suivies, y compris celles créées plus tard. `--branch` restreint le suivi à une liste et à
  des motifs (`--branch main,dev,release/*`) ; une nouvelle branche qui correspond à un motif est suivie
  automatiquement. Voir [Architecture](03-architecture.md#branches-suivies).
  `--branch ""` revient à toutes les branches. Un dépôt déjà suivi n'est pas ajouté en double : son
  label et ses branches sont mis à jour s'ils sont donnés, et son suivi reprend s'il avait été arrêté.
  L'accès est vérifié auprès de GitHub (une requête) ; `repo add` ne lance pas de synchro.
- **`repo list`** affiche tous les dépôts de l'espace, suivi arrêté compris. La colonne DÉPÔT montre le
  label s'il existe, sinon `owner/repo` ; l'auteur est celui du dernier commit. États gris : `arrêté`
  (suivi arrêté), `erreur`, `jamais` (jamais synchronisé), `archivé`, `vide` (synchronisé sans commit).
- **`repo remove`** demande confirmation (`[o/N]`) quand l'entrée standard est un terminal ; sinon (script,
  tube) l'option **`--yes` / `-y` est obligatoire**. Une réponse autre que `o`, `oui`, `y` ou `yes`, ou un
  Ctrl+C pendant la question (pris en compte tout de suite), annule avec un code de retour non nul. Sans `--keep-history`, le dépôt, ses branches, commits et synchros sont
  supprimés de la base (les contributeurs restent) ; avec `--keep-history`, le suivi s'arrête (le dépôt
  n'est plus synchronisé par `watchoo sync`, il apparaît `arrêté`) et `repo add` le reprend.
- **`sync`** affiche un tableau par dépôt (statut `ok`, `erreur`, `ignoré` s'il est déjà en cours de
  synchronisation, `reporté` si le quota est bas, avec l'heure de reprise en heure locale), le bilan et le
  quota restant. `watchoo sync owner/repo` synchronise ce dépôt même si son suivi est arrêté. Code de
  retour non nul si un dépôt est en erreur.
- **`--since`** accepte une durée relative : un entier suivi de `d` (jours), `h` (heures) ou `m`
  (minutes), par exemple `7d`, `30d`, `12h`.
- **`log -n`** (ou `--limit`) limite le nombre de commits affichés (20 par défaut, `0` = tous). Un commit
  présent sur plusieurs branches n'apparaît qu'une fois.
- **`log --author`** compare le login GitHub, le nom ou un e-mail de l'auteur, **à l'identique** (pas de
  sous-chaîne), sans tenir compte de la casse des lettres ASCII (« ÉLODIE » ne trouve pas « élodie »).
- **`log --branch`** montre les commits **vus sur cette branche** pendant les synchros : ceux qu'elle
  partage avec la branche de référence (souvent `main`) n'y sont rattachés que s'ils ont été vus en la
  comparant (voir [Architecture](03-architecture.md#synchronisation-incrémentale)).
- **`contributors`** classe les **auteurs** de commits par nombre de commits. Les lignes ajoutées et
  supprimées ne sont connues que pour les commits découverts après le premier import d'un dépôt.
- **`status`** : compte `gh` et ses droits, quota restant (lecture gratuite), dépôts par état, dépôts en
  erreur, dernière synchro réussie, branches suivies. Sous 10 % de quota restant, le garde-fou est
  signalé (« Quota bas : synchros reportées jusqu'à HH:MM », `quota_low` en JSON), avec la même règle
  que le moteur. Un `gh` non connecté est signalé sans faire échouer
  la commande. Le token n'est jamais affiché.
- **Sorties** : un tableau texte aligné, sans bordure, en-têtes en majuscules ; dates relatives
  (« il y a 2 h ») en heure locale (réglage `timezone`, sinon fuseau du système). Couleurs (en-têtes en
  gras, états gris en gris) seulement sur un terminal, sans `NO_COLOR` ni `TERM=dumb`. Sur un terminal
  sans Unicode, symboles et en-têtes ASCII (`! 42 j`, `DEPOT`) ; une sortie redirigée reste en UTF-8.
  Les textes venus des dépôts ou de la base (messages de commit, noms Git, labels, messages d'erreur)
  sont **nettoyés** avant d'être écrits, en texte comme dans le TUI : séquences d'échappement (ESC, CSI,
  OSC) retirées, autres caractères de contrôle et de format (sauts de ligne, marques de sens d'écriture)
  remplacés par une espace. Un étudiant ne peut donc ni changer le titre de la fenêtre ni effacer
  l'écran de l'enseignant. Le JSON garde le texte d'origine, échappé par l'encodeur.
- **Labels** : `repo add --label` applique la même règle que l'import (UTF-8 valide, aucun caractère de
  contrôle, 200 caractères au plus) ; un label invalide est refusé avant tout appel à GitHub.
- **`--json`** : chaque commande qui produit des données écrit un objet JSON stable (clés en anglais,
  `snake_case`, dates UTC RFC 3339 ou `null`) ; ses clés sont listées dans [Sortie JSON](#sortie-json)
  et vérifiées par les tests. Les erreurs vont sur la sortie d'erreur.
- **Hors espace** : toutes les commandes sauf `init`, `version` et `help` s'arrêtent avec « aucun espace
  Watchoo ici, lancez « watchoo init » (ou indiquez un espace avec -C <répertoire>) ».
- **`serve`** écoute sur `web.addr` (`127.0.0.1:8080` par défaut ; `--addr` le remplace) et affiche
  l'URL à ouvrir (carte d'en-tête sur un terminal). Hors boucle locale (`127.0.0.0/8`, `::1`,
  `localhost`), il refuse de démarrer sans authentification : `web.auth.username` avec le mot de passe
  dans la variable `WATCHOO_WEB_PASSWORD` (12 caractères au moins, jamais dans un fichier), ou
  `web.trusted_proxy: true` derrière un reverse proxy authentifiant, avec ses noms publics dans
  `web.allowed_hosts` (avertissement au démarrage ; sans `web.allowed_hosts`, refus). Port
  déjà pris : message clair. `--interval` comme pour `tui` (`0` = synchro manuelle seule) ; au démarrage,
  une synchro n'est lancée que si la dernière date de plus d'un intervalle. `--json` est refusé. Ctrl+C
  (ou SIGTERM) arrête proprement : plus de nouvelle requête, synchro en cours annulée et attendue, base
  fermée ensuite. Voir [Interface web](06-interface-web.md).
- `watchoo help [commande]` et l'option `-h` / `--help` affichent l'aide en français. Il n'y a pas de
  commande `completion` (celle que `cobra` ajoute d'office est désactivée : son aide est en anglais).
- **`--version`**, option de `watchoo` seul (`watchoo init --version` est refusée), a le même effet que
  `watchoo version`, `--json` compris.

## Sortie JSON

Avec `--json`, chaque commande qui produit des données écrit **un seul objet JSON** (indenté, suivi d'une
fin de ligne) sur la sortie standard, et rien d'autre : ni carte d'en-tête, ni astuce. Avertissements,
questions de confirmation et erreurs vont sur la sortie d'erreur ; en cas d'erreur, la sortie standard
reste vide (sauf `repo import`, qui écrit son bilan puis rend un code non nul s'il reste des dépôts
inaccessibles).

Règles communes, qui forment un **contrat stable** pour les scripts (une clé n'est ni renommée ni
retirée sans version incompatible : MINOR tant que Watchoo est en 0.y.z, MAJOR à partir de 1.0.0, voir
[Skills](11-skills.md)) :

- clés en anglais, en `snake_case` ; toutes les clés sont toujours présentes ;
- dates en UTC, RFC 3339 à la seconde (`"2026-10-27T10:00:00Z"`), `null` si inconnues ;
- listes vides écrites `[]`, jamais `null` ; un texte absent vaut `""` ;
- textes venus des dépôts (messages, noms, labels) gardés tels quels, échappés par l'encodeur ;
- jamais de token. `tui` et `serve` refusent `--json`.

Clés de premier niveau (la liste est vérifiée par un test, `internal/cli/json_schema_test.go`) :

| Commande | Clés de premier niveau | Notes |
| --- | --- | --- |
| `watchoo init` | `name`, `root`, `dir`, `config_file`, `database`, `parent_workspace`, `github`, `warnings` | `github` : `host`, `login`, `token_source`, `scopes` ; `parent_workspace` vide sans espace parent |
| `watchoo version` | `version`, `commit`, `date`, `go`, `os`, `arch` | Voir ci-dessous |
| `watchoo repo add` | `outcome`, `repository` | `outcome` : `added`, `already` ou `updated` ; `repository` : objet dépôt |
| `watchoo repo import` | `file`, `format`, `dry_run`, `summary`, `entries` | `format` : `csv` ou `list` ; `summary` : `added`, `already_tracked`, `updated`, `inaccessible`, `errors` ; chaque entrée : `line`, `repo`, `outcome`, `repository` (`null` si inaccessible ou en erreur), `error` |
| `watchoo repo list` | `repositories` | Chaque élément : objet dépôt plus `activity`, `commits`, `tracked_branches`, `last_commit` (objet commit ou `null`), `last_sync` (`null` si aucune) |
| `watchoo repo remove` | `action`, `repository`, `deleted_commits` | `action` : `deleted`, `stopped` ou `already_stopped` |
| `watchoo sync` | `started_at`, `finished_at`, `summary`, `repositories`, `rate_limit` | `summary` : `repositories`, `success`, `error`, `locked`, `deferred`, `new_commits` ; chaque dépôt : `id`, `full_name`, `display_name`, `status`, `new_commits`, `tracked_branches`, `stats_fetched`, `duration_ms`, `error`, `resume_at` |
| `watchoo log` | `commits` | Du plus récent au plus ancien ; objets commit |
| `watchoo contributors` | `contributors` | Chaque élément : `id`, `name`, `login`, `display_name`, `avatar_url`, `commits`, `additions`, `deletions`, `repositories`, `last_commit_at` |
| `watchoo status` | `workspace`, `github`, `rate_limit`, `rate_limit_error`, `quota_low`, `repositories`, `errors`, `last_synced_at`, `tracked_branches` | `workspace` : `name`, `root` ; `github` : `ok`, `host`, `login`, `token_source`, `scopes`, `error` ; `repositories` : `total`, `active`, `flemme`, `dead`, `error`, `never_synced`, `archived`, `empty`, `stopped` ; chaque erreur : `repo`, `message` |

Objets partagés :

- **dépôt** : `id`, `full_name`, `owner`, `name`, `label`, `display_name`, `url`, `default_branch`,
  `branch_patterns` (`[]` = toutes les branches), `enabled`, `archived`, `stars`, `open_issues`,
  `last_synced_at`, `last_commit_at` ;
- **commit** : `repository` (`id`, `full_name`, `display_name`), `sha`, `short_sha`, `message`, `url`,
  `author` (`name`, `login`, `display_name`), `authored_at`, `committed_at`, `additions`, `deletions` ;
- **activité** : `state` (`active`, `flemme`, `dead`, `stopped`, `error`, `never_synced`, `archived`,
  `empty`), `state_label`, `gray`, `days_since_last_commit` (`null` sans commit), `flemme_after_days`,
  `dead_after_days` ;
- **synchro** (`last_sync`) : `status` (`running`, `success`, `error`), `started_at`, `finished_at`,
  `new_commits`, `error` ;
- **quota** (`rate_limit`, `null` si inconnu) : `limit`, `remaining`, `used`, `reset`.

### Version

`watchoo version --json` (ou `watchoo --version --json`) :

```json
{
  "version": "v0.1.0",
  "commit": "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
  "date": "2026-10-27T10:00:00Z",
  "go": "go1.27.3",
  "os": "linux",
  "arch": "amd64"
}
```

- `version`, `commit` (SHA complet) et `date` (date **du commit** compilé, pour des compilations
  reproductibles) sont injectés à la compilation par `-ldflags` sur `internal/cli.Version`,
  `internal/cli.Commit` et `internal/cli.Date` (Makefile, GoReleaser et Dockerfile, mêmes options ; voir
  [Distribution](03-architecture.md#distribution)).
- Sans ces options, ils viennent des informations de compilation de Go quand elles existent :
  `go install github.com/studoo-app/watchoo/cmd/watchoo@v0.1.0` donne la version du module ; un
  `go build` dans un dépôt Git donne le commit, sa date et la version du module (`v0.1.0` sur un tag,
  une pseudo-version `v0.0.0-…` ailleurs, `dev` si l'arbre a des modifications non commitées). Sinon
  `version` vaut `dev`, `commit` `""` et `date` `null`.
- En texte : `watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, linux/amd64)` ; commit et date sont
  omis s'ils sont inconnus. La carte d'en-tête, le TUI, l'interface web et le `User-Agent` des appels
  GitHub montrent la même version.

## Carte d'en-tête

Sur un terminal, l'aide générale, `watchoo version` et le compte rendu de `watchoo init` commencent par une
**carte d'en-tête** : un cadre aux coins arrondis avec, à gauche, le hibou en petit format
(`ascii-mini.txt`, voir [Logo et icônes](06-interface-web.md#versions-texte-cli-et-tui)) et, à sa droite,
« Watchoo », la version et un sous-titre ; dessous, parfois, une astuce. Des lignes d'état précédées
d'un `●` peuvent suivre la carte. Exemple, `watchoo` lancé dans un espace :

```
╭───────────────────────────────────────────────────────────────────╮
│  █▄    ▄█  Watchoo v0.1.0                                         │
│  █ ●██● █  Qui a travaillé, sur quoi, et quand ?                  │
│  ▀██▙▟██▀                                                         │
│                                                                   │
│  Astuce : « watchoo repo add owner/repo » puis « watchoo sync ».  │
╰───────────────────────────────────────────────────────────────────╯

● Espace : licence-projet (~/supervision/licence-projet)

Watchoo supervise des dépôts GitHub, surtout privés, et répond à trois questions :
…
```

| Sortie | Sous-titre et astuce de la carte | Après la carte |
| --- | --- | --- |
| Aide générale : `watchoo`, `watchoo help`, `watchoo --help` / `-h` | « Qui a travaillé, sur quoi, et quand ? » ; astuce « `watchoo init` fait de ce répertoire un espace de supervision » si aucun espace n'est trouvé, « `watchoo repo add owner/repo` puis `watchoo sync` » dans un espace, « `watchoo help <commande>` détaille une commande » si l'espace est illisible | Une ligne vide, la ligne d'état de l'espace, une ligne vide, puis l'aide habituelle |
| `watchoo version`, `watchoo --version` | Version de Go et plateforme (`go1.27.3 · darwin/arm64`) ; pas d'astuce | Rien |
| `watchoo init` réussi | « Espace de supervision « nom » créé » ; astuce « `watchoo repo add owner/repo` ajoute un premier dépôt à suivre » | Une ligne vide, puis le compte rendu en lignes d'état : espace, réglages, base, compte `gh` et ses droits |
| `watchoo status` | « État de l'espace « nom » » ; astuce « `watchoo sync` met à jour les dépôts suivis » | Une ligne vide, puis le résumé habituel |

- **Scripts** : la carte n'est affichée que si la sortie standard est un terminal, et jamais avec `--json`.
  Une sortie redirigée (fichier, tube) reste identique : aide sans carte, `watchoo version` sur une seule
  ligne (`watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, darwin/arm64)`), compte rendu habituel de `init`. La sortie d'erreur
  (erreurs, avertissements) n'a jamais de carte, l'aide des sous-commandes (`watchoo help init`) non plus.
- **Ligne d'état de l'espace** (aide générale) : `● Espace : <nom> (<racine>)`, avec `~` pour le
  répertoire personnel, ou `● Aucun espace de supervision ici`. L'espace est cherché comme le feraient les
  autres commandes (répertoire courant et ses parents, `-C`, `WATCHOO_DIR`), localement, sans lancer
  `gh`. La recherche ne fait jamais échouer l'aide : une erreur (réglages invalides, répertoire de `-C`
  inexistant…) est résumée sur la ligne d'état, `● Espace : <message>`.
- **Largeur** : celle du contenu, sans dépasser 80 colonnes ni la largeur du terminal (variable `COLUMNS`
  si c'est un entier positif, sinon celle que donne le système). Un texte trop long est coupé sur les
  espaces. Sur un terminal de moins de 40 colonnes, pas de carte : la sortie est celle d'une sortie
  redirigée. Une lettre accentuée occupe une colonne, qu'elle soit précomposée (`é`) ou décomposée (`e`
  suivi d'un accent combinant, forme fréquente dans les noms de répertoires créés sous macOS, donc dans
  le nom par défaut d'un espace).
- **Unicode** : si la locale est en UTF-8 (première variable non vide parmi `LC_ALL`, `LC_CTYPE` et
  `LANG`) ou dans Windows Terminal (`WT_SESSION`). Sinon, la carte et les lignes d'état sont en ASCII pur :
  bordures `+`, `-` et `|`, hibou `ascii-mini-pur.txt`, guillemets droits au lieu de « », accents
  retirés, puce `*`.
- **Couleurs** : bordure, version, sous-titre et lignes d'état en gris, « Watchoo » en gras, pupilles du
  hibou en vert, commande citée dans l'astuce en cyan. Pas de couleur si `NO_COLOR` est définie et non
  vide, si `TERM=dumb`, ni sous Windows dans une console qui n'accepte pas les séquences ANSI.
- **Version** : un numéro reçoit le préfixe `v` (`0.1.0` devient `v0.1.0`) ; `v0.1.0` ou `dev` (binaire
  compilé sans version) sont affichés tels quels.

## Import groupé : `watchoo repo import`

Pour ajouter d'un coup tous les dépôts d'une promotion, au lieu d'un `repo add` par étudiant.

### Format du fichier

Un fichier **CSV** avec une ligne d'en-tête. Seule la colonne `repo` est obligatoire.

```csv
repo,label,branches
alice/projet-licence,Alice Martin,
bob/projet-licence,Bob Durand,"main,dev"
https://github.com/chloe/projet-licence,Chloé Petit,
mon-org/projet-groupe-3,Groupe 3 (Dan et Eva),
```

| Colonne | Obligatoire | Contenu |
| --- | --- | --- |
| `repo` | oui | `owner/repo` ou URL GitHub |
| `label` | non | Nom affiché à la place de `owner/repo` (nom de l'étudiant, du binôme, du groupe) |
| `branches` | non | Branches à suivre, liste et motifs comme `--branch` ; vide = toutes les branches |

Un simple fichier texte est aussi accepté : un dépôt par ligne, sans en-tête, lignes vides et lignes
commençant par `#` ignorées.

### Comportement

1. Lit tout le fichier et le **valide** avant d'ajouter quoi que ce soit : une ligne mal formée arrête
   l'import en indiquant le numéro de ligne. Un dépôt présent deux fois dans le fichier (`owner/repo` ou
   URL, sans tenir compte de la casse) est une ligne en erreur : rien n'est importé, le message donne la
   ligne du doublon et celle de la première occurrence.
2. Pour chaque dépôt, **vérifie l'accès** auprès de GitHub (une requête par dépôt). Un dépôt privé
   inaccessible (étudiant qui n'a pas encore donné l'accès) est signalé, sans bloquer les autres.
3. Ajoute les dépôts accessibles. Un dépôt **déjà suivi** n'est pas ajouté en double ; son `label` et ses
   branches sont mis à jour si le fichier en donne. On peut donc relancer l'import sans risque, par exemple
   quand des étudiants ont enfin donné l'accès.
4. Affiche un **bilan** :

```
IMPORT  licence-projet.csv
  ajoutés          18
  déjà suivis       3
  mis à jour        1
  inaccessibles     2   carol/projet-licence, dave/projet-licence
  lignes en erreur  0
```

- Une cellule `label` ou `branches` **vide** veut dire « non fourni » : la valeur existante est gardée (un
  import ne peut donc pas remettre un dépôt à « toutes les branches » ; utiliser `repo add owner/repo
  --branch ""`).
- `-` à la place du fichier lit la liste sur l'entrée standard.
- Le bilan détaille ensuite chaque dépôt inaccessible ou en erreur, avec son numéro de ligne.
- `--dry-run` : fait la validation et les vérifications d'accès, affiche le bilan (« à ajouter » au lieu
  d'« ajoutés »), mais n'enregistre rien.
- `--json` : bilan au format JSON.
- Code de retour non nul s'il reste des dépôts inaccessibles, pour les scripts.
- L'import ne lance pas de synchro : on enchaîne avec `watchoo sync`.

## Mode interactif (TUI)

`watchoo tui` ouvre un mode interactif dans le terminal (Bubble Tea). On y parcourt les dépôts, les commits
et les contributeurs au clavier, avec filtre et vue détail, et on peut y lancer une synchro. Voir
[Architecture](03-architecture.md#terminal--mode-interactif-tui).

- Seule la commande `tui` est interactive. Toutes les autres affichent un tableau texte ou du JSON
  (`--json`), puis rendent la main.
- `watchoo tui` lancé hors d'un terminal s'arrête avec un message clair.
- Tant qu'il est ouvert, `watchoo tui` synchronise automatiquement (30 min par défaut, désactivable). Une
  touche lance une synchro manuelle de tous les dépôts, une autre celle du dépôt sélectionné.
- Les couleurs sont désactivées hors terminal et quand `NO_COLOR` est définie.
- `--interval` remplace `sync.interval` le temps de la session : `0` pour la synchro manuelle seule, sinon
  15 min au minimum ; sans l'option, la synchro automatique suit `sync.auto` et `sync.interval`.
- Pendant le TUI, les logs ne sont pas écrits (ils casseraient l'affichage). `watchoo tui --json` est
  refusé, comme un argument en trop ; la commande s'arrête aussi si l'entrée ou la sortie standard n'est
  pas un terminal.
- **Vues** : Dépôts (dépôt, dernier commit, auteur, branches, état), Commits (quand, dépôt, auteur, SHA
  court, première ligne du message ; les 200 derniers) et Contributeurs (nom, login, commits, `+/-`,
  dépôts, dernier commit). Le **détail d'un dépôt** montre l'URL, l'état et ses seuils, le dernier commit,
  les branches, le nombre de commits, la dernière synchro (ou son erreur), étoiles et issues, les 15
  derniers commits et les 5 premiers contributeurs.
- **En-tête** : hibou mini (pupilles vertes), version et espace, onglets, ligne de synchro (en cours avec
  la progression, dernière synchro, prochaine synchro automatique à HH:MM, ou « Synchro manuelle (s) »).
- **Synchro** : une seule à la fois (une demande de plus est ignorée et signalée) ; à la fin, une ligne de
  bilan (nouveaux commits, erreurs, dépôts ignorés ou reportés avec l'heure de reprise locale), puis les
  vues sont relues. Une synchro automatique qui tombe pendant une synchro est sautée, la suivante reste
  planifiée. Quitter pendant une synchro l'annule : `watchoo tui` attend alors que le moteur ait clos
  ses synchros (« synchro annulée », quelques secondes au plus) avant de fermer la base, pour qu'aucun
  dépôt ne reste verrouillé.

| Touche | Effet |
| --- | --- |
| `tab` / `maj+tab`, `1` `2` `3` | Vue suivante / précédente, vue Dépôts, Commits, Contributeurs |
| `↑` `↓` ou `k` `j`, `pg.préc` `pg.suiv`, `g` `G` | Déplacer la sélection, par page, début et fin de la liste |
| `entrée` | Détail du dépôt sélectionné (vue Dépôts ou Commits) |
| `échap` | Retour, ou effacer le filtre |
| `/` | Filtrer la vue (sous-chaîne, sans tenir compte de la casse) ; `entrée` garde le filtre, `ctrl+u` le vide |
| `s` / `S` | Synchroniser tous les dépôts / le dépôt sélectionné |
| `r` | Relire la base |
| `?` | Aide des touches |
| `q`, `ctrl+c` | Quitter (pendant la saisie d'un filtre, seul `ctrl+c` quitte) |

## Exemples de sortie

`watchoo repo list` :

```
DÉPÔT                 DERNIER COMMIT     AUTEUR      ÉTAT
moi/projet-api        il y a 2 h         moi         -
moi/outils-reseau     il y a 5 j         bob         -
moi/scripts           il y a 9 j         moi         ~ 9 j
mon-org/site-web      il y a 42 j        alice       ⚠ 42 j
```

`-` = Actif (0 à 5 j), `~` = Flemme (6 à 15 j), `⚠` = Dead (plus de 15 j). Voir les
[règles d'activité](02-fonctionnalites.md#règles-fonctionnelles).

## Parcours type

Superviser les projets des étudiants de licence :

```bash
mkdir licence-projet
cd licence-projet
watchoo init
watchoo repo import promo-licence.csv
watchoo repo add retardataire/projet-licence --label "Zoé Leroy"
watchoo sync
watchoo repo list
watchoo contributors --since 7d
watchoo serve
```

Depuis n'importe où, consulter un autre espace : `watchoo -C ~/supervision/master-projet status`.
