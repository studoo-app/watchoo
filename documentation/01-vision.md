# 1. Vision et objectifs

[← Sommaire](README.md)

## Le problème

Quand on suit plusieurs dépôts GitHub (projets perso, dépôts d'équipe, dépendances, organisations), il
n'existe pas de vue simple et locale pour savoir d'un coup d'œil :

- quels dépôts ont reçu de nouveaux commits ;
- quand a eu lieu la dernière activité ;
- qui contribue, et à quelle fréquence ;
- quels dépôts sont en sommeil depuis trop longtemps.

## La réponse : Watchoo

Un **binaire Go unique** qui surveille une liste de dépôts GitHub, stocke leur historique dans une base
SQLite locale, et le restitue :

- en **ligne de commande** (tableaux lisibles, sortie JSON scriptable) ;
- en **mode interactif** dans le terminal (`watchoo tui`) : dépôts, commits et contributeurs au clavier ;
- via une **interface web locale** servie par le même binaire (`watchoo serve`).

## Objectifs

| Objectif | Description | Version |
| --- | --- | --- |
| **Superviser** | Détecter les nouveaux commits sur toutes les branches suivies de chaque dépôt | MVP |
| **Attribuer** | Identifier les contributeurs (auteur, committer, compte GitHub) et leur activité | MVP |
| **Visualiser** | Tableau de bord : état de chaque dépôt, derniers commits, évolution dans le temps | MVP / V2 |
| **Alerter** | Notifier quand un dépôt est mis à jour ou reste inactif trop longtemps | V3 |

## Périmètre

- **Initial** : GitHub uniquement. **Les dépôts suivis sont principalement privés** ; les dépôts publics
  sont aussi pris en charge. Le token doit donc donner accès en lecture aux dépôts privés concernés, et les
  données locales sont traitées comme confidentielles (voir
  [Architecture](03-architecture.md#dépôts-privés)).
- **Prévu dès la conception** : ajout ultérieur d'un dépôt Git local (V2), de GitLab ou de Gitea (V3),
  grâce à une interface `Provider`. Aucun code spécifique à GitHub ne doit fuiter hors du provider GitHub.

## Espaces de supervision

Watchoo supervise des **groupes** de dépôts séparés. Chaque groupe vit dans son propre répertoire, initialisé
avec `watchoo init`, qui contient sa configuration et sa base SQLite. Exemple : un répertoire
`licence-projet` pour les projets des étudiants de licence, un autre pour ceux de master. Les données ne se
mélangent jamais. Voir [Espaces de supervision](10-espaces-de-supervision.md).

## Hors périmètre (pour l'instant)

- Outil multi-utilisateurs ou SaaS : Watchoo est un outil **personnel et local**.
- Analyse du contenu du code (qualité, sécurité, diff détaillé).
- Écriture sur GitHub : Watchoo est **en lecture seule**.

## Utilisateurs cibles

- **Enseignant qui supervise les projets de ses étudiants** : un espace par promotion ou par module
  (ex. « licence-projet »), pour voir qui travaille, quand, et qui décroche.
- Développeur ou mainteneur qui suit plusieurs projets.
- Responsable technique qui veut une vue d'activité d'une équipe ou d'une organisation.
- Toute personne voulant détecter des dépôts abandonnés ou inactifs.
