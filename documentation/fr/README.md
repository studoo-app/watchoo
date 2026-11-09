# Documentation Watchoo

> Document de définition du projet — version du 1er novembre 2026 · Benoit
>
> Statut : **v1.0.0, version stable**. Les phases 1 à 5 (MVP) et 6 (V2) de la [roadmap](07-roadmap.md)
> sont réalisées, puis la page par contributeur (0.5.0), la mise à jour de Watchoo (0.6.0), l'interface
> bilingue, anglais par défaut et français (0.7.0), et la stabilisation de la surface publique (1.0.0). Historique des versions : [CHANGELOG.md](../../CHANGELOG.md).

Watchoo est un binaire Go unique qui surveille une liste de dépôts GitHub et répond à trois questions :
**qu'est-ce qui a bougé, quand, et qui l'a fait.** Il s'utilise en ligne de commande (CLI), en mode
interactif dans le terminal (`watchoo tui`) ou via une interface web locale servie par le même binaire.

## Sommaire

| # | Document | Contenu |
| --- | --- | --- |
| 1 | [Vision et objectifs](01-vision.md) | Pourquoi Watchoo, objectifs, périmètre, hors périmètre |
| 2 | [Fonctionnalités](02-fonctionnalites.md) | Périmètre MVP, V2, V3 |
| 3 | [Architecture technique](03-architecture.md) | Schéma d'ensemble, stack Go, organisation du code, points d'attention |
| 4 | [Modèle de données](04-modele-donnees.md) | Tables SQLite, relations, index |
| 5 | [CLI](05-cli.md) | Commandes, options globales, exemples de sortie |
| 6 | [Interface web](06-interface-web.md) | Pages, comportement HTMX, sécurité |
| 7 | [Roadmap](07-roadmap.md) | Phases de réalisation, critères de passage, critère de fin du MVP |
| 8 | [Prompt pour agent de code](08-prompt-agent.md) | Consigne prête à coller pour faire construire le projet phase par phase |
| 9 | [Stratégie de tests](09-tests.md) | Mock de `gh`, API GitHub simulée, tests par couche, CI sans service extérieur |
| 10 | [Espaces de supervision](10-espaces-de-supervision.md) | `watchoo init`, un répertoire par groupe supervisé, scénario « étudiants de licence » |
| 11 | [Skills des agents de code](11-skills.md) | Skills installés via skills.sh, organisation dans le dépôt, règles |
| 12 | [Supervision et diagnostic](12-supervision-diagnostic.md) | Détection de signaux à revoir, garde-fous, plan par phases (proposition) |
| 13 | [Conventions de projet](13-conventions.md) | Indicateurs de conformité à une charte (commits, branches, PR, SemVer, CHANGELOG, README, INDEX), revue de jalon, décisions |
| — | [Glossaire](glossaire.md) | Vocabulaire du projet |

## En une image

```mermaid
flowchart LR
    U1["Utilisateur terminal"] --> CLI["watchoo (CLI)"]
    U1 --> TUI["watchoo tui (interactif)"]
    U2["Utilisateur navigateur"] --> WEB["watchoo serve (web)"]
    CLI --> CORE["Cœur métier"]
    TUI --> CORE
    WEB --> CORE
    CORE --> DB[("SQLite")]
    CORE --> GH["API GitHub"]
```

## Principes directeurs

1. **Un seul binaire, une base par espace** : CLI, TUI et web partagent le même cœur métier ; chaque
   groupe supervisé a son propre répertoire (`watchoo init`), avec sa config et sa base SQLite.
2. **Zéro installation** : SQLite embarqué sans CGO, assets web embarqués (HTMX + DaisyUI), pas de framework ni de build JavaScript.
3. **Sobriété vis-à-vis de l'API GitHub** : synchronisation incrémentale, ETag, respect du rate limit.
4. **Extensible** : l'accès aux forges passe par une interface `Provider` (GitHub d'abord, GitLab/Gitea ensuite).
5. **Livrer par phases** : chaque phase produit quelque chose d'utilisable de bout en bout.

## Licence

Code sous [licence Apache 2.0](../../LICENSE) ; le nom et le logo restent réservés (voir [NOTICE](../../NOTICE)).
Les dépendances (cobra, viper, go-github, SQLite modernc, goose, chi, Bubble Tea, DaisyUI, HTMX, Chart.js)
sont sous des licences compatibles (Apache 2.0, MIT, BSD).
