# 13. Conventions de projet

[← Sommaire](README.md)

**Statut** : **décision du mainteneur** (phase 8, 8 novembre 2026) ; les choix sont consignés en
[section 11](#11-décisions-du-mainteneur). Aucun code n'est encore écrit : chaque phase commence après
validation de son plan. L'exception au périmètre est reportée dans la [Vision](01-vision.md) ; les reprises
dans [Fonctionnalités](02-fonctionnalites.md), [Modèle de données](04-modele-donnees.md), [CLI](05-cli.md)
et [Interface web](06-interface-web.md) se feront phase par phase.

## 1. Décision

Le formateur impose aux groupes une **charte de conventions** (commits, branches et pull requests,
versions, CHANGELOG, README, documentation) et la vérifie en **revue de jalon**. Watchoo mesure le respect
de cette charte, dépôt par dépôt, et en tire une **vue « revue de jalon »**.

Ces mesures sont des **indicateurs de conformité** : déterministes, vérifiables, sans comparaison à un
historique personnel ni poids caché. Ils ne sont pas des soupçons et restent **séparés du
[diagnostic](12-supervision-diagnostic.md)** : aucune donnée, aucun score et aucune page en commun. Comme
le diagnostic, leurs résultats sont **réservés au formateur**.

## 2. Positionnement et garde-fous

- **Constat, pas verdict** : chaque résultat affiche sa règle, ses valeurs brutes et ses exclusions. Watchoo
  ne note pas : il ne calcule aucune note globale (pas de « /20 ») et ne classe ni dépôts ni personnes.
- **Inconnu n'est pas non conforme** : un critère qui ne peut pas être mesuré (clone indisponible, mode
  sans API, donnée absente) est affiché « inconnu » et sort du total.
- **Réservé au formateur** : la vue et la sortie `--json` sont destinées au formateur qui supervise
  l'espace, comme le diagnostic (décision du mainteneur). Elles ne contiennent rien du diagnostic ; les
  étudiants connaissent la charte et ses critères, pas les résultats individuels calculés par Watchoo.
- **Minimisation** : rien n'est persisté du contenu lu (message, corps de PR, README, CHANGELOG, INDEX,
  chemins de fichiers) ; seuls des codes d'écart, des compteurs, des masques et des numéros de version sont
  enregistrés. Les listes détaillées (documents non référencés, liens morts) sont **recalculées à
  l'affichage** depuis le clone, jamais stockées ni journalisées.
- **Lecture seule et locale** : inchangé. Aucun appel en écriture à GitHub, aucune donnée envoyée ailleurs.
  Aucun code n'est exécuté (une installation « reproductible » n'est pas vérifiée en lançant le projet).
