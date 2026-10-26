# 5. CLI

[← Sommaire](README.md)

## Options globales

| Option | Effet |
| --- | --- |
| `--json` | Sortie JSON pour rendre toutes les commandes scriptables |
| `-C <répertoire>` / `--workspace <répertoire>` | Travaille sur l'espace de ce répertoire au lieu de celui trouvé à partir du répertoire courant |

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
| `watchoo version` | Affiche la version de Watchoo, de Go et la plateforme (`watchoo --version` fait de même). Fonctionne hors d'un espace et sans `gh` | MVP |
| `watchoo repo add owner/repo [--branch main,dev,release/*] [--label "Alice Martin"]` | Ajoute un dépôt à surveiller (toutes ses branches par défaut), avec un nom affiché facultatif | MVP |
| `watchoo repo import fichier.csv [--dry-run]` | Ajoute d'un coup une liste de dépôts (ex. ceux d'une promotion), vérifie l'accès à chacun et affiche un bilan | MVP |
| `watchoo repo add-org mon-org` | Ajoute tous les dépôts d'une organisation | V2 |
| `watchoo repo list` | Tableau : dépôt, dernier commit, auteur, état (Actif / Flemme / Dead) | MVP |
| `watchoo repo remove owner/repo [--keep-history]` | Arrête le suivi et supprime l'historique du dépôt après confirmation (`--keep-history` pour le garder) | MVP |
| `watchoo sync [owner/repo]` | Synchronise tout ou un seul dépôt | MVP |
| `watchoo log [--repo X] [--author Y] [--since 7d] [-n 20]` | Derniers commits filtrés | MVP |
| `watchoo contributors [--repo X] [--since 30d]` | Classement des contributeurs | MVP |
| `watchoo status` | Résumé : dépôts à jour, inactifs, en erreur, quota API restant, compte `gh` utilisé et ses droits (sans afficher le token) | MVP |
| `watchoo serve [--addr :8080] [--interval 30m]` | Lance l'interface web et la synchro périodique (30 min par défaut, 15 min minimum) | MVP |
| `watchoo tui` | Ouvre le mode interactif dans le terminal | MVP |

### Détails

- **`repo add`** accepte `owner/repo` ou une URL GitHub complète. Sans `--branch`, **toutes les branches**
  du dépôt sont suivies, y compris celles créées plus tard. `--branch` restreint le suivi à une liste et à
  des motifs (`--branch main,dev,release/*`) ; une nouvelle branche qui correspond à un motif est suivie
  automatiquement. Voir [Architecture](03-architecture.md#branches-suivies).
- **`--since`** accepte une durée relative (`7d`, `30d`, `12h`).
- **`log -n`** limite le nombre de commits affichés (20 par défaut).
- **`serve`** écoute sur `127.0.0.1` par défaut (voir [Interface web](06-interface-web.md)).
- `watchoo help [commande]` et l'option `-h` / `--help` affichent l'aide en français. Il n'y a pas de
  commande `completion` (celle que `cobra` ajoute d'office est désactivée : son aide est en anglais).
- **`--version`**, option de `watchoo` seul (`watchoo init --version` est refusée), a le même effet que
  `watchoo version`, `--json` compris.

## Carte d'en-tête

Sur un terminal, l'aide générale, `watchoo version` et le compte rendu de `watchoo init` commencent par une
**carte d'en-tête** : un cadre aux coins arrondis avec, à gauche, le hibou en petit format
(`ascii-mini.txt`, voir [Logo et icônes](06-interface-web.md#versions-texte-cli-et-tui)) et, à sa droite,
« Watchoo », la version et un sous-titre ; dessous, parfois, une astuce. Des lignes d'état précédées
d'un `●` peuvent suivre la carte. Exemple, `watchoo` lancé dans un espace :

```
╭───────────────────────────────────────────────────────────────╮
│  █▄    ▄█  Watchoo v0.1.0                                     │
│  █ ●██● █  Qui a travaillé, sur quoi, et quand ?              │
│  ▀██▙▟██▀                                                     │
│                                                               │
│  Astuce : « watchoo help <commande> » détaille une commande.  │
╰───────────────────────────────────────────────────────────────╯

● Espace : licence-projet (~/supervision/licence-projet)

Watchoo supervise des dépôts GitHub, surtout privés, et répond à trois questions :
…
```

| Sortie | Sous-titre et astuce de la carte | Après la carte |
| --- | --- | --- |
| Aide générale : `watchoo`, `watchoo help`, `watchoo --help` / `-h` | « Qui a travaillé, sur quoi, et quand ? » ; astuce « `watchoo init` fait de ce répertoire un espace de supervision » si aucun espace n'est trouvé, sinon « `watchoo help <commande>` détaille une commande » | Une ligne vide, la ligne d'état de l'espace, une ligne vide, puis l'aide habituelle |
| `watchoo version`, `watchoo --version` | Version de Go et plateforme (`go1.27.3 · darwin/arm64`) ; pas d'astuce | Rien |
| `watchoo init` réussi | « Espace de supervision « nom » créé » ; astuce « `watchoo help` liste les commandes » | Une ligne vide, puis le compte rendu en lignes d'état : espace, réglages, base, compte `gh` et ses droits |

- **Scripts** : la carte n'est affichée que si la sortie standard est un terminal, et jamais avec `--json`.
  Une sortie redirigée (fichier, tube) reste identique : aide sans carte, `watchoo version` sur une seule
  ligne (`watchoo v0.1.0 (go1.27.3, darwin/arm64)`), compte rendu habituel de `init`. La sortie d'erreur
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
   l'import en indiquant le numéro de ligne.
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

- `--dry-run` : fait la validation et les vérifications d'accès, affiche le bilan, mais n'enregistre rien.
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
