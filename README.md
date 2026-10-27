<p align="center">
  <img src="assets/logo/watchoo-nom.svg" alt="Logo de Watchoo : un hibou géométrique" width="400">
</p>

## **Qui a travaillé, sur quoi, et quand ?** 
Watchoo surveille une liste de dépôts GitHub et vous montre
d'un coup d'œil leur activité : les nouveaux commits, les contributeurs, et les dépôts qui ne bougent plus.

> 🚧 **Projet en développement.** Les phases 1 à 4 de la [roadmap](documentation/07-roadmap.md) sont en
> place : Watchoo s'utilise en ligne de commande (`repo add`, `repo import`, `sync`, `log`,
> `contributors`, `status`…), en mode interactif (`watchoo tui`) et dans le navigateur (`watchoo serve`).
> Les binaires prêts à l'emploi arrivent avec la phase 5 ; la [documentation](documentation/README.md)
> définit l'outil complet.

## Pour qui ?

- **Enseignants** qui suivent les projets de leurs étudiants : qui avance, qui décroche.
- **Développeurs et responsables techniques** qui suivent plusieurs dépôts à la fois.

Watchoo fonctionne avec les dépôts **privés**, et toutes les données restent sur votre machine.

## Ce que fait Watchoo

- 📥 Récupère les nouveaux commits de chaque dépôt, sur toutes ses branches.
- 👥 Montre qui contribue, et combien.
- 🚦 Donne un état à chaque dépôt : **Actif** (moins de 6 jours), **Flemme** (6 à 15 jours), **Dead** (plus
  de 15 jours sans commit).
- 🔄 Se met à jour automatiquement, ou à la demande.

Trois façons de l'utiliser, avec un seul programme :

| Mode | Commande | Pour |
| --- | --- | --- |
| Ligne de commande | `watchoo log`, `watchoo repo list`… | Des réponses rapides, des scripts |
| Interactif dans le terminal | `watchoo tui` | Naviguer au clavier |
| Interface web locale | `watchoo serve` | Un tableau de bord dans le navigateur |

## Exemple : suivre une promotion d'étudiants

```bash
mkdir licence-projet && cd licence-projet
watchoo init                          # crée l'espace de supervision
watchoo repo import promo-licence.csv # ajoute les dépôts de tous les étudiants
watchoo sync                          # récupère les commits
watchoo repo list                     # qui est Actif, en Flemme ou Dead
watchoo serve                         # tableau de bord sur http://127.0.0.1:8080/
```

```
DÉPÔT                 DERNIER COMMIT     AUTEUR      ÉTAT
Alice Martin          il y a 2 h         alice       -
Bob Durand            il y a 9 j         bob         ~ 9 j
Groupe 3              il y a 42 j        dan         ⚠ 42 j
```

Chaque répertoire initialisé avec `watchoo init` est un groupe indépendant : une promotion, un module, vos
projets perso… Les données ne se mélangent jamais.

## Prérequis

- La [CLI GitHub `gh`](https://cli.github.com/), installée et connectée (`gh auth login`). Watchoo l'utilise
  pour accéder à GitHub et ne stocke jamais votre token.

## Compiler depuis les sources

Avec Go (version indiquée dans `go.mod`) :

```bash
make build    # produit bin/watchoo, à copier dans un répertoire du PATH
make check    # gofmt, go vet, tests, code sqlc et CSS à jour, compilation
```

Puis, dans le répertoire d'un groupe à superviser (ex. `~/supervision/licence-projet`) : `watchoo init`.

## En savoir plus

- [Documentation complète](documentation/README.md) : vision, fonctionnalités, architecture, roadmap.
- [AGENTS.md](AGENTS.md) : consignes pour les agents de code qui travaillent sur le projet.

Watchoo est écrit en Go : un seul binaire, sans dépendance à installer.

## Licence

Watchoo est distribué sous la [licence Apache 2.0](LICENSE). Le nom « Watchoo » et le logo du hibou n'en
font pas partie : une version modifiée et redistribuée doit porter un autre nom (voir [NOTICE](NOTICE)).