- **Désactivé par défaut** : tout passe par le réglage `conventions.enabled`.
- **Exception au périmètre** : lire README, CHANGELOG, INDEX, `documentation/` et les fichiers d'outillage
  sort de la ligne « analyse du contenu du code » du [hors périmètre](01-vision.md#hors-périmètre-pour-linstant).
  L'exception retenue : lecture en mémoire de ces seuls fichiers, sans persistance du contenu ; le style
  des messages est jugé contre une charte connue des étudiants, **jamais** pour en tirer un soupçon d'usage
  de l'IA.

## 3. Référence : la charte

Valeurs par défaut de la configuration ([section 8](#8-configuration)) ; chaque formateur peut les adapter.

| | Exigence | Normes |
| --- | --- | --- |
| A | Commits `<type>(<scope>): <description>` ; types `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert` ; minuscules, impératif, sans point final, ~50 caractères ; corps séparé par une ligne vide ; footers `Clé: valeur` ; rupture par `!` ou `BREAKING CHANGE:`. Interdits : `update`, `maj`, `wip`, `test`, « Dépôt 1 », message vide ou sans type, passé composé, première personne. Outillage : hook `commit-msg` ou commitlint, gabarit de PR | [Conventional Commits 1.0.0](https://www.conventionalcommits.org/fr/v1.0.0/) |
| B | Une branche par fonctionnalité, `<type>/<scope>-<sujet-court>` ; jamais de travail direct sur `main`, protégée ; titre de PR conventionnel ; corps : Contexte, Ce qui change, Comment tester, Captures, `Closes #n` ; une relecture par un autre membre ; squash merge | — |
| C | Tags `vX.Y.Z`, pré-version `v2.0.0-rc.1`, jamais réécrits ; jalons `v1.0.0`, `v2.0.0`, `v3.0.0` | [SemVer 2.0.0](https://semver.org/lang/fr/) |
| D | `CHANGELOG.md` à la racine : `## [Unreleased]` en tête, `## [X.Y.Z] - AAAA-MM-JJ` du plus récent au plus ancien, catégories Added, Changed, Deprecated, Removed, Fixed, Security dans cet ordre, déclaration SemVer, pas d'entrée vide | [Keep a Changelog 1.1.0](https://keepachangelog.com/fr/1.1.0/) |
| E | `README.md` à la racine : présentation, groupe, stack et prérequis, installation et lancement, variables d'environnement, arborescence, liens vers `/documentation/INDEX.md` et `CHANGELOG.md`, licence ou confidentialité | — |
| F | `documentation/INDEX.md` (tableau `Document \| Thème \| Version \| Dernière mise à jour \| Responsable`, une ligne par fichier) ; sous-dossiers `NN-thème/` ; fichiers en minuscules à tirets, sans espace ni accent ; en-tête de chaque document ; schémas intégrés, `Media/` ; pas d'archive | — |
| G | Revue de jalon : 100 % de commits conformes ; branches par fonctionnalité, `main` protégée, PR relues ; tags de jalon jamais réécrits ; CHANGELOG à jour ; README avec installation ; INDEX exhaustif | — |

## 4. Données lues

| Source | Ce qui est lu | Mode |
| --- | --- | --- |
| Base (déjà synchronisée) | `commits.message`, `commit_branches`, `branches`, `pull_requests` (titre, branches, états, dates), `tags`, `releases` | les deux |
| Clone local, **par les objets git** | `git rev-list` (premier parent, intervalles entre tags), `git for-each-ref refs/remotes/origin/` (toutes les branches du distant), `git ls-tree` et `git cat-file blob` sur `origin/<défaut>` ou sur un tag, `git merge-base --is-ancestor` | les deux (le clone existe avec ou sans `gh`) |
| API, sans requête de plus | `merge_commit_sha` (PR fusionnées seulement), corps de PR (en mémoire) et dépôt d'origine dans `GET /pulls` ; `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` dans `GET /repos` | API |
| API, requête supplémentaire | `GET /repos/{o}/{r}/pulls/{n}/reviews` | API |

- Le clone n'est **jamais** lu par le système de fichiers de `repos/` : la copie de travail est remise à
  zéro à chaque synchro et peut avoir été modifiée à la main.
- Pas de repli par l'API pour l'arbre et les fichiers (même décision que pour les langages) : sans clone,
  les critères concernés sont « inconnus ».
- Les tags et chemins venus du dépôt passent après `--end-of-options` et sont validés ; une commande git
  qui échoue renvoie une erreur typée (« objet absent »), sans recopier son stderr (qui contiendrait le
  chemin ou le nom du tag) dans les logs.

## 5. Catalogue des indicateurs

Les préfixes CC, BR, PR, SV, CL, RD, DOC et TL ne recoupent pas ceux du diagnostic (D, R, F, A, H).
Colonnes : **mode** « les deux » = calculable sans API ; « API » = « inconnu » en clone seul. **Fiab.** :
H haute, M moyenne, F faible. **Prio** : P1 à P3.

### 5.1 Commits (CC)

**Parseur strict commun** (CC, PR1, SV5, CL10) : le sujet est la première ligne non vide, sans le suffixe
de squash « (#n) ». Il est conforme s'il correspond, en tenant compte de la casse, à
`^(<types>)(\([a-z0-9][a-z0-9._/-]*\))?!?: \S.*$`. Le parseur permissif du signal F4 du diagnostic reste
un mode « tolérant » distinct.

**Périmètre** (décision du mainteneur) : **toutes les branches suivies**, telles qu'elles sont aujourd'hui
(`git rev-list` des `origin/<branche>` suivies dans le clone, chaque commit compté une fois), depuis
`conventions.since`. `commit_branches` ne sert qu'en repli, avec la mention « peut inclure des commits
réécrits » (les rattachements d'un commit à une branche ne sont jamais retirés). La branche par défaut seule
forme une seconde vue, informative.

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CC1 | Taux de commits conformes | Commits sans écart bloquant (CC2) / commits évalués (hors CC4) | dépôt, intervalle entre tags | base + clone | les deux | H | P1 |
| CC2 | Écarts par motif | **Bloquants** : `empty`, `forbidden` (sujet entier dans la liste interdite, ou préfixe `wip`, `fixup!`, `squash!`), `github_default` (`^(Update\|Create\|Delete\|Rename) \S+$`, « Add files via upload »), `no_type`, `unknown_type`, `type_case`, `separator`, `trailing_period`, `body_separator`, `length_hard` (> 72 caractères). **Avertissements** : `desc_capitalized` (sauf sigle), `length_soft` (> 50), `footer_format`, `git_revert` (`Revert "…"` produit par Git, compté) | commit ; motif × dépôt, motif × contributeur | base | les deux | H | P1 |
| CC3 | Taux par contributeur | CC1 sur les commits dont il est l'auteur, avec tendance depuis le dernier tag | contributeur, contributeur × dépôt | base | les deux | H | P1 |
| CC4 | Exclusions | Hors dénominateur, comptés à part : fusions (`parent_count ≥ 2`), bots (suffixe `[bot]` et comptes du catalogue), commit racine « Initial commit » créé par GitHub ou Classroom. Les éditions web (« Update README.md ») et les comptes d'agents IA restent évalués | commit | base | les deux | H | P1 |
| CC5 | Formulation à revoir | Avertissement seulement : `past_tense` (lexique fermé de participes FR et EN), `progressive` (`-ing`), `first_person`. Aucune règle sur la seule terminaison « -é » | commit | base | les deux | M | P2 |
| CC6 | Footers et issues | Part des commits avec un footer valide et part de ceux qui citent une issue (`(Closes\|Fixes\|Resolves\|Refs) #\d+`) ; informatif | dépôt, contributeur | base | les deux | H | P3 |
| CC7 | Langue des messages | Mélange FR / EN dans un même dépôt, si la charte impose une langue ; informatif | dépôt | base | les deux | M | P3 |

### 5.2 Branches (BR)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| BR1 | Nom de branche conforme | Branches du distant (`refs/remotes/origin/` du clone) et `pull_requests.head_branch` des PR non issues d'un fork, hors branche par défaut et exclusions (`gh-pages`, `dependabot/*`, `renovate/*`, réglage). Conforme si `^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$` ; un seul segment après `/` : « à revoir ». Motif donné : type inconnu, majuscule, `_`, espace, accent, `/` absent | branche, dépôt ; contributeur par l'auteur de la PR | clone + base | les deux | H | P1 |
| BR2 | Branche réutilisée | Une `head_branch` (hors fork) avec plus d'une PR fusionnée vers la branche par défaut | branche, dépôt | base | API | M | P2 |
| BR3 | Branche par défaut protégée | **Reporté** (décision du mainteneur : pas mis en œuvre dans la phase 8). Mesure prévue : `protected` de `GET /branches` : oui, non, inconnu (clone), non applicable (par exemple dépôt privé sur GitHub Free). **Instantané** sans historique | dépôt | API, 0 requête | API | M | — |
| BR4 | Branche par défaut réécrite | À la synchro, l'ancienne tête n'est plus ancêtre de la nouvelle (`merge-base --is-ancestor`) ; événement daté (ancien et nouveau SHA). Ancien objet absent du clone : « inconnu », jamais « réécrit ». En mode API, l'erreur `ErrUnknownBase` de la synchro sert aussi de signal | événement, dépôt | clone | les deux | H | P2 |
| BR5 | Méthodes de fusion autorisées | `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` du dépôt (renvoyés à un compte qui a le droit push) | dépôt | API, 0 requête | API | M | P3 |

### 5.3 Pull requests (PR)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PR1 | Titre de PR conventionnel | Parseur commun sur le titre des PR hors brouillon et hors bots ; PR ouvertes et fusionnées comptées à part | PR, dépôt, auteur | base | API | H | P1 |
| PR2 | Titre interdit ou par défaut | Titre normalisé dans la liste interdite, ou égal (après normalisation : casse, `-` et `_` en espaces) au nom de la branche de tête ou au sujet du seul commit de la PR (titres proposés par GitHub) | PR | base | API | H | P1 |
| PR3 | Commits arrivés sur la branche par défaut hors PR | Parcours `git rev-list --first-parent origin/<défaut>` depuis `since`, sans le commit racine. **Par PR** : SHA égal au `merge_commit_sha` d'une PR fusionnée ; suite rebasée par GitHub, reconnue par une heuristique sans requête (commits consécutifs dont le committer est GitHub et dont `committed_at` vaut le `merged_at` de la PR qui la termine ; décision du mainteneur) ; fusion dont le committer est GitHub. **Hors PR** : committer humain, fusion locale poussée, édition web sans PR. Sinon : « indéterminé ». En clone seul, repli sur `^Merge pull request #\d+` et `\(#\d+\)$` avec le committer GitHub reconnu à son e-mail `noreply@github.com` (en mémoire) | commit ; dépôt ; « auteur du commit arrivé hors PR » (jamais « a poussé ») | clone + base | les deux (dégradé en clone) | H en API, M en clone | P1 |
| PR4 | Relue par un autre membre | Nouvelle méthode Provider `PullRequestReviews` (login, identifiant, état, date ; jamais le corps). Relue si, pour au moins un relecteur d'un autre identifiant que l'auteur et non exclu (formateur, bots), le dernier état parmi `APPROVED`, `CHANGES_REQUESTED` et `DISMISSED` déposé au plus tard à `merged_at` est `APPROVED`. Un autre compte suffit ; le réglage `pr.reviewer_must_contribute` exige en plus un relecteur auteur d'au moins un commit du dépôt. Seuls des compteurs sont stockés, pas les relecteurs. Vue dérivée : « fusionnées sans relecture » | PR, dépôt ; relectures reçues par l'auteur de la PR | API, 1 requête par PR | API | H | P1 |
| PR5 | Corps structuré et `Closes #n` | Corps lu en mémoire (comme pour D2) : titres de section, ou texte en gras, parmi des synonymes FR et EN versionnés (contexte, ce qui change, comment tester, captures) ; une section vide (ou réduite à un commentaire HTML) ne compte pas. Mot-clé de fermeture cherché à part. Seul un masque de bits est stocké (NULL tant qu'il est inconnu). Faux négatif connu : issue liée par la barre latérale « Development » | PR, dépôt | API, 0 requête | API | M | P2 |
| PR6 | Fusion en squash | Commit de `merge_commit_sha` à un parent avec le suffixe « (#n) » : squash ; deux parents : merge commit. Conforme si squash **et** message conforme au parseur | PR | base | API | M | P2 |
| PR7 | Gabarit de PR | Voir TL3 | dépôt | clone | les deux | H | P2 |

### 5.4 Tags et SemVer (SV)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SV1 | Nom de tag SemVer | `v` minuscule puis version SemVer 2.0.0 (comparateur extrait de `internal/selfupdate` vers `internal/semver`). Motifs : sans `v`, `V`, composant manquant, zéro en tête, pré-version vide ; `+build` en avertissement | tag, dépôt | base | les deux | H | P1 |
| SV2 | Jalons présents | Pour chaque jalon configuré : présent, présent sous un nom non conforme, absent ; une pré-version ne vaut pas jalon. Avec une échéance : comparée à la première observation du tag ; « indéterminé » si aucune synchro réussie n'a eu lieu entre la date du commit visé et l'échéance (affichage « posé entre X et Y ») | jalon × dépôt | base | les deux | H (présence), M (échéance) | P1 |
| SV3 | Tag réécrit ou supprimé | Table `tag_observations`, alimentée seulement si la lecture des tags a réussi et n'est pas tronquée : même nom avec un autre SHA (réécrit), disparu (supprimé). Libellé : « aucune réécriture observée depuis le … », jamais « jamais réécrit » ; aucune attribution à une personne | tag, dépôt | base | les deux | M | P1 |
| SV4 | Pré-versions | Forme `-rc.N`, `-beta.N`, `-alpha.N` ; les formes libres (`-rc1`, `-RC.1`) donnent un avertissement, pas un écart (décision du mainteneur) ; avertissement aussi si une rc est posée après la version finale ou si la précédence contredit l'ascendance | tag | base + clone | les deux | M | P2 |
| SV5 | Saut cohérent avec les commits | Pour chaque tag final F, base B = plus grand tag final conforme inférieur à F qui en est ancêtre ; commits de `B..F` hors fusions et bots. Rupture : MAJOR attendue (MINOR avant 1.0.0) ; `feat` : au moins MINOR. **Seul le sous-versionnement est signalé** ; « indéterminé » si moins de 80 % des commits de l'intervalle sont conformes ; premier tag non évalué | paire de tags | clone + base | les deux | M | P2 |
| SV6 | Release associée | Hors brouillons : jalon sans release, release sans tag, drapeau `prerelease` incohérent. **Information seule** : la charte ne parle que de tags, SV6 ne compte pas dans la conformité | tag, release | base | API | H | P2 |
| SV7 | Précédence et historique | Version supérieure posée sur un ancêtre d'une version inférieure ; saut d'incrément (v1 → v3) en information | paire de tags | base + clone | les deux | M | P3 |

### 5.5 CHANGELOG (CL)

Le blob `CHANGELOG.md` est lu en mémoire depuis le clone (au plus 1 Mio) ; les blocs de code sont ignorés.

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CL1 | Présent à la racine | `CHANGELOG.md` exact ; sinon mal nommé (`(?i)^change[-_]?log(\.md)?$`), ailleurs, absent, ou non mesurable (dépôt vide) | dépôt, tag | clone | les deux | H | P1 |
| CL2 | En-tête et déclarations | H1 `Changelog` (ou `Journal des modifications`), mention de Keep a Changelog et de SemVer avant le premier H2 | dépôt | clone | les deux | H | P1 |
| CL3 | `[Unreleased]` en tête | Premier H2 = `## [Unreleased]` ; écarts : absent, mal placé, en double, sans crochets | dépôt | clone | les deux | H | P1 |
| CL4 | Titres de version et dates | `## [X.Y.Z(-pre)] - AAAA-MM-JJ`, `[YANKED]` admis ; date réelle, non future. Écarts : `v` dans le titre (avertissement), crochets manquants, date au format FR, absente, invalide, future | version | clone | les deux | H | P1 |
| CL5 | Ordre décroissant | Précédence SemVer strictement décroissante, dates non croissantes | dépôt | clone | les deux | H | P2 |
| CL6 | Catégories | H3 dans {Added, Changed, Deprecated, Removed, Fixed, Security}, ou leurs libellés français (Ajouté, Modifié, Obsolète, Supprimé, Corrigé, Sécurité), admis par défaut ; sans doublon, dans cet ordre, aucune puce hors catégorie | version | clone | les deux | H | P1 |
| CL7 | Entrées vides | Version publiée sans puce, catégorie sans puce, puce vide ou « TODO » ; un `[Unreleased]` vide n'est pas un écart | dépôt | clone | les deux | H | P2 |
| CL8 | Tags et sections cohérents | Tag `vX.Y.Z` sans section, section datée sans tag (`[YANKED]` exempté) ; écart de date affiché sans être compté | tag | base + clone | les deux | H | P1 |
| CL9 | À jour au tag | Dans `<tag>:CHANGELOG.md`, la section `## [X.Y.Z]` existe ; résultat en cache par (tag, SHA) | jalon | clone | les deux | H | P1 |
| CL10 | Fraîcheur | Nombre de `feat` et `fix` sur la branche par défaut depuis le dernier commit qui a touché le CHANGELOG ; grisé si CL9 est conforme | dépôt | clone + base | les deux | M | P2 |
| CL11 | Liens de comparaison | Définition `[X]: url` pour chaque titre ; hors total | dépôt | clone | les deux | M | P3 |
| CL12 | Recopie de `git log` | Au moins 10 puces, dont au moins 60 % identiques à un sujet de commit, ou au moins 20 % de SHA ou de « Merge » ; seuls les pourcentages sont stockés ; « à relire » | dépôt | clone + base | les deux | F | P3 |

### 5.6 README (RD)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| RD1 | Présent à la racine | `README.md` exact ; `README.<langue>.md` ne compte pas et n'est pas une faute | dépôt, tag | clone | les deux | H | P1 |
| RD2 | Sections attendues | Titres hors code, normalisés et comparés à des synonymes FR et EN : présentation, groupe, stack et prérequis, installation, lancement, variables d'environnement, arborescence, licence ou confidentialité (un fichier `LICENSE` est noté). Écart : « non trouvée par son titre ». Seul un masque est stocké | dépôt, tag | clone | les deux | M | P1 |
| RD3 | Liens vers INDEX et CHANGELOG | Lien relatif, absolu depuis la racine (`/documentation/INDEX.md`) ou URL du même dépôt ; états : valide, mort, absent, vers le dossier seulement | dépôt, tag | clone | les deux | H | P1 |
| RD4 | Taille | Mots hors code, seuil réglable (150) ; valeur brute affichée | dépôt | clone | les deux | M | P2 |
| RD5 | Installation | Bloc de commandes dans la section installation ou lancement ; `.env.example` présent si des variables sont annoncées ; piste seulement | dépôt | clone | les deux | F | P3 |

### 5.7 Documentation (DOC)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| DOC1 | INDEX présent | `documentation/INDEX.md` exact ; variantes signalées (`docs/`, `index.md`) | dépôt, tag | clone | les deux | H | P1 |
| DOC2 | Tableau de l'INDEX | Tableau GFM avec les 5 colonnes attendues (synonymes admis) ; nombre de lignes. Les cellules ne sont jamais stockées (la colonne Responsable contient des noms) | dépôt, tag | clone | les deux | H | P1 |
| DOC3 | INDEX exhaustif | Nombre de `*.md` de `documentation/` (hors INDEX et `Media/`) qu'aucun lien de l'INDEX ne vise ; lignes de l'INDEX en double | dépôt, tag | clone | les deux | H | P1 |
| DOC4 | Liens morts | Liens relatifs du README, de l'INDEX et des documents dont la cible n'existe pas ; différence de casse signalée comme telle | dépôt | clone | les deux | H | P1 |
| DOC5 | Nommage | Dossiers `^\d{2}-[a-z0-9]+(-[a-z0-9]+)*$` ; fichiers en minuscules à tirets, sans espace ni accent (exceptions : `INDEX.md`, `README.md`, et `Media/` avec sa majuscule, admis par la charte) | dépôt, tag | clone | les deux | H | P1 |
| DOC6 | En-têtes de document | Titre, objectif, périmètre, auteur, date, version : front matter ou lignes « Clé : valeur » dans les 40 premières lignes ; compteurs par champ | dépôt | clone | les deux | M | P2 |
| DOC7 | Schémas et images | Images relatives présentes ou cassées, images externes (badges exclus), blocs `mermaid`, dossier `Media/` | dépôt | clone | les deux | H | P2 |
| DOC8 | Archives | `.zip`, `.rar`, `.7z`, `.tar(.gz)` **dans tout le dépôt** (décision du mainteneur) ; les `.docx` et `.pdf` sous `documentation/` sont signalés en information | dépôt, tag | clone | les deux | H | P1 |
| DOC9 | Fraîcheur | Dates du dernier commit sur `documentation/`, l'INDEX, le CHANGELOG et le code ; commits hors documentation depuis ; piste | dépôt | clone | les deux | M | P2 |
| DOC10 | Dates déclarées | Date de la colonne de l'INDEX comparée au dernier commit du document | document | clone | les deux | F | P3 |

Les listes de DOC3, DOC4 et DOC5 (chemins) sont recalculées à l'affichage ; seuls les compteurs sont stockés.

### 5.8 Outillage (TL)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TL1 | commitlint configuré | Fichier de configuration (profondeur ≤ 2) ou clé `commitlint` / `@commitlint/cli` dans `package.json` ; sous-état « étend config-conventional » ; date d'introduction | dépôt | clone | les deux | H | P2 |
| TL2 | Hook `commit-msg` versionné | `.husky/commit-msg` (avec `prepare: husky`), lefthook, pre-commit en `commit-msg`, cz, cog. Statut « configuré », **jamais « actif »** | dépôt | clone | les deux | M | P2 |
| TL3 | Gabarit de PR | Emplacements reconnus par GitHub, casse ignorée ; nombre de sections de la charte présentes. Absent : « non trouvé dans le dépôt » (un gabarit d'organisation est invisible) | dépôt | clone | les deux | H | P2 |
| TL4 | CI vérifiant les conventions | Un workflow cite une action connue (commitlint, semantic-pull-request…) ; seul l'identifiant du motif est gardé | dépôt | clone | les deux | M | P3 |
| TL5 | Effet de l'outillage | CC1 avant et après la plus ancienne introduction parmi TL1, TL2, TL4 ; « n/a » si un effectif est inférieur à 10 | dépôt | base + clone | les deux | M | P3 |

## 6. Ce qui n'est pas mesurable

| Exigence | Pourquoi | Ce que Watchoo montre |
| --- | --- | --- |
| Sens du message, type juste, CHANGELOG « pour les humains » | Jugement humain | Syntaxe (CC2), formulation manifeste (CC5), recopie probable (CL12) |
| Impératif en français | « ajoute » est impératif et présent ; l'infinitif est courant | Cas manifestes seulement : participe passé, première personne |
| Hook installé et actif | `.git/hooks` n'est pas versionné ; `--no-verify` ; éditions web | « Configuré » (TL2), effet mesuré (TL5) |
| Une branche, une PR = un sujet | Sémantique | Branche réutilisée (BR2), taille de la PR en contexte |
| Qualité de la relecture, identité réelle du relecteur | Humain ; comptes multiples | Approbation, délai, comptes exclus |
| Protection de la branche par défaut | Droit admin requis pour le détail ; notion absente de git ; dépend du plan GitHub ; BR3 reporté | Effets observés : commits hors PR (PR3), réécritures (BR4) |
| Tags « jamais réécrits », date de pose d'un tag | Instantanés seulement ; pas de date de tag | « Aucune réécriture observée depuis le … », première observation (SV3) |
| Justesse sémantique du saut de version | Pas d'API publique déclarée | Cohérence avec les commits déclarés (SV5) |
| Installation reproductible | Il faudrait exécuter le code | Piste RD5 |
| Exactitude du README, de l'INDEX, des en-têtes | Il faudrait croiser des données personnelles | Présence et forme |
| Branches créées et supprimées entre deux synchros | Invisibles (`fetch --prune`) | Rien |

## 7. Restitution

**Pas de note globale.** Chaque famille affiche un taux `n/m` expliqué (dénominateur et exclusions). Une
liste de contrôle compte des **critères** (« 7/9 critères conformes, 1 inconnu ») ; elle ouvre toujours le
détail et ne trie ni ne classe rien.

- **Par dépôt** : CC1, BR1, PR1, PR3, PR4, SV1, SV2, puis les grilles CHANGELOG (CL1 à CL9), README (RD1 à
  RD3), documentation (DOC1 à DOC5, DOC8) et outillage.
- **Par contributeur** : CC3 et ses principaux motifs, titres de PR (PR1), commits arrivés hors PR dont il
  est l'auteur (PR3), relectures reçues sur ses PR (PR4 ; les relectures données ne sont pas comptées,
  faute de stocker les relecteurs). Aucun indicateur de dépôt (CL, RD, DOC, SV) n'est
  attribué à une personne.

### 7.1 Revue de jalon (RJ)

Tableau critère × jalon (`v1.0.0`, `v2.0.0`, `v3.0.0`). Les critères de fichiers (RD, DOC, CL) sont
évalués **sur l'arbre du tag** (en cache par couple tag-SHA) ; les commits et les PR **sur l'intervalle
entre le tag précédent et le tag du jalon**. Tag absent : intervalle borné par l'échéance configurée, sinon
« non évaluable ». `conventions.since` est obligatoire pour la RJ.

| Critère G | Indicateurs | Cible |
| --- | --- | --- |
| 100 % de commits conformes | CC1 sur l'intervalle, toutes branches suivies | 100 % |
| Branches par fonctionnalité, `main` protégée, PR relues | BR1, PR3 = 0, PR4 ; protection « non évaluée » (BR3 reporté) | conforme, ou « inconnu » en clone seul |
| Tags de jalon jamais réécrits | SV1, SV2, SV3 | présents, aucune réécriture observée |
| CHANGELOG à jour au jalon | CL9, CL8 | section présente au tag |
| README avec installation | RD1, RD3, section installation de RD2 (RD5 en piste) | présents ; les autres sections de RD2 en information |
| INDEX exhaustif | DOC1, DOC3 | 0 document non référencé |

### 7.2 Interfaces

- **Web** : route `/conventions` (vue d'ensemble et revue de jalon), entrée de navigation seulement si
  `conventions.enabled`. Section « Conventions » sur la page du dépôt (badges par PR, pastille par tag, bloc
  « Outillage ») ; ligne « Commits conformes 18/25 (72 %) » sur la page du contributeur ; pastille compacte
  sur le tableau de bord, sans tri par cette valeur. Icônes toujours accompagnées d'un texte.
- **CLI** : une seule commande, sans sous-commande (un dépôt nommé comme une sous-commande serait ambigu) :
  `watchoo conventions [repo] [--commits] [--failing] [--contributor <x>] [--milestone vX.Y.Z] [--json]`.
  `--commits --failing` liste le SHA court, l'auteur et les codes d'écart, jamais le message. Pas d'option
  `--strict` (code de sortie selon la conformité) pour l'instant : ajoutable plus tard en MINOR.
- **JSON** : clés en anglais (`commits.rate`, `commits.issues[{code,count,shas}]`, `semver_checks[]`,
  `changelog.rules[]`…), figées par `internal/cli/json_schema_test.go`. Aucune clé ajoutée à `prs` ou
  `releases` dans un premier temps.
- Tous les textes passent par `internal/i18n`, en anglais et en français.

## 8. Configuration

Bloc `conventions:` des réglages, séparé de `diagnostic:` ; les valeurs par défaut sont celles de la charte
de la [section 3](#3-référence--la-charte).

| Réglage | Défaut | Rôle |
| --- | --- | --- |
| `enabled` | `false` | Active les conventions |
| `since` | vide | Date de début de l'évaluation (obligatoire pour la revue de jalon) ; surchargeable par dépôt (`watchoo repo add --conventions-since AAAA-MM-JJ`, comme `--flemme` et `--dead` ; colonne `repositories.conventions_since`) |
| `commit.types`, `commit.scopes` | types de la charte, tous scopes | Types et scopes admis |
| `commit.max_subject_soft`, `commit.max_subject_hard` | 50, 72 | Longueur du sujet (avertissement, bloquant) |
| `commit.forbidden` | liste de la charte | Sujets interdits |
| `commit.language` | `any` | `fr`, `en` ou `any` |
| `branch.pattern`, `branch.exclude` | `<type>/<scope>-<sujet>`, `gh-pages`, `dependabot/*`, `renovate/*` | Nommage (RE2 validée au chargement) |
| `pr.body_sections`, `pr.require_closes` | sections de la charte, `true` | Corps de PR |
| `pr.min_reviews`, `pr.reviewer_must_contribute` | 1, `false` | Relectures ; un autre compte suffit, sauf si le relecteur doit être contributeur |
| `tags.milestones` | `v1.0.0`, `v2.0.0`, `v3.0.0` (échéance facultative) | Jalons |
| `tags.prerelease_forms` | `rc`, `beta`, `alpha` | Pré-versions conformes ; les autres formes donnent un avertissement |
| `changelog.categories_fr` | `true` | Admet les libellés français (catégories, H1) |
| `readme.sections` | sections de la charte | Synonymes des sections attendues |
| `docs.dir`, `docs.media_dir`, `docs.archives_scope` | `documentation`, `Media`, `repository` | Arborescence ; archives interdites dans tout le dépôt |
| `excluded_accounts` | vide | Comptes exclus (formateur, bots), propres aux conventions : distincts de `diagnostic.exclude` |

Le bloc se partage par [`configshare`](02-fonctionnalites.md) (décision du mainteneur), sauf `enabled`,
`excluded_accounts` (qui contient des logins) : partager une charte est l'usage visé. La date propre à un
dépôt est exportée avec lui, comme ses seuils d'activité. Les clés sont ensuite figées par SemVer.

## 9. Impact technique

- **Packages** :
  - `internal/conventions` : fonctions pures, sur le modèle d'`internal/aisignature`, qui renvoient des
    codes stables et ne lisent ni la base ni la forge (`ParseCommit`, `CheckBranch`, `CheckPRTitle`,
    `CheckPRBody`, `CheckTag`, `ExpectedBump`, `CheckChangelog`, `CheckReadme`, `CheckIndex`,
    `CheckDocsTree`, `DetectTooling`) ;
  - `internal/semver` : comparateur extrait d'`internal/selfupdate`, sans changement de surface ;
  - la reconnaissance des bots passe d'`internal/diagnostic` à `internal/aisignature` (catalogue de comptes).
- **Synchro** : `internal/sync/conventions.go`, appelé quand `conventions.enabled`. Le clone est lu par des
  interfaces optionnelles du provider git (`TreePaths`, `ReadBlobs`, `FirstParent`, `IsAncestor`,
  `RevList`), sans ajout à l'interface `Provider`. Côté `Provider` : `PullRequest.MergeCommitSHA`
  (rempli seulement si la PR est fusionnée), `PullRequest.FromFork` et
  `PullRequestReviews` (GET ; `ErrUnsupported` en clone). Le calcul n'est relancé que si la tête de la
  branche par défaut, `RulesVersion` ou l'empreinte de la configuration ont changé. Échecs journalisés en
  Debug, sans message ni chemin.
- **Données** (migrations à partir de `00016`, décrites d'abord dans [Modèle de données](04-modele-donnees.md)) :
  - `commits.parent_count`, écrit à la synchro depuis `Commit.Parents` ; l'existant est rattrapé par un
    `UPDATE` depuis `git rev-list --parents` (l'insertion ne modifie jamais un commit déjà en base) ;
  - `repository_conventions(repo_id, rev_sha, rev_kind, tag_name, rules_version, config_hash, results,
    computed_at)` : codes et compteurs, par tête et par tag ;
  - `tag_observations(repo_id, name, sha, first_seen_at, last_seen_at, ended_at, end_reason)` ;
  - `branch_rewrites(repo_id, branch, old_sha, new_sha, observed_at)` ;
  - `repositories.conventions_since` (NULL = date de l'espace) ;
  - `pull_requests.merge_commit_sha`, `from_fork`, `body_sections` (NULL = inconnu),
    `reviews_by_others`, `approved_before_merge`, `reviews_checked_at` : des compteurs et des drapeaux,
    jamais les logins des relecteurs (décision du mainteneur) ;
  - `branches.protected` n'est pas ajouté tant que BR3 est reporté ;
  - soit 19 tables au lieu de 16.
  - CC, BR1, PR1, PR2, SV1, SV2 et SV6 sont calculés à la volée depuis la base : un changement de charte
    prend effet tout de suite.
- **Quota** :
  - phases 8.1 à 8.4 : aucune requête ;
  - 8.5 : aucune requête en régime courant ; activer les conventions (ou monter `RulesVersion`) déclenche
    **une** relecture complète de la liste des PR (au plus 10 pages par dépôt) pour remplir
    `merge_commit_sha`, `from_fork` et `body_sections` ;
  - 8.6 : une requête par PR fusionnée, une seule fois, plus les PR ouvertes dont `updated_at` a changé ;
    par exemple 30 dépôts × 40 PR = 1 200 requêtes au premier passage (24 % du quota horaire), plafonné
    par synchro et coupé sous la réserve de quota. Le cache ETag est partagé dans `watchoo serve` mais vide
    à chaque processus CLI ;
  - les releases sont déjà relues en entier à chaque synchro : coût inchangé.
- **Interaction avec le diagnostic** : la fusion en squash imposée par la charte produit de gros commits
  uniques sur la branche par défaut, qui alimentent A1 (commit massif) et A2 (dépôt rendu en une fois) et
  effacent la matière de H4. Quand `conventions.enabled` est actif, les commits de squash identifiés
  (`merge_commit_sha` d'une PR, ou « (#n) » avec le committer GitHub) sont exclus de A1 et A2, et le
  critère « nouveauté : sujet Conventional Commits » de F4 est neutralisé après `conventions.since`. À
  reporter dans [Supervision et diagnostic](12-supervision-diagnostic.md).
- **Surface publique** : nouvelle commande, route, réglages, clés JSON et migrations additives : MINOR.
- **Tests** : tables de cas dans `internal/conventions` (chaque code de CC2, exemples de la charte, regex
  officielle de semver.org, CHANGELOG de Watchoo comme cas conforme) ; `gittest` (outillage, CHANGELOG à
  chaque tag, tag réécrit et branche réécrite par `push --force`, push direct et fusion) ; `githubtest`
  (`merge_commit_sha`, `/pulls/{n}/reviews`) ; e2e CLI (`conventions --json`) et web ; catalogues i18n EN et
  FR.

## 10. Plan par phases (proposition)

Chaque phase est livrable seule ; les phases 8.1 à 8.4 suffisent pour une revue de jalon en clone seul,
8.5 et 8.6 complètent les critères « inconnus ».

| Phase | Contenu | Coût | Version |
| --- | --- | --- | --- |
| 8.0 | Ce document validé (fait, 8 novembre 2026) et exception reportée dans 01 ; reprises dans 02, 04, 05, 06 et 12 avec chaque phase | aucun code | — |
| 8.1 | `internal/conventions` (parseur), `internal/semver`, réglages `conventions.*`, CC1 à CC4, `commits.parent_count` et son rattrapage, `watchoo conventions --commits`, section web et page contributeur, ajustements de A1, A2 et F4 | 0 quota, 1 migration | 1.7.0 |
| 8.2 | À la volée : BR1, PR1, PR2, SV1, SV2 (présence), SV6 ; première revue de jalon ; CC5 à CC7 | 0 quota | 1.8.0 |
| 8.3 | Arbre du clone : `repository_conventions`, CL1 à CL9, RD1 à RD4, DOC1 à DOC8, TL1 à TL3 ; revue de jalon sur l'arbre des tags | 0 quota, 1 migration | 1.9.0 |
| 8.4 | Historique : `tag_observations` (SV3, échéances de SV2), `branch_rewrites` (BR4), SV4, SV5, SV7, CL10 | 0 quota, 2 migrations | 1.10.0 |
| 8.5 | API sans requête de plus : `merge_commit_sha`, `from_fork`, `body_sections`, méthodes de fusion ; PR3 complet, PR5, PR6, BR2, BR5 | une relecture des listes de PR | 1.11.0 |
| 8.6 | Relectures : PR4, « fusionnées sans relecture », relectures reçues | 1 requête par PR fusionnée | 1.12.0 |
| 8.7 | Pistes facultatives : CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4, TL5 | 0 quota | MINOR |

## 11. Décisions du mainteneur

Décisions du 8 novembre 2026.

| # | Question | Décision |
| --- | --- | --- |
| 1 | Lire README, CHANGELOG, `documentation/` et l'outillage, exclus par la [vision](01-vision.md) | **Oui**, en mémoire, sans persistance du contenu ; exception reportée dans la vision |
| 2 | Restitution et visibilité | Taux et critères `n/m`, **sans note globale** ; résultats **réservés au formateur**, comme le diagnostic |
| 3 | Périmètre du « 100 % de commits conformes » | **Toutes les branches suivies** ; la branche par défaut seule en information |
| 4 | `conventions.since` | Date de l'espace, **surchargeable par dépôt** |
| 5 | Stockage des relectures | **Compteurs seuls** (pas de table `pull_request_reviews`) ; relectures reçues seulement |
| 6 | Partage de la charte | **Oui** par `configshare`, sauf `enabled` et `excluded_accounts` |
| 7 | Jalons par défaut | **`v1.0.0`, `v2.0.0`, `v3.0.0`** |
| 8 | Release GitHub par jalon (SV6) | **Information seule** |
| 9 | Relecteur « autre membre » | **Un autre compte suffit** ; `pr.reviewer_must_contribute` en option |
| 10 | Comptes exclus | **Propres aux conventions**, distincts de `diagnostic.exclude` |
| 11 | Fusion par rebase (PR3) | **Heuristique sans requête** ; « indéterminé » en cas de doute |
| 12 | Option `--strict` | **Pas pour l'instant** |
| 13 | Langue des commits | **Libre** (`commit.language: any`) ; mélange signalé en information (CC7) |
| 14 | Tolérances par défaut (avertissement plutôt qu'écart) | `[v1.2.0]` au CHANGELOG ; catégories et H1 en français ; pré-versions libres et `+build` ; `Media/` avec sa majuscule |
| 15 | Archives | Interdites **dans tout le dépôt** ; `.docx` et `.pdf` sous `documentation/` signalés en information |
| 16 | Protection de branche (BR3) | **Reportée** : pas mise en œuvre dans la phase 8 |

### 11.1 Points restant à vérifier

- **Seuils** à valider sur des dépôts réels avant affichage : 80 % pour SV5, 150 mots pour RD4, écart de
  date de CL8, seuils de CL12 et DOC9.
- **Avant la phase 8.6** : une relecture met-elle à jour `updated_at` de la PR (relecture des PR ouvertes) ?
- **Si BR3 est repris** : `protected` reflète-t-il les rulesets ? Sur quel plan GitHub sont les dépôts
  étudiants (la protection d'un dépôt privé n'existe pas sur GitHub Free) ?

## 12. Références

- Conventional Commits 1.0.0 : <https://www.conventionalcommits.org/fr/v1.0.0/>
- Semantic Versioning 2.0.0 : <https://semver.org/lang/fr/>
- Keep a Changelog 1.1.0 : <https://keepachangelog.com/fr/1.1.0/>
- commitlint et `@commitlint/config-conventional` : <https://commitlint.js.org/>
- API GitHub, revues de pull request : <https://docs.github.com/en/rest/pulls/reviews>
- API GitHub, branches (`protected`) : <https://docs.github.com/en/rest/branches/branches>
