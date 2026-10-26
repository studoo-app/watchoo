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
| `watchoo version` | Affiche la version de Watchoo, de Go et la plateforme. Fonctionne hors d'un espace et sans `gh` | MVP |
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
