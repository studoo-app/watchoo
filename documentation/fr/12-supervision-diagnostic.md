# 12. Supervision et diagnostic

[← Sommaire](README.md)

**Statut** : décision du mainteneur. Phases **7.1 à 7.5 réalisées** (fondations, signaux déclaratifs,
commande `watchoo diagnostic`, signaux de rythme, reprise de l'historique, revue humaine sur le web, signaux
de forme F1 à F5 ; publiées en v1.4.0, sections 9.1 à 9.5) ; phase **7.7 réalisée** (signaux absolus et
contre-indicateurs, section 9.6 ; publiée en v1.5.0) ; phase 7.6 à venir dans une version ultérieure, puis F6 (classifieur de contenu,
reporté), chacune après validation de son plan (voir [Roadmap](07-roadmap.md)). Source de la partie
signaux : la spécification « Détection des signaux de production de code assistée par IA », **version 0.2**
(3 novembre 2026, références vérifiées).

## 1. Décision

Watchoo passe d'un outil de **suivi d'activité** à un outil de **supervision et de diagnostic** des dépôts
et de leurs contributeurs. Il ne se contente plus de dire *ce qui a bougé, quand et par qui* : il signale
ce qui mérite une lecture humaine et explique pourquoi.

La première brique de diagnostic est la détection de signaux suggérant une production de code assistée par
IA (complétion, chat, agent de codage). Elle remplace, pour le périmètre de ce document, la règle « analyse
du contenu du code hors périmètre » de [la vision](01-vision.md#hors-périmètre-pour-linstant).

### Contexte d'usage (décision du mainteneur, 4 novembre 2026)

- **Contexte pédagogique** : les personnes suivies sont des **apprenants**, dans un environnement
  d'apprentissage. Le contexte salarié (Code du travail, CSE) n'est pas celui de Watchoo.
- **Finalité** : **accompagner** les apprenants, qu'ils travaillent avec ou sans IA. Le diagnostic donne au
  formateur un état des lieux et des **pistes de réflexion** sur chaque projet, à reprendre avec l'apprenant
  (« explique-moi ce commit »). Il ne sert ni à noter, ni à sanctionner, ni à détecter de la triche.
- **Accès aux résultats** : réservé au formateur qui supervise l'espace. Les apprenants n'ont pas accès
  aux signaux ni aux pistes de réflexion de Watchoo (outil local, voir [Espaces](10-espaces-de-supervision.md)).
  Un « flag » est donc, dans ce contexte, une piste de réflexion pour le formateur.

## 2. Finalité et garde-fous (non négociables)

- **Un flag n'est pas un verdict.** Il invite à regarder. Aucune décision automatique : ni notation, ni
  sanction, ni évaluation RH, ni export vers un système d'évaluation.
- **Explicabilité** : chaque flag affiche les signaux déclenchés, leurs valeurs brutes et la médiane
  personnelle. Jamais un score opaque seul.
- **Comparaison à soi-même** : les signaux de rythme comparent une personne à son propre historique.
- **Hiérarchie des preuves** : un signal déclaratif (trailer, signature, bot) pèse plus que plusieurs
  signaux statistiques.
- **Boucle d'apprentissage** : chaque décision humaine est enregistrée pour mesurer la précision des
  signaux et ajuster les seuils.
- **Transparence** : les résultats restent réservés au formateur, mais les apprenants sont informés que leurs
  dépôts font l'objet d'un diagnostic, de sa finalité (accompagnement) et des critères utilisés
  (principe 1.2.5 de la spécification ; obligation d'information du RGPD, art. 13 et 14). Les critères sont
  affichés dans l'interface.
- **Minimisation** : le message de commit est déjà stocké par Watchoo (`commits.message`, depuis la V1) ; ce
  document ne l'étend pas. Les signaux ne persistent que les **identifiants d'outils reconnus**, jamais le
  message entier. Les logs n'affichent jamais de message, d'e-mail ni de contenu de diff. Les signaux de forme
  sont calculés en mémoire, et seuls leurs résultats sont persistés. Décision à confirmer : faut-il, à terme,
  ne plus conserver `commits.message` ?
- **Périmètre d'un espace** : les flags restent dans leur espace de supervision ([Espaces](10-espaces-de-supervision.md)).
  Ils ne se mélangent jamais entre espaces.
- **Lecture seule et local** : inchangé. Aucune écriture sur GitHub, aucune donnée envoyée ailleurs.

**Hors périmètre** : détection de plagiat entre personnes, analyse de sessions de chat externes (elles ne
laissent pas de trace dans Git), décision automatique de quelque nature que ce soit.

## 3. Référentiel : production sans IA

Les signaux de rythme supposent un débit « normal » sans IA. Les ordres de grandeur publiés (Brooks 1975,
COCOMO, McConnell 2006, Jones, Cusumano et al. 2003, Prechelt 2000, Peng et al. 2023) servent de **contexte**,
jamais de seuil. Le seuil opérationnel est toujours la médiane personnelle sur une fenêtre glissante.
Points de méthode à retenir : dénominateur (le codage n'est qu'une fraction du travail), lignes nettes
(lignes qui survivent), asymétrie des distributions (médianes et statistiques robustes), langage.
Les statistiques de productivité souvent citées sans source primaire doivent être prises avec prudence
(Bossavit 2014).

## 4. Données et prétraitement

| Donnée | Source |
| --- | --- |
| Ajouts, suppressions, nombre de fichiers par commit | `git log --numstat --no-merges -M` (API ou clone local) |
| Auteur, dates | `%aI` (date d'auteur, pas de commit, sensible aux rebases) |
| Trailers et signatures | message complet, lu en mémoire puis réduit à l'outil reconnu |
| Branche et description de PR | API (pull requests déjà lues en V2) |
| Contenu des diffs (signaux F) | lecture en mémoire, jamais persistée |

Prétraitement :

1. Identités : les contributeurs déjà unifiés par Watchoo (`contributors`) servent de référence.
2. Commits de fusion exclus.
3. Renommages détectés (`-M`).
4. Fichiers exclus par motifs configurables : verrouillages (`package-lock.json`, `go.sum`…), dépendances
   embarquées (`vendor/`, `node_modules/`), fichiers générés (`*.pb.go`, `*.min.js`, `dist/`…), fichiers
   binaires.
5. Journée travaillée : seuls les jours avec au moins un commit entrent dans les médianes journalières.

Limites connues : squash merges (trailers effacés), rebases (dates réécrites), personnes qui commitent
rarement (gros commits naturels). Le signal R2 se lit à la lumière des habitudes de la personne.

## 5. Catalogue des signaux

Chaque signal a un identifiant, une définition, un seuil par défaut et un niveau de fiabilité.

### Famille D — déclaratifs (fiabilité élevée)

| ID | Signal | Déclenchement par défaut |
| --- | --- | --- |
| D1 | Trailer de co-auteur IA (`Co-authored-by:`) | Correspondance avec la liste d'outils maintenue |
| D2 | Signature dans le message ou la PR (« Generated with… ») | Correspondance avec la liste maintenue |
| D3 | Auteur bot (`*[bot]`, comptes connus) | Compte connu |
| D4 | Fichier de configuration d'agent (`AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `.github/copilot-instructions.md`…) | Présence et date d'ajout (contexte uniquement) |
| D5 | Branche nommée par un agent (`codex/`, `copilot/`, `claude/`, `cursor/`…) | Préfixe connu |

Remarque : le réglage `git.addAICoAuthor` de VS Code (1.110, février 2026) ajoute un trailer automatiquement, en trois
modes : `off`, `chatAndAgent` et `all` (défaut passé à `all` en avril 2026). Le trailer n'est ajouté que pour les
commits faits **depuis VS Code**. La présence de D1 dépend donc de l'éditeur et de la configuration. D4 indique
que l'outil est utilisé dans le dépôt, pas qu'un commit est assisté.

### Famille R — rythme (fiabilité moyenne)

Calculés par auteur, comparés à son historique sur **90 jours glissants** précédant l'événement.

Score z modifié : `z* = 0,6745 × (x − m) / MAD` (m : médiane de l'historique, MAD : écart absolu médian).
Seuil par défaut : **z\* > 3,5** (Iglewicz & Hoaglin, 1993). Si MAD = 0 ou moins de 20 observations, le signal
est marqué « historique insuffisant » et ne se déclenche pas.

| ID | Signal | Déclenchement par défaut |
| --- | --- | --- |
| R1 | Lignes nettes par jour | z\* > 3,5 |
| R2 | Taille du commit (lignes ajoutées) | z\* > 3,5 **et** > 150 lignes |
| R3 | Ratio d'ajout (ajouts / total) | > 0,95 sur un commit > 150 lignes, si la médiane personnelle est < 0,80 |
| R4 | Vitesse apparente (lignes / minutes depuis le commit précédent du même jour) | > 10 lignes/min sur un commit > 100 lignes |
| R5 | Nombre de fichiers par commit | z\* > 3,5 **et** ≥ 8 fichiers |
| R6 | Rupture de régime (médiane R1 sur 14 jours vs 90 jours précédents) | Rapport > 3 |

R4 est le signal le plus fragile : un copier-coller légitime peut le déclencher. Il reste en « à regarder »
tant qu'il n'a pas été calibré.

### Famille F — forme (fiabilité faible)

Indices d'appoint, **ne déclenchent jamais un flag seuls**.

| ID | Signal | Déclenchement par défaut |
| --- | --- | --- |
| F1 | Densité de commentaires (lignes de commentaire / lignes ajoutées) | z\* > 3,5 |
| F2 | Docstrings systématiques | > 90 % alors que la médiane personnelle est < 30 % |
| F3 | Diversité lexicale des identifiants | z\* < −3,5 |
| F4 | Style des messages (longueur, puces, conventional commits) | Changement brusque sur les 20 messages précédents |
| F5 | Tests livrés dans le même commit que le code | < 10 % des commits historiques de la personne |
| F6 | Classifieur de contenu (optionnel, **désactivé par défaut**) | Probabilité > 0,9 |

F6 a un poids faible : les détecteurs existants généralisent mal (Suh et al., 2025). Il est **reporté** à une
étape ultérieure (section 9) : il demanderait un modèle externe, donc une dépendance hors de la pile
documentée. F1 à F5 sont réalisés en phase 7.5 (règles précises en section 9.5).

## 6. Niveaux, score et sortie

| Niveau | Règle par défaut | Lecture |
| --- | --- | --- |
| **Déclaré** | Au moins un signal D1, D2, D3 ou D5 sur le commit | Usage attesté par les traces de l'outil |
| **Probable** | ≥ 2 signaux R **et** ≥ 1 signal F, sans contre-indicateur H (A1 à A4 comptent comme R, A5 à A7 comme F ; phase 7.7) | Faisceau d'indices convergents |
| **À regarder** | 1 signal R, ou ≥ 2 signaux F | Anomalie isolée, souvent explicable |

D4 ne déclenche aucun niveau seul : il est affiché en contexte. Le score pondéré (D = 10, R = 3, F = 1,
F6 = 0,5) sert seulement au tri, et sera recalibré à partir des revues humaines.

**Score d'un dépôt** (1.6.0, page de détail du dépôt seulement) : la somme des poids des signaux de toutes
les pistes du dépôt, chaque signal déclenché compté une fois par piste : D1, D2, D3, D5 = 10 ; R1 à R6 et A1
à A4 = 3 ; F1 à F5 et A5 à A7 = 1 ; H1 à H4 et D4 = 0 (contexte ou contre-indicateur). Il n'est jamais affiché
seul : toujours à côté du nombre de pistes par niveau et du décompte de chaque signal avec ses points, et du
rappel qu'une piste n'est jamais un verdict. Il ne classe rien : pas de colonne de scores entre dépôts, pas
d'export, pas d'usage dans une évaluation (section 2).

**Score IA d'un contributeur** (1.6.0, liste des contributeurs, décision du mainteneur) : même calcul sur les
pistes dont il est l'auteur, tous dépôts confondus (commit et jour : le contributeur auteur ; pull request : son
login de forge). Il remplace la colonne « Signatures d'IA » de la liste, et la carte « Signatures d'IA » de la page du
contributeur (même score), quand le diagnostic est activé (« — » sinon). Il est présenté par l'icône de son palier (`diagnostic.Tier`) : **rien** à 0 (aucune piste), **moyen**
de 1 à 29, **élevé** dès 30 (3 signaux déclarés, ou 10 signaux de rythme) ; pour moyen et élevé, le score est
écrit au centre de l'icône, à 0 l'icône est affichée seule ; le nombre de pistes par niveau est dans l'infobulle (et le texte de remplacement nomme le palier). La liste reste triée par
commits : jamais par ce score. Ce n'est pas une note : il invite à ouvrir les pistes, comme le score d'un dépôt.

Sortie JSON proposée (`watchoo diagnostic --json`, nom à valider) :

```json
{
  "commit": "a1b2c3d",
  "auteur": "e.martin",
  "date": "2026-10-14T16:42:00+02:00",
  "niveau": "probable",
  "score": 7,
  "signaux": [
    {"id": "R1", "valeur": 412, "mediane_perso": 35, "z_modifie": 6.1},
    {"id": "F2", "valeur": 0.94, "mediane_perso": 0.12}
  ],
  "contexte": {"agent_configure_depuis": null, "fichiers_exclus": ["package-lock.json"]},
  "revue": null
}
```

## 7. Revue humaine

Fiche de revue (CLI et interface web) : niveau, signaux avec valeurs brutes et médiane personnelle, diff
du commit et des voisins (± 3), graphique du débit de la personne sur 90 jours avec le point signalé,
contexte (période, fichiers exclus, configuration d'agent).

Décision enregistrée : `assisté` · `non assisté` · `indéterminé` ; cause si non assisté (`copier-coller
interne`, `code généré par outil`, `import de code existant`, `refactoring`, `autre`) ; confiance (faible,
moyenne, élevée) ; note libre ; relecteur et date.

Exploitation : précision par signal, recalibrage des poids (régression logistique sur les décisions
`assisté` / `non assisté`, `indéterminé` exclu), ajustement des seuils (précision / rappel), accord entre
relecteurs (κ de Cohen sur un échantillon commun), alimentation de la liste d'exclusion de fichiers.

## 8. Impacts techniques (à reporter au moment de l'implémentation)

- **Données** : nouvelles tables pour les statistiques par commit, les flags et les revues. Les tables et les
  champs seront décrits dans [Modèle de données](04-modele-donnees.md) avant migration (goose, sqlc). Le
  contenu des commits n'est pas persisté.
- **Durée de conservation** : à fixer par espace, avec purge. Valeur par défaut à proposer au mainteneur.
- **Provider** : les lectures de diff passent par l'interface `Provider` (GitHub ou clone local). Elles
  consomment du quota API, à traiter comme la synchro (ETag, rate limit, incrémental).
- **Interface** : pages et commandes en FR et EN, chaque texte passe par `internal/i18n`.
- **Surface publique** : nouvelle commande, nouvelles clés `--json` et nouveaux réglages. Ajout = MINOR, mais
  un changement incompatible ensuite = MAJOR ([Skills](11-skills.md#versions-et-changelogmd)).

## 9. Plan par phases (proposition)

Chaque phase suit la règle du projet : code, tests sans service extérieur (dépôts de test `gittest`), `go vet`,
résumé de ce qui marche. Le plan et l'arborescence de chaque phase sont proposés avant le code.

- **7.1 Fondations** (réalisée) : détaillée en section 9.1. Livrable : `watchoo sync` alimente la table des
  signaux par commit, activée par espace, sans rien changer à l'affichage ni aux sorties publiques existantes.
- **7.2 Déclaratif** (réalisée) : signaux D1 à D5 (D2 pour les descriptions de PR et D5 en plus de 7.1), niveau
  « Déclaré », sortie `watchoo diagnostic`. Livrable : flags déclaratifs visibles et expliqués en CLI.
- **7.3 Rythme** (réalisée) : signaux R1 à R6, fenêtre de 90 jours, z modifié, niveau « À regarder »
  (« Probable » attend la phase 7.5), reprise de l'historique. Livrable : pistes statistiques, historique
  insuffisant signalé.
- **7.4 Revue** (réalisée) : fiche de revue en CLI et sur le web, enregistrement des décisions, graphique de débit.
  Livrable : chaque flag peut être étiqueté.
- **7.5 Forme** (réalisée) : signaux F1 à F5, niveau « Probable ». Livrable : indices d'appoint, jamais seuls.
  Détaillée en section 9.5.
- **7.6 Calibration** : précision par signal, recalibrage, κ. Livrable : rapport de calibration, seuils
  documentés. Démarre quand un volume de revues suffisant existe ; **version ultérieure** (décision du
  mainteneur, 4 novembre 2026) : les phases 7.1 à 7.5 sortent sans elle, seuils de la spécification.
- **7.7 Signaux absolus, marqueurs de travail et contre-indicateurs** (réalisée ; plan validé le 4 novembre
  2026, section 9.6) : couvrir les projets rendus en peu de commits, que les signaux R et F
  (comparaison à soi-même, 20 observations) ne voient pas. Indépendante de 7.6 : peut sortir avant elle.
- **7.8 Lignes en cause à l'affichage** (réalisée ; option « extraire à l'affichage, sans rien garder »
  choisie par le mainteneur le 4 novembre 2026) : la fiche de revue cite, sous chaque signal, les lignes
  ajoutées qui l'ont déclenché. Détaillée en section 9.7.
- **Étape ultérieure — F6, classifieur de contenu** (reporté par le mainteneur le 4 novembre 2026) : optionnel,
  désactivé par défaut, poids faible (0,5). Il demande un modèle de détection (type GPTSniffer ou
  DetectCodeGPT, références 17 à 19), donc une dépendance hors de la pile documentée, soumise à l'accord du
  mainteneur ; il serait calculé en local, sans envoyer de code ailleurs. À reconsidérer après la
  calibration (7.6), si les revues montrent que F1 à F5 ne suffisent pas.

## 9.1 Phase 7.1 — fondations (réalisée)

**Objectif** : collecter, par commit, les données nécessaires aux signaux, sans affichage ni décision.
Plan validé par le mainteneur le 3 novembre 2026 : table séparée, désactivé par défaut, 365 jours,
`commits.message` conservé tel quel, D2 (description de PR) reporté en 7.2.

**Point de départ** (dans le code) : `commits` stockait déjà `message`, `additions`, `deletions` et
`stats_known` ; les statistiques viennent du clone Git (`CommitStatsBatch`, numstat par paquets) ou de
l'API (`CommitStats`, en repli). Il manquait le détail des fichiers, les lignes après exclusion et les
identifiants d'outils.

**Ce qui est livré** :

```
internal/diagnostic/       paquet métier, sans log ni dépendance vers cli, tui, web
  rules.go                 listes D1 (trailers), D2 (signatures), D3 (bots), D4 (fichiers d'agent),
                           motifs d'exclusion par défaut
  exclude.go               Excluder : motifs « nom/ » (répertoire), nom de fichier, chemin complet
  compute.go               Compute : lignes et fichiers après exclusion, outils, bot, fichier d'agent
internal/provider/         CommitStats.Files (chemin, lignes, binaire), rempli par le clone Git seulement
internal/store/            migration 00007_commit_signals.sql, queries/diagnostic.sql, diagnostic.go
internal/sync/signals.go   calcul après ApplyRepoSync, purge en fin de synchro
internal/workspace/        réglages diagnostic.enabled, diagnostic.exclude, diagnostic.retention_days
```

**Règles retenues** :
- **Désactivé par défaut** (`diagnostic.enabled: false`) : rien n'est calculé ni purgé.
- **Nouveaux commits seulement** : les commits synchronisés après l'activation, dont le clone Git local
  donne le détail des fichiers. L'historique antérieur n'est pas mesuré ; le repli sur l'API sans clone
  ne donne pas de signal (la réponse de l'API n'est lue qu'avec `per_page=1`, pour ne pas télécharger le
  code des dépôts privés).
- **Commits de fusion exclus** (`--no-merges` de la spécification, section 3.2).
- **Fichiers binaires toujours exclus** ; motifs par défaut : spécification, section 3.2. Syntaxe des motifs
  (`path.Match`) : `nom/` un répertoire n'importe où (jokers acceptés, ex. `*.egg-info/`), `a/b/` un
  sous-répertoire n'importe où, `/nom/` un répertoire à la racine seulement, `motif` un nom de fichier,
  `chemin/motif` ou `/fichier` un chemin depuis la racine.
- **Renommages détectés** (`-M`, spécification, section 3.2) : un fichier déplacé compte pour ses seules
  lignes modifiées (0 pour un simple déplacement), comme dans les statistiques de GitHub.
- **Trailer D1 de Claude** : reconnu à l'adresse `noreply@anthropic.com` seulement, jamais au nom (Claude est
  aussi un prénom).
- **Signaux mesurés** : D1 (trailers `Co-authored-by:`), D2 pour la partie message de commit, D3 (auteur
  bot), D4 (le commit **modifie** un fichier de configuration d'agent ; la date d'ajout viendra avec
  l'affichage du contexte). D5 (nom de branche de PR) et la partie description de PR de D2 viennent en 7.2.
- **Écriture unique** : une ligne n'est jamais réécrite. Changer `diagnostic.exclude` ne s'applique
  qu'aux commits suivants ; une commande de recalcul est reportée.
- **Purge** en fin de chaque synchro : lignes dont la date d'écriture dépasse `diagnostic.retention_days`.
- **Jamais partagé** : `watchoo config export` / `import` ignorent `diagnostic.*`, et la réécriture de
  `config.yaml` après un import les garde.
- **Échec isolé** : un échec d'écriture des signaux devient un avertissement de la synchro du dépôt, sans
  la faire échouer.

**Tests** : voir [Tests](09-tests.md#supervision-et-diagnostic-phase-7).

**Reste pour l'affichage** (phases suivantes) : la commande `watchoo diagnostic`, les niveaux, la revue
humaine. Aucune sortie publique existante (`log`, `status`, `--json`) n'a changé.

## 9.2 Phase 7.2 — signaux déclaratifs et première sortie (réalisée)

Plan validé par le mainteneur le 4 novembre 2026 : seuls les outils détectés sont gardés (jamais le texte
d'une description de PR), clés `--json` en anglais comme le reste de Watchoo, sortie en CLI seulement
(web et TUI en 7.4, avec la fiche de revue).

- **Signal précisé** : chaque outil reconnu est gardé avec le signal qui l'a trouvé (`D1:claude` pour un
  trailer, `D2:unnamed` pour une signature), pour que chaque piste dise d'où elle vient.
- **D2 sur les PR** : la description d'une PR est lue en mémoire par le provider GitHub ; seuls les
  identifiants d'outils sont gardés (`pull_requests.tool_ids`, migration `00012`). Pour les PR lues après la
  mise à jour, diagnostic activé ; les clones locaux n'ont pas de PR.
- **D5** : préfixe de la branche d'une PR (`codex/`, `copilot/`, `claude/`, `cursor/`), lu dans
  `pull_requests.head_branch`, déjà en base.
- **D4 en contexte** : date du premier ajout d'un fichier de configuration d'agent dans le dépôt
  (`git log --diff-filter=A` sur le clone), gardée dans `repositories.agent_config_since` (migration
  `00012`) et affichée comme contexte, sans niveau.
- **Niveau « Déclaré »** (`declared`) : au moins un signal D1, D2, D3 ou D5 ; calculé par
  `internal/diagnostic`, avec la liste des signaux et de leurs valeurs.
- **Commande `watchoo diagnostic [--repo X] [--author Y] [--since 30d]`** (texte et `--json`), réservée au
  formateur : pistes de réflexion du plus récent au plus ancien (commits et PR), signaux et valeurs, contexte
  D4 par dépôt, rappel « piste de réflexion, pas un verdict ». Diagnostic désactivé : la commande le dit, sans
  erreur.
- **Purge** : les identifiants d'outils des PR suivent aussi `diagnostic.retention_days` (date d'ouverture).

## 9.3 Phase 7.3 — signaux de rythme et reprise de l'historique (réalisée)

Plan validé par le mainteneur le 4 novembre 2026 : seuils de la spécification sans calibration jusqu'à la
phase 7.6 ; une commande de reprise de l'historique déjà cloné (le bouton web vient avec les pages web du
diagnostic, en 7.4) ; R4 désactivé par défaut.

- **Signaux R1 à R6** (section 5), calculés à la lecture par `internal/diagnostic` à partir de
  `commit_signals` (aucune nouvelle table) : score z modifié, seuil 3,5, fenêtre de 90 jours. L'historique
  est celui de l'auteur **dans le dépôt** (un projet d'apprenant par dépôt) ; un jour est un jour du fuseau
  d'affichage de l'espace.
- **Historique insuffisant** : moins de 20 observations, ou MAD nul : le signal n'est pas déclenché, et la
  sortie dit combien d'auteurs sont dans ce cas.
- **R6** compare la médiane R1 des 14 derniers jours (au moins 3 jours travaillés) à celle des 90 jours
  précédents (au moins 20 jours travaillés, médiane positive) ; seul le premier jour d'une rupture est
  signalé.
- **R4** (vitesse apparente) n'est calculé qu'avec le réglage `diagnostic.speed_signal: true` (défaut
  `false`) ; un commit à la même minute que le précédent n'est pas mesuré.
- **Niveaux** : « Déclaré » garde la priorité ; sinon un ou plusieurs signaux R donnent « À regarder »
  (`to_review`). « Probable » demande un signal de forme (phase 7.5).
- **Pistes** : par commit (R2 à R5) ou par jour (R1, R6 : `kind` `day`) ; chaque signal donne sa valeur
  brute, la médiane personnelle et le score z.
- **Reprise de l'historique** : `watchoo diagnostic backfill [--repo X]` mesure, sur les clones locaux, les
  commits déjà en base sans signaux et plus récents que la durée de conservation, hors fusions. Un clone
  existant est lu tel quel, sans `git fetch` (la reprise peut tourner pendant la synchro de `watchoo serve`) ;
  un clone absent est d'abord créé.
- **Auteur** : celui du commit, relu à chaque calcul (une fusion de contributeurs garde l'historique de
  l'apprenant).
- **Conservation** : aucune piste n'est montrée au-delà de `diagnostic.retention_days`, pistes de PR
  comprises (leur branche, D5, reste en base avec la PR). Le filtre `--since` garde un jour à cheval sur la
  borne.

## 9.4 Phase 7.4 — revue humaine sur le web (réalisée)

Plan validé par le mainteneur le 4 novembre 2026 : revues dans une table `diagnostic_reviews` soumise à la
durée de conservation des signaux ; revue sur le web seulement (la CLI reste en lecture) ; un seul
relecteur (outil mono-utilisateur) : le compte de l'interface web s'il existe, sinon le nom de l'espace.

- **Page « Diagnostic »** (`/diagnostic`, dans la navigation quand le diagnostic est activé) : rappel
  « pistes de réflexion, pas des verdicts », pistes de la plus récente à la plus ancienne avec leur niveau,
  leurs signaux et l'état de leur revue, filtre par dépôt, historiques insuffisants, contexte D4, et bouton
  de **reprise de l'historique** (lancée en arrière-plan, une à la fois).
- **Fiche de revue** (`/diagnostic/lead?key=…`) : niveau, signaux avec valeur brute, médiane personnelle
  et score z ; pour un commit, son **diff lu sur le clone local** (jamais gardé ni journalisé, tronqué au-delà
  de 200 Ko) et les commits mesurés voisins de l'auteur (± 3) ; **graphique du débit** de l'auteur sur 90
  jours (lignes ajoutées par jour) avec le jour signalé ; contexte D4 ; lien vers la forge.
- **Décision** (section 6.2) : `assisted`, `not_assisted` ou `undetermined` ; cause si non assisté
  (`internal_copy`, `tool_generated`, `existing_import`, `refactoring`, `other`) ; confiance (`low`,
  `medium`, `high`) ; note libre (2 000 caractères au plus) ; relecteur et date automatiques. Une piste a
  au plus une revue, remplacée si elle est refaite ; les signaux présents lors de la revue sont gardés
  (calibration de la phase 7.6).
- **Purge** : une revue suit la durée de conservation, à partir de la date de sa piste.

## 9.5 Phase 7.5 — signaux de forme (réalisée)

Plan validé par le mainteneur le 4 novembre 2026 : seules les **mesures** sont gardées, jamais le code (à
revoir seulement si la calibration en montre le besoin) ; F6 reporté (section 9).

- **Mesure** : à la synchro (et par la reprise de l'historique), les **lignes ajoutées** de chaque commit
  mesuré sont lues en flux sur le clone local (`git diff-tree -p -U0 -M`, premier parent, fichiers binaires
  et motifs de `diagnostic.exclude` écartés) et analysées **en mémoire** (`internal/diagnostic`,
  `FormMeter`), sans analyse syntaxique : reconnaissance ligne à ligne des commentaires, des définitions de
  fonctions et des identifiants, pour Go, JavaScript/TypeScript (Vue, Svelte), PHP, Java, Kotlin, C#, Scala,
  Swift, Rust, Dart, C/C++, Python, Ruby, scripts shell, SQL, HTML/Twig/XML et CSS. Une ligne de plus de
  1 000 caractères (code minifié) est ignorée. Le message du commit est mesuré en mémoire lui aussi.
- **Gardé** (11 colonnes de `commit_signals`, migration `00014`) : nombres seulement — lignes source,
  lignes de commentaire, définitions et définitions documentées, identifiants et leur diversité lexicale,
  lignes de test, longueur du message, lignes en liste, sujet Conventional Commits. Ni ligne de code, ni
  identifiant, ni chemin, ni message. `form_known` distingue une forme mesurée d'une ligne d'avant la phase
  7.5 ou d'une lecture en échec ; la reprise de l'historique complète ces lignes.
- **Signaux** (calculés à la lecture, comme R1 à R6 ; historique de l'auteur dans le dépôt, 90 jours, au
  moins 20 observations, commits dont la forme est mesurée seulement) :

| ID | Mesure | Déclenchement |
| --- | --- | --- |
| F1 | Lignes de commentaire / lignes source ajoutées (commits d'au moins 20 lignes source) | z\* > 3,5 |
| F2 | Définitions documentées / définitions (commentaire juste avant, docstring juste après en Python) | ≥ 3 définitions, part > 90 %, médiane personnelle < 30 % |
| F3 | Diversité lexicale des identifiants (hors mots-clés et chaînes) : MATTR, fenêtre de 50 identifiants (Covington et McFall, 2010) | z\* < −3,5 |
| F4 | Longueur du message (hors trailers et signatures) par rapport aux 20 messages précédents | z\* > 3,5 **et** une nouveauté absente des 20 messages : liste d'au moins 2 lignes, ou sujet Conventional Commits |
| F5 | Lignes de test livrées avec au moins 20 lignes de code (fichiers `*_test.go`, `test_*.py`, `*.spec.ts`, `*Test.java`, répertoires `tests/`, `__tests__/`…) | ≥ 5 lignes de test, alors que moins de 10 % des commits de code de l'historique en ont |

- **Choix de mise en œuvre** (la spécification laisse ces points ouverts) : MATTR plutôt que le simple
  rapport « identifiants distincts / identifiants », qui baisse mécaniquement avec la taille du commit ; F4
  exige une longueur inhabituelle **et** une nouveauté de forme, pour qu'un apprenant qui adopte
  Conventional Commits après un cours ne soit pas signalé sur ce seul changement ; F5 compare à une part
  (pas à une médiane) ; un MAD nul ne donne pas de score, comme pour R1 à R6.
- **Niveaux** (section 6) : « Probable » (`probable`) pour au moins 2 signaux R et 1 signal F ; « À
  regarder » pour 1 signal R ou au moins 2 signaux F ; **un signal F seul ne lève jamais de piste**. Les
  signaux F portent sur un commit : ils se combinent avec R2 à R5 du même commit, pas avec R1 et R6 (pistes
  par jour). « Déclaré » garde la priorité ; les signaux F présents s'affichent alors avec les autres.
- **Affichage** : chaque signal F donne sa valeur brute, la médiane personnelle (F5 : la part habituelle des
  commits avec tests) et, pour F1, F3 et F4, le score z ; libellés en anglais et en français.

## 9.6 Phase 7.7 — signaux absolus, marqueurs de travail et contre-indicateurs (réalisée)

**Origine** : le référentiel « Signaux de détection d'usage IA dans les repos étudiants » (agent
AGENT_DETECTION_IA, 5 pages, 8 catégories, transmis par le mainteneur le 4 novembre 2026 ; document interne,
référence 34). Ses seuils viennent de l'expérience d'enseignants, **sans validation empirique** : ils sont
repris comme valeurs de départ, à calibrer (7.6). Mêmes garde-fous que le reste du diagnostic (section 2).

**Constat** (comparaison avec la 1.4.0) : le référentiel juge un projet avec des **seuils absolus** (plus de
500 lignes dans un commit, aucun TODO sur 800 lignes…) ; Watchoo compare l'apprenant à **son propre
historique** (au moins 20 observations en 90 jours). Un projet rendu en 1 à 5 commits, cas typique du
référentiel, ne lève donc aucune piste R ni F. Le référentiel tempère aussi par des **contre-indicateurs**
(traces de travail humain), que la 1.4.0 n'a pas.

### Nouvelles mesures (en mémoire, nombres seulement)

Ajoutées au `FormMeter` (même lecture des lignes ajoutées que F1 à F5) et au style du message, gardées dans
`commit_signals` (migration `00015`). Ni ligne, ni mot, ni chemin gardé.

| Mesure | Définition |
| --- | --- |
| `decorative_lines` | Lignes de commentaire décoratives : au moins 8 caractères de remplissage consécutifs (`─ ━ ═ = - * ~ #`), seuls ou encadrant un titre (`// ── Section ──`) |
| `fr_words`, `fr_words_unaccented` | Dans les commentaires, mots d'une liste fixe de mots français accentués courants (« données », « accès », « méthode », « élément », « requête », « paramètre »…), comptés avec et sans leurs accents (« donnees ») |
| `wip_markers` | Marqueurs de travail en cours : `TODO`, `FIXME`, `XXX`, `HACK` en commentaire ; lignes de debug (`print(` en Python, `var_dump(`, `dd(`, `dump(`, `console.log(`, `fmt.Println(` hors `main`… liste par langage) |
| `short_identifiers` | Identifiants d'une lettre (`$i`, `j`, `k`, `x`…) parmi les identifiants déjà comptés pour F3 |
| `private_defs` | Définitions privées parmi les définitions (`private function`, `private` en Java/C#/Kotlin, `def _nom` en Python, `#nom` en JS) |
| `message_revert`, `message_fix` | Message de la forme `Revert "…"` (git revert) ; sujet de correction (`fix`, `corrige`, `correction`, `oups`…) |

### Famille A — signaux absolus (sans historique personnel)

| ID | Signal | Déclenchement par défaut (référentiel) | Compte comme |
| --- | --- | --- | --- |
| A1 | Commit massif | ≥ 500 lignes source ajoutées (fichiers retenus) | R |
| A2 | Dépôt rendu « en une fois » | L'auteur a au plus 3 commits mesurés dans le dépôt, et l'un d'eux apporte ≥ 80 % de ses lignes source ajoutées (et au moins 200) | R |
| A3 | Poussée après un silence | ≥ 300 lignes source ajoutées après au moins 14 jours sans commit de l'auteur dans le dépôt | R |
| A4 | Revert puis remplacement | Commit `Revert "…"` suivi, dans l'heure, d'un commit de l'auteur d'au moins 100 lignes source ajoutées (sans vérifier les fichiers : les chemins ne sont pas gardés) | R |
| A5 | Commentaires décoratifs | ≥ 3 lignes décoratives dans le commit | F |
| A6 | Commentaires français sans accents | ≥ 10 mots repérés dans le commit, ≥ 90 % sans accents | F |
| A7 | Code « dans son état final » | Sur l'ensemble de l'auteur dans le dépôt : ≥ 800 lignes source ajoutées, aucun marqueur de travail et aucun identifiant d'une lettre | F |

A1 à A4 comptent comme des signaux de rythme et A5 à A7 comme des signaux de forme pour les niveaux
(section 6) : un dépôt rendu en un commit de 600 lignes (A1, A2) avec des séparateurs décoratifs (A5) donne
« Probable ». A2 et A7 portent sur l'ensemble de l'auteur dans le dépôt ; ils sont **rattachés à son commit le
plus gros** (lignes source ajoutées), pour que la même piste réunisse les indices d'un dépôt rendu d'un coup.

**Mise en œuvre** : les mesures sont calculées à la synchro et par la reprise de l'historique ; les signaux A
et H à la lecture, comme R et F. La migration `00015` remet `form_known` à 0 : les commits mesurés avant la
phase 7.7 n'ont pas encore ces mesures, la reprise de l'historique les complète. A1 à A7 ne comptent que les
commits dont la forme est mesurée ; A3 demande un commit précédent de l'auteur dans le dépôt ; H3 (squelette)
tempère toutes les pistes de l'auteur dans le dépôt ; H4 compte les corrections des 14 jours qui précèdent le
commit.

### Famille H — contre-indicateurs (traces de travail humain)

| ID | Indice | Repère |
| --- | --- | --- |
| H1 | Marqueurs de travail | Au moins un marqueur dans le commit (ou le projet, pour une piste `project`) |
| H2 | Variables courtes | Au moins 3 identifiants d'une lettre |
| H3 | Squelette initial | Premier commit de l'auteur : au moins 5 fichiers et au plus 2 lignes ajoutées par fichier en moyenne |
| H4 | Corrections itératives | Au moins 3 commits de correction (`message_fix`) de l'auteur dans les 14 jours qui précèdent |

Un contre-indicateur **ne supprime jamais une piste** : il l'**abaisse d'un niveau** (« Probable » →
« À regarder » ; « À regarder » reste « À regarder ») et s'affiche à côté des signaux, avec la mention
« trace de travail humain ». Il ne touche jamais « Déclaré ».

### Hors périmètre de 7.7 (avec la raison)

- **Documentation** (README impeccable, décalage doc/code) et **CSS** (palettes, polices, chaînes de classes
  Bootstrap) : demanderaient de garder ou de comparer des contenus, pas seulement des nombres.
- **Homogénéité stylistique, rupture de convention** (français → anglais, MySQLi → PDO), **pages
  quasi identiques**, **architecture trop mature** : demandent une analyse syntaxique ou de garder des
  empreintes de fichiers ; à reconsidérer après calibration.
- **Bonnes pratiques hors programme** : dépendent du programme du cours (liste par espace possible plus
  tard).
- **Cohérence entre projets d'un même apprenant** (palette identique, saut de niveau, même faille) : demande
  de croiser des dépôts ; le croisement entre personnes reste hors périmètre (section 2).

### Décisions (validées par le mainteneur le 4 novembre 2026)

1. Seuils du référentiel repris tels quels (500, 80 %, 14 jours, 90 %, 800), non calibrés jusqu'à 7.6.
2. Nature de piste `project` : validée, puis **abandonnée à la mise en œuvre**. Une piste `project` ne
   portant que A7 (un signal de forme) n'aurait jamais été affichée ; A2 et A7 sont rattachés au commit le plus
   gros de l'auteur, sans nouvelle nature ni migration de `diagnostic_reviews`.
3. Contre-indicateurs : abaissent d'un niveau, ne suppriment jamais une piste.
4. A4 sans recouvrement de fichiers (les chemins ne sont pas gardés).
5. Liste des mots français accentués et des appels de debug : fixe, dans le code, documentée ; pas de réglage.
6. Ordre : 7.7 avant 7.6 (elle ne dépend pas des revues), dans une version MINEURE (ajouts de signaux et de
   colonnes ; aucune clé `--json` retirée).

## 9.7 Phase 7.8 — lignes en cause à l'affichage (réalisée, publiée en v1.6.0)

Décision du mainteneur (4 novembre 2026), entre deux options : **A. extraire à l'affichage, sans rien
garder** (retenue) ou B. garder des extraits en base. La minimisation (section 2) est inchangée : aucune ligne
de code n'est écrite en base ni dans les logs.

- **Lecture** : à l'ouverture de la fiche d'une piste de commit, Watchoo relit les lignes ajoutées du commit
  sur le clone local existant (sans `fetch`, comme le diff), refait la mesure de forme et note, par signal, les
  lignes qui la déclenchent, avec leur fichier et leur numéro dans le fichier après le commit.
- **Signaux qui citent des lignes** : F1 (commentaires), F2 (définitions documentées), F5 (premier ajout de
  chaque fichier de test), A5 (lignes décoratives), A6 (commentaires français sans accents), H1 (marqueurs de
  travail), H2 (variables d'une lettre). Les autres (rythme, message, signaux sur le commit entier comme A1, A2,
  A7) renvoient au diff.
- **Affichage** : section « Lignes en cause » de la fiche, un bloc par signal de la piste, 20 lignes au plus par
  signal, puis « et N autres lignes ». Sans clone : « indisponible », comme le diff.
- **Limite** : un commit absent du clone (historique réécrit, branche non suivie) n'a pas de lignes en cause.

## 10. Points ouverts (à trancher par le mainteneur)

1. **Information des apprenants** : texte et moment (avant la première analyse : présentation du module,
   charte du cours…). Ce qui est communiqué : l'existence du diagnostic, sa finalité, ses critères ; jamais
   les résultats individuels, réservés au formateur.
2. **Durée de conservation** par défaut, et purge.
3. **Contexte d'usage** : **tranché**, pédagogique (section 1). Le RGPD suppose une validation par le DPO de
   l'établissement (registre des traitements, durée de conservation). Pour mémoire, un usage en entreprise
   relèverait d'autres règles (Code du travail, art. L1222-4, L2312-37 et L2312-38 ; Cass. soc., 11 déc. 2019,
   n° 18-11.792) et n'est pas prévu.
4. **Présentation aux apprenants** : **tranché**, outil d'accompagnement et de dialogue (« explique-moi ce
   commit »), pas détecteur de triche.
5. **Références** : celles de la spécification v0.2, section 8, toutes vérifiées le 3 novembre 2026. Les sources
   secondaires y sont signalées ; McConnell (2006) reste à vérifier dans le livre.
6. **Contenu du code** : **tranché** le 4 novembre 2026. Les lignes en cause sont extraites à l'affichage, sans
   rien garder (phase 7.8, section 9.7). Garder des extraits en base (consultables sans clone) reste écarté ;
   à reconsidérer seulement si la calibration (7.6) le demande.

## 11. Références

Références de la spécification v0.2 (section 8), vérifiées le 3 novembre 2026. Les numéros servent de renvoi depuis
les autres sections. Les sources secondaires sont signalées.

### 11.1 Algorithmes et méthodes

- **Score z modifié et seuil de 3,5** (signaux R, section 5) : Iglewicz, B., Hoaglin, D. C. (1993). *How to Detect
  and Handle Outliers*. ASQC Quality Press (vol. 16). NIST/SEMATECH, *e-Handbook of Statistical Methods*, 1.3.5.17.
  https://itl.nist.gov/div898/handbook/eda/section3/eda35h.htm
- **Régression logistique et courbes précision / rappel** (section 6.3) : méthodes standard, sans référence propre
  dans la spec.
- **κ de Cohen** (accord entre relecteurs, section 6.3) : méthode standard, sans référence propre dans la spec.
- **MATTR, moving-average type-token ratio** (F3, section 9.5) : Covington, M. A., McFall, J. D. (2010). « Cutting the
  Gordian Knot: The Moving-Average Type–Token Ratio (MATTR) ». *Journal of Quantitative Linguistics*, 17(2), 94-100.
  DOI : 10.1080/09296171003643098. *(Ajoutée en phase 7.5 : diversité lexicale indépendante de la longueur.)*
- **Conventional Commits 1.0.0** (F4) : https://www.conventionalcommits.org/fr/v1.0.0/ ; **trailers Git** (retirés de la
  longueur du message) : documentation de `git interpret-trailers`, https://git-scm.com/docs/git-interpret-trailers

### 11.2 Productivité des développeurs (sans IA) : section 3

1. Brooks, F. P. (1975 ; éd. 1995). *The Mythical Man-Month*. Addison-Wesley. ISBN 0-201-00650-2 et 0-201-83595-9.
2. Jones, C. (2008). *Applied Software Measurement* (3e éd.). McGraw-Hill.
3. Jones, C., Bonsignour, O. (2011). *The Economics of Software Quality*. Addison-Wesley.
4. Jones, C. *Software Estimating Rules of Thumb* (1997-2007). Software Productivity Research.
   https://www.scribd.com/document/482442740/Capers-Rules-pdf
5. McConnell, S. (2006). *Software Estimation: Demystifying the Black Art*. Microsoft Press. *(À vérifier dans le livre.)*
6. Cusumano, M., MacCormack, A., Kemerer, C. F., Crandall, B. (2003). « Software Development Worldwide: The State of
   the Practice ». *IEEE Software*, 20(6), 28-34. DOI : 10.1109/MS.2003.1241363
   https://dblp.org/rec/journals/software/CusumanoMKC03
7. MacCormack, A., Kemerer, C. F., Cusumano, M., Crandall, B. (2003). « Trade-offs between Productivity and Quality in
   Selecting Software Development Practices ». *IEEE Software*, 20(5), 78-85. DOI : 10.1109/MS.2003.1231158
   (Source de la médiane de 17,6 lignes/jour, 29 projets HP.)
8. Skeptics Stack Exchange, « Do professional software developers write an average of 10 lines of code per day? »
   (source secondaire). https://skeptics.stackexchange.com/questions/17224/
9. Prechelt, L. (2000). « An Empirical Comparison of Seven Programming Languages ». *IEEE Computer*, 33(10), 23-29.
   DOI : 10.1109/2.876288
10. Prechelt, L. (2000). *An empirical comparison of C, C++, Java, Perl, Python, Rexx, and Tcl for a search/string-processing
    program*. Université de Karlsruhe. https://publikationen.bibliothek.kit.edu/372000
11. QSM, *Performance Benchmark Tables*. https://qsm.com/resources/qsm-benchmark-tables
12. Beller, M., Orgovan, V., Buja, S., Zimmermann, T. (2020). « Mind the Gap: On the Relationship Between Automatically
    Measured and Self-Reported Productivity ». arXiv:2012.07428.
13. Forsgren, N., Storey, M.-A., Maddila, C., Zimmermann, T., Houck, B., Butler, J. (2021). « The SPACE of Developer
    Productivity ». *ACM Queue*, 19(1), 20-48. DOI : 10.1145/3453928. https://queue.acm.org/detail.cfm?id=3454124
14. Sadowski, C., Zimmermann, T. (dir.) (2019). *Rethinking Productivity in Software Engineering*. Apress.
    DOI : 10.1007/978-1-4842-4221-6
15. Bossavit, L. (2012 ; éd. papier 2013). *The Leprechauns of Software Engineering*. Leanpub. https://leanpub.com/leprechauns

### 11.3 Effet de l'IA sur la productivité

16. Peng, S., Kalliamvakou, E., Cihon, P., Demirer, M. (2023). « The Impact of AI on Developer Productivity: Evidence
    from GitHub Copilot ». arXiv:2302.06590.

### 11.4 Détection de code généré par IA (contenu) : signaux F

17. Suh, H., Tafreshipour, M., Li, J., Bhattiprolu, A., Ahmed, I. (2025). « An Empirical Study on Automatically Detecting
    AI-Generated Source Code: How Far Are We? ». *ICSE 2025*, 859-871. DOI : 10.1109/ICSE55347.2025.00064.
    https://arxiv.org/abs/2411.04299 *(Utilisée pour le poids faible de F6.)*
18. Shi, Y., Zhang, H., Wan, C., Gu, X. (2025). « Between Lines of Code: Unraveling the Distinct Patterns of Machine and
    Human Programmers » (DetectCodeGPT). *ICSE 2025*. https://arxiv.org/abs/2401.06461
    Code : https://github.com/YerbaPage/DetectCodeGPT *(Utilisée pour F3.)*
19. Nguyen, P. T., Di Rocco, J., Di Sipio, C., Rubei, R., Di Ruscio, D., Di Penta, M. (2024). « GPTSniffer: A
    CodeBERT-based classifier to detect source code written by ChatGPT ». *Journal of Systems and Software*, 112059.
    DOI : 10.1016/j.jss.2024.112059. Prépublication : arXiv:2307.09381.

### 11.5 Tatouage (watermarking) du code généré

20. Lee, T., Hong, S., Ahn, J., Hong, I., Lee, H., Yun, S., Shin, J., Kim, G. (2024). « Who Wrote this Code? Watermarking
    for Code Generation » (SWEET). *ACL 2024*, 4890-4911. DOI : 10.18653/v1/2024.acl-long.268.
    https://arxiv.org/abs/2305.15060
21. Kirchenbauer, J., Geiping, J., Wen, Y., Katz, J., Miers, I., Goldstein, T. (2023). « A Watermark for Large Language
    Models ». *ICML 2023*, PMLR 202, 17061-17084. https://arxiv.org/abs/2301.10226
    *(Hors périmètre de 7.1 : signalé pour référence, aucun signal ne repose sur le tatouage.)*

### 11.6 Traces d'agents dans les dépôts Git : signaux D et R

22. Robbes, R., Matricon, T., Degueule, T., Hora, A., Zacchiroli, S. (2026). « Agentic Much? Adoption of Coding Agents on
    GitHub ». arXiv:2601.18341 ; à paraître dans *ACM TOSEM*. *(Heuristiques par fichiers, trailers et auteurs ; taille
    des commits assistés : repères de D1, D4 et R2.)*
23. Ghaleb, T. A. (2026). « Fingerprinting AI Coding Agents on GitHub ». *MSR 2026*. arXiv:2601.17406. *(Signatures par
    agent : repères de D1 et F4.)*
24. Ghaleb, T. A. (2026). « AgenTag: Attribution of AI Coding Agents from Behavioral Fingerprints ». arXiv:2608.00966.
25. Li, H., Zhang, H., Hassan, A. E. (2026). « AIDev: Studying AI Coding Agents on GitHub ». *(Jeu de données ; identifiant
    arXiv à relever sur la page de l'article.)*
26. Stein, M. (2026). « How are AI agents used? Evidence from 177,000 MCP tools ». arXiv:2603.23802. *(Annexe A.3 :
    limites de la détection par métadonnées Git ; repère des faux négatifs, section 7.2.)*
27. coderbuds/ai-detector. https://github.com/coderbuds/ai-detector *(Outil : règles de détection dans PR et commits.)*
28. satinath-nit/ai-usage-measurement-framework. https://github.com/satinath-nit/ai-usage-measurement-framework
29. Réglage VS Code `git.addAICoAuthor` : notes de version VS Code 1.110 (février 2026) ; PR microsoft/vscode#310226
    (16 avril 2026). Synthèse secondaire : https://www.penligent.ai/hackinglabs/fr/vs-code-copilot-co-author-when-ai-attribution-becomes-a-supply-chain-trust-problem/
30. Vaughan, D. (2026). « The Invisible Agent Problem ». *(Blog, source secondaire.)*
    https://codex.danielvaughan.com/2026/06/27/detecting-ai-coding-agents-open-source-180-million-repo-census-codex-cli-attribution-supply-chain/

### 11.7 Textes juridiques (France)

31. Code du travail, art. L1222-4 (information préalable du salarié).
32. Code du travail, art. L2312-37 et L2312-38 (consultation du CSE). https://code.travail.gouv.fr/code-du-travail/l2312-38
33. Cour de cassation, chambre sociale, 11 décembre 2019, n° 18-11.792.

### 11.8 Référentiel d'enseignants : famille A et contre-indicateurs H

34. « Signaux de détection d'usage IA dans les repos étudiants » : référentiel des signaux indirects de l'agent
    AGENT_DETECTION_IA (5 pages, 8 catégories : historique Git, style, qualité, structure, documentation, cohérence
    inter-projets, signaux humains, mesures quantitatives). Document interne transmis par le mainteneur le 4 novembre
    2026, non publié ; seuils issus de l'expérience, sans validation empirique. *(Source de la phase 7.7.)*
