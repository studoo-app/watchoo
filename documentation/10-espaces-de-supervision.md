# 10. Espaces de supervision

[← Sommaire](README.md)

Un **espace de supervision** est un répertoire qui regroupe tout ce qui concerne **un groupe de dépôts
suivis** : sa configuration, sa base SQLite et donc toutes ses données. On peut en avoir autant qu'on veut,
complètement indépendants les uns des autres. C'est le même principe que `git init`, qui fait d'un
répertoire un dépôt Git.

## Pourquoi `watchoo init`

Watchoo sert à superviser **des groupes** distincts, qu'on ne veut pas mélanger. Exemple : un enseignant
qui suit les projets de plusieurs promotions.

```
~/supervision/
  licence-projet/        ← watchoo init : les projets des étudiants de licence
    .watchoo/
  master-projet/         ← watchoo init : les projets des étudiants de master
    .watchoo/
  perso/                 ← watchoo init : mes propres dépôts
    .watchoo/
```

Chaque espace a ses dépôts, ses contributeurs, son historique, ses réglages. Supprimer le répertoire
supprime toutes les données de ce groupe, et seulement celles-là.

## Scénario : superviser les projets des étudiants de licence

```bash
mkdir licence-projet
cd licence-projet
watchoo init                          # crée l'espace : .watchoo/ avec config et base SQLite
watchoo repo import promo-licence.csv  # ajoute d'un coup les dépôts de toute la promotion
watchoo sync                          # première récupération des commits
watchoo repo list                     # qui travaille, qui est en Flemme, qui est Dead
watchoo contributors --since 7d       # activité de chaque étudiant sur la semaine
watchoo serve                         # tableau de bord web de la promotion
```

Toutes les commandes lancées dans `licence-projet/` (ou un de ses sous-répertoires) travaillent sur
**cet espace-là uniquement**.

Le fichier `promo-licence.csv` liste un dépôt par étudiant ou par groupe, avec le nom à afficher
(format détaillé dans [CLI](05-cli.md#import-groupé--watchoo-repo-import)) :

```csv
repo,label
alice/projet-licence,Alice Martin
bob/projet-licence,Bob Durand
```

Le bilan de l'import signale les dépôts encore inaccessibles (étudiants qui n'ont pas donné l'accès).
On relance le même import plus tard, sans créer de doublon.

## Contenu d'un espace

```
licence-projet/
  .watchoo/
    config.yaml      réglages de l'espace (nom, intervalle, seuils, branches exclues…)
    watchoo.db       base SQLite : dépôts, commits, contributeurs, journal des synchros
    .gitignore       contient « * » : le contenu n'est jamais commité par erreur
```

- Le répertoire `.watchoo/` est créé avec les droits **`0700`**, et ses fichiers avec les droits **`0600`** :
  seul l'utilisateur peut les lire. Les données viennent de dépôts privés et contiennent des noms et des
  e-mails d'étudiants.
- Le `.gitignore` interne évite de pousser la base sur GitHub si l'espace se trouve dans un dépôt Git.
- **Nom de l'espace** : par défaut le nom du répertoire (`licence-projet`), modifiable dans
  `config.yaml`. Il est affiché dans le titre du web, du TUI et dans `watchoo status`.

## Comment Watchoo trouve l'espace

Comme Git, Watchoo cherche `.watchoo/` dans le **répertoire courant, puis dans ses parents**. Le premier
trouvé est utilisé.

| Cas | Comportement |
| --- | --- |
| Commande lancée dans l'espace ou un sous-répertoire | Utilise cet espace |
| Option `-C <répertoire>` (ou `--workspace <répertoire>`) | Utilise l'espace de ce répertoire, d'où qu'on lance la commande |
| Variable `WATCHOO_DIR` | Même effet que `-C`, pratique pour Docker et les scripts |
| Aucun espace trouvé | Erreur claire : « aucun espace Watchoo ici, lancez `watchoo init` » |

## Comportement de `watchoo init`

1. Vérifie que `gh` est installé et connecté (voir
   [Architecture](03-architecture.md#authentification--gh-imposé)).
2. Crée `.watchoo/` dans le **répertoire courant** (ou celui donné par `-C`), avec `config.yaml`,
   la base SQLite au bon schéma (migrations goose) et le `.gitignore`.
3. **Refuse** si le répertoire est déjà un espace, pour ne jamais écraser des données.
4. **Prévient** si un espace parent existe déjà (ex. `init` lancé dans `~/supervision/licence-projet/groupe-a`
   alors que `licence-projet` est un espace) : on crée alors un espace imbriqué, indépendant du parent.

Le token n'est **pas** propre à l'espace : tous les espaces utilisent le compte `gh` de l'utilisateur
(ou celui défini par `github.user` dans la config de l'espace).

## Réglages : utilisateur et espace

| Niveau | Fichier | Contenu |
| --- | --- | --- |
| Utilisateur (facultatif) | `$XDG_CONFIG_HOME/watchoo/config.yaml`, sinon `~/.config/watchoo/config.yaml` | Valeurs par défaut communes à tous les espaces (fuseau, seuils, intervalle…) |
| Espace | `.watchoo/config.yaml` | Réglages propres au groupe ; ils remplacent ceux de l'utilisateur |
| Ligne de commande / environnement | options et variables `WATCHOO_*` | Remplacent tout le reste, pour une exécution |

## Plusieurs espaces en même temps

- Chaque espace est indépendant : on peut lancer `watchoo serve` dans `licence-projet` et dans
  `master-projet` en même temps.
- Deux `serve` ne peuvent pas écouter sur le même port. Le port se règle par espace (`web.addr` dans
  `config.yaml`) ; si le port est déjà pris, `serve` s'arrête avec un message qui l'indique.
- Le quota GitHub est **partagé** entre tous les espaces, puisqu'ils utilisent le même compte `gh`.
  Chaque espace affiche le quota restant réel (lu dans les réponses de GitHub).

## Données personnelles des étudiants

Un espace d'enseignement contient des données personnelles (noms, e-mails, activité de chaque étudiant).

- Les données restent **sur la machine** de l'enseignant ; Watchoo n'envoie rien ailleurs que vers
  l'API GitHub.
- **Supprimer** un espace (le répertoire) supprime toutes ses données.
- `watchoo repo remove` supprime aussi l'historique du dépôt retiré, après confirmation (`--keep-history`
  pour le garder).
- _À préciser :_ durée de conservation (ex. fin de l'année universitaire) et information des étudiants,
  selon les règles de l'établissement (RGPD).
