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
- `config.yaml` commence par la carte d'en-tête de Watchoo, en commentaires YAML : le hibou en petit
  format, « Watchoo » et « Réglages de l'espace de supervision », dans un cadre (le nom de l'espace n'y
  figure pas). Suivent les réglages, commentés en français ; seul le nom de l'espace est actif. Le fichier
  [`config.example.yaml`](../config.example.yaml) montre ce contenu :

  ```yaml
  # ╭─────────────────────────────────────────────────╮
  # │  █▄    ▄█  Watchoo                              │
  # │  █ ●██● █  Réglages de l'espace de supervision  │
  # │  ▀██▙▟██▀                                       │
  # ╰─────────────────────────────────────────────────╯
  #
  # Réglages de l'espace de supervision Watchoo.
  ```
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

### Seuls les espaces de l'utilisateur sont acceptés

Un enseignant clone souvent les dépôts qu'il supervise, parfois **dans** son espace
(`~/supervision/licence-projet/alice-projet/`). Un étudiant peut y avoir commité un `.watchoo/`
(`git add -f` contourne le `.gitignore`) : lancée depuis ce clone, une commande le trouverait avant le
vrai espace et lirait des réglages fournis par l'étudiant (base, `web.*`, `github.gh_path`…). Comme
le `safe.directory` de Git, Watchoo ne fait donc confiance qu'aux espaces de l'utilisateur. Sous Linux
et macOS, avant de lire les réglages d'un espace, il **refuse** :

- un `.watchoo/` qui n'appartient pas à l'utilisateur courant, ou dont les droits dépassent `0700`
  (lisible ou modifiable par le groupe ou les autres) ;
- un `.watchoo/config.yaml` dans le même cas (propriétaire différent, droits au-delà de `0600`).

`watchoo init` pose toujours ces droits ; Git, lui, ne les pose jamais (un clone crée des répertoires en
`0755` et des fichiers en `0644`), si bien qu'un `.watchoo/` venu d'un dépôt est refusé. Le message cite
le chemin et le remède : `chmod 700 …` (ou `chmod 600 …`) s'il s'agit bien de votre espace, sinon lancer
la commande avec `-C <votre espace>` (ou `WATCHOO_DIR`). Sous Windows, ce contrôle n'est pas fait.

En complément, `github.gh_path` doit être un **nom** cherché dans le `PATH` (`gh`) ou un **chemin
absolu** : un chemin relatif avec un séparateur (`./outils/gh`, `.watchoo/gh`) est refusé, car il
dépendrait du répertoire d'où la commande est lancée.

## Comportement de `watchoo init`

1. Vérifie que `gh` est installé et connecté (voir
   [Architecture](03-architecture.md#authentification--gh-imposé)).
2. Crée `.watchoo/` dans le **répertoire courant** (ou celui donné par `-C`), avec `config.yaml`,
   la base SQLite au bon schéma (migrations goose) et le `.gitignore`.
3. **Refuse** si le répertoire est déjà un espace, pour ne jamais écraser des données.
4. **Prévient** si un espace parent existe déjà (ex. `init` lancé dans `~/supervision/licence-projet/groupe-a`
   alors que `licence-projet` est un espace) : on crée alors un espace imbriqué, indépendant du parent.

Sur un terminal, le compte rendu de `init` commence par la [carte d'en-tête](05-cli.md#carte-den-tête) ;
redirigé ou avec `--json`, il reste du texte simple ou du JSON.

Le token n'est **pas** propre à l'espace : tous les espaces utilisent le compte `gh` de l'utilisateur
(ou celui défini par `github.user` dans la config de l'espace).

## Réglages : utilisateur et espace

| Niveau | Fichier | Contenu |
| --- | --- | --- |
| Utilisateur (facultatif) | `$XDG_CONFIG_HOME/watchoo/config.yaml`, sinon `~/.config/watchoo/config.yaml` | Valeurs par défaut communes à tous les espaces (fuseau, seuils, intervalle…) |
| Espace | `.watchoo/config.yaml` | Réglages propres au groupe ; ils remplacent ceux de l'utilisateur |
| Ligne de commande / environnement | options et variables `WATCHOO_*` | Remplacent tout le reste, pour une exécution |

### Partager les réglages avec d'autres superviseurs

`watchoo config export` et `watchoo config import` (CLI) ou la page Réglages (web) partagent les dépôts
suivis et les **réglages communs** d'un espace : `sync.auto`, `sync.interval`, `sync.parallelism`,
`activity.flemme_after_days`, `activity.dead_after_days`, `branches.exclude` et `timezone`. Les réglages
propres à la machine ou à la personne ne sont **jamais** partagés : `name`, `github.*` (compte `gh`,
chemin de `gh` : un fichier venu d'ailleurs ne doit jamais choisir le programme lancé) et `web.*`.
Détail : [Partage de la configuration](05-cli.md#partage-de-la-configuration--watchoo-config).

Quand un import change au moins un réglage commun, `.watchoo/config.yaml` est **réécrit** à partir du
modèle de `watchoo init` (mêmes commentaires), les réglages importés décommentés :

- `name`, `github.*` et `web.*` déjà présents dans le fichier sont gardés tels quels, ainsi que les
  réglages communs que l'import ne donne pas ;
- les commentaires personnels sont perdus ; l'ancien fichier est copié dans `.watchoo/config.yaml.bak`
  (`0600`, remplacé à chaque import) ;
- le nouveau fichier est écrit à côté en `0600`, relu, validé et comparé aux valeurs attendues avant de
  remplacer l'ancien (renommage atomique) ; en cas d'écart ou d'erreur, `config.yaml` n'est pas modifié ;
- les valeurs sont écrites comme des valeurs YAML échappées (chaînes entre guillemets) : un fuseau ou un
  motif contenant `"`, `#`, `:` ou un saut de ligne ne peut jamais créer une autre clé ;
- si les réglages importés sont déjà ceux en vigueur, le fichier n'est pas touché ; un réglage masqué
  par une variable `WATCHOO_*` est comparé à sa valeur dans les fichiers, sans la variable (la valeur
  partagée est écrite même si la variable a déjà la même valeur, et un second import ne réécrit rien) ;
- une variable `WATCHOO_*` définie garde la priorité sur la valeur écrite ; l'aperçu le signale pour
  chaque réglage importé qu'elle masque, et refuse l'import si, variables réappliquées, les réglages
  deviennent invalides ;
- le fichier utilisateur (`~/.config/watchoo/config.yaml`) n'est jamais modifié.

## Plusieurs espaces en même temps

- Chaque espace est indépendant : on peut lancer `watchoo serve` dans `licence-projet` et dans
  `master-projet` en même temps.
- Deux `serve` ne peuvent pas écouter sur le même port. Le port se règle par espace (`web.addr` dans
  `config.yaml`) ; si le port est déjà pris, `serve` s'arrête avec un message qui l'indique.
- Réglages web de l'espace (clés `web.*`) : `web.addr` (adresse d'écoute), `web.auth.username`
  (authentification basique ; le mot de passe n'est jamais dans un fichier : variable
  `WATCHOO_WEB_PASSWORD`), `web.trusted_proxy` (reverse proxy authentifiant déclaré) et
  `web.allowed_hosts` (noms publics du proxy acceptés dans l'en-tête `Host`). Deux `serve` sur
  la même machine gardent chacun leur jeton CSRF (cookie nommé avec le port).
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
