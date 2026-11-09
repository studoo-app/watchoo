# 13. Conventions de projet

[← Sommaire](README.md)

**Statut** : **décision du mainteneur** (phase 8, 8 novembre 2026) ; les choix sont consignés en
[section 11](#11-décisions-du-mainteneur). Phases 8.1 à 8.7 réalisées, non publiées ; seul BR3 (protection de branche) reste
reporté. L'exception au périmètre est reportée dans la [Vision](01-vision.md) ; les
reprises dans [Fonctionnalités](02-fonctionnalites.md), [Modèle de données](04-modele-donnees.md),
[CLI](05-cli.md) et [Interface web](06-interface-web.md) se font phase par phase.

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
| Base (déjà synchronisée) | `commits.message`, `commit_branches`, `branches`, `pull_requests` (titre, branches, états, dates ; depuis la phase 8.5 `merge_commit_sha`, `from_fork`, `body_sections` ; depuis la phase 8.6 les compteurs de relectures `reviews_by_others`, `approvals_by_others`, `approvals_by_contributors` et `reviews_checked_at`), `repositories` (depuis la phase 8.5 `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge`, `conventions_prs_version`), `tags`, `releases` | les deux |
| Clone local, **par les objets git** | `git rev-list` (premier parent, intervalles entre tags), `git for-each-ref refs/remotes/origin/` (toutes les branches du distant), `git ls-tree -r -l -z` sur le SHA complet de la tête de la branche par défaut (liste des branches de la source de la synchro) ou du commit d'un tag (lu dans la base) ; jamais un nom de branche ou de tag, puis `git cat-file --batch` sur les identifiants d'objets (voir [section 9](#9-impact-technique)), `git merge-base --is-ancestor` | les deux (le clone existe avec ou sans `gh`) |
| API, sans requête de plus | `merge_commit_sha` (PR fusionnées seulement), corps de PR (en mémoire) et dépôt d'origine dans `GET /pulls` ; `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` dans `GET /repos` | API |
| API, requête supplémentaire | `GET /repos/{o}/{r}/pulls/{n}/reviews` (phase 8.6, conventions activées : login, identifiant, état et date de chaque relecture, en mémoire ; jamais le corps ; réduits à des compteurs à la synchro) | API |

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
| CC2 | Écarts par motif | **Bloquants** : `empty`, `forbidden` (sujet entier dans la liste interdite, ou préfixe `wip`, `fixup!`, `squash!`, `amend!`), `github_default` (`^(Update\|Create\|Delete\|Rename) \S+$`, « Add files via upload »), `no_type`, `unknown_type`, `type_case`, `separator`, `scope` (scope mal formé, vide ou absent de `commit.scopes`), `trailing_period`, `body_separator`, `length_hard` (> 72 caractères). **Avertissements** : `desc_capitalized` (sauf sigle), `length_soft` (> 50), `footer_format`, `git_revert` (`Revert "…"` produit par Git, compté) | commit ; motif × dépôt, motif × contributeur | base | les deux | H | P1 |
| CC3 | Taux par contributeur | CC1 sur les commits dont il est l'auteur, avec tendance depuis le dernier tag | contributeur, contributeur × dépôt | base | les deux | H | P1 |
| CC4 | Exclusions | Hors dénominateur, comptés à part : fusions (`parent_count ≥ 2`), bots (suffixe `[bot]` et comptes du catalogue), commit racine « Initial commit » créé par GitHub ou Classroom ; `parent_count` inconnu (NULL) : repli sur les messages de fusion par défaut et sur le sujet exact « Initial commit » avec le committer GitHub. Les éditions web (« Update README.md ») et les comptes d'agents IA restent évalués | commit | base | les deux | H | P1 |
| CC5 | Formulation à revoir | Avertissement seulement : `past_tense` (lexique fermé de participes FR et EN), `progressive` (`-ing`), `first_person`. Aucune règle sur la seule terminaison « -é » | commit | base | les deux | M | P2 |
| CC6 | Footers et issues | Part des commits avec un footer valide et part de ceux qui citent une issue (`(Closes\|Fixes\|Resolves\|Refs) #\d+`, casse du mot-clé ignorée, « : » facultatif après lui (`Refs: #12`, forme de pied), cherché dans tout le message, sujet compris) ; informatif | dépôt, contributeur | base | les deux | H | P3 |
| CC7 | Langue des messages | Mélange FR / EN dans un même dépôt, si la charte impose une langue ; informatif | dépôt | base | les deux | M | P3 |

### 5.2 Branches (BR)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| BR1 | Nom de branche conforme | Branches du distant (`refs/remotes/origin/` du clone) et `pull_requests.head_branch` des PR non issues d'un fork (`from_fork`, depuis la phase 8.5 ; une PR d'origine inconnue est comptée), hors branche par défaut et exclusions (`gh-pages`, `dependabot/*`, `renovate/*`, réglage). Conforme si `^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$` ; un seul segment après `/` : « à revoir ». Codes (`--json`) : bloquants `unknown_type` (type inconnu), `uppercase` (majuscule), `underscore` (`_`), `space` (espace), `accent` (accent), `no_slash` (`/` absent), `pattern` (nom qui ne suit pas la règle sans autre motif nommé, `feat/a.b`, ou motif personnalisé non satisfait) ; avertissement `single_segment` (« à revoir ») | branche, dépôt ; contributeur par l'auteur de la PR | clone + base | les deux | H | P1 |
| BR2 | Branche réutilisée | Une `head_branch` (hors fork) avec plus d'une PR fusionnée vers la branche par défaut | branche, dépôt | base | API | M | P2 |
| BR3 | Branche par défaut protégée | **Reporté** (décision du mainteneur : pas mis en œuvre dans la phase 8). Mesure prévue : `protected` de `GET /branches` : oui, non, inconnu (clone), non applicable (par exemple dépôt privé sur GitHub Free). **Instantané** sans historique | dépôt | API, 0 requête | API | M | — |
| BR4 | Branche par défaut réécrite | À la synchro, l'ancienne tête n'est plus ancêtre de la nouvelle (`merge-base --is-ancestor`) ; événement daté (ancien et nouveau SHA). Ancien objet absent du clone : « inconnu », jamais « réécrit ». Seulement si les conventions sont activées, pour la branche par défaut suivie avant et après la synchro et sous le même nom (un changement de branche par défaut n'est pas une réécriture) ; l'ascendance est lue dans le clone mis à jour par la synchro, dans les deux modes. En mode API seulement, quand le clone ne tranche pas (pas de clone, ou commit absent du clone), l'erreur `ErrUnknownBase` de la synchro sur l'ancienne tête (commit disparu du dépôt, ou historiques sans ancêtre commun) sert aussi de signal. Limite : GitHub garde longtemps les commits orphelins, sa comparaison réussit donc souvent après un force-push (`diverged`, `behind`) et l'API seule ne voit pas la réécriture ; en clone seul, `ErrUnknownBase` signifie un commit absent du clone et n'est jamais un signal | événement, dépôt | clone | les deux | H | P2 |
| BR5 | Méthodes de fusion autorisées | `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` du dépôt (renvoyés à un compte qui a le droit push) | dépôt | API, 0 requête | API | M | P3 |

### 5.3 Pull requests (PR)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PR1 | Titre de PR conventionnel | Parseur commun sur le titre des PR hors brouillon et hors bots ; PR ouvertes et fusionnées comptées à part | PR, dépôt, auteur | base | API | H | P1 |
| PR2 | Titre interdit ou par défaut | Titre normalisé dans la liste interdite, ou égal (après normalisation : casse, `-` et `_` en espaces) au nom de la branche de tête ou au sujet du seul commit de la PR (titres proposés par GitHub ; comparaison au seul commit **non mesurée** : il faudrait `GET /pulls/{n}/commits`, une requête par PR). Codes (`default_codes`) : `forbidden`, `github_default`, `head_branch` | PR | base | API | H | P1 |
| PR3 | Commits arrivés sur la branche par défaut hors PR | Parcours `git rev-list --first-parent origin/<défaut>` depuis `since`, sans le commit racine. **Par PR** : SHA égal au `merge_commit_sha` d'une PR fusionnée ; suite rebasée par GitHub, reconnue par une heuristique sans requête (commits consécutifs dont le committer est GitHub, au même `committed_at` que le commit le plus récent de la suite, qui est le `merge_commit_sha` de la PR qui la termine ; à défaut de `merge_commit_sha` connu, `merged_at` de la PR à ±2 s ; décision du mainteneur) ; fusion dont le committer est GitHub. **Hors PR** : committer humain, fusion locale poussée, édition web sans PR. Sinon : « indéterminé ». En clone seul, repli sur `^Merge pull request #\d+` et `\(#\d+\)$` avec le committer GitHub reconnu à son e-mail `noreply@github.com` (en mémoire) | commit ; dépôt ; « auteur du commit arrivé hors PR » (jamais « a poussé ») | clone + base | les deux (dégradé en clone) | H en API, M en clone | P1 |
| PR4 | Relue par un autre membre | Nouvelle méthode Provider `PullRequestReviews` (login, identifiant, état, date ; jamais le corps). Relue si, pour au moins `pr.min_reviews` (1 par défaut) relecteurs distincts (par identifiant), d'un autre identifiant que l'auteur et non exclus (formateur, bots), le dernier état parmi `APPROVED`, `CHANGES_REQUESTED` et `DISMISSED` déposé au plus tard à `merged_at` est `APPROVED`. Un autre compte suffit ; `pr.reviewer_must_contribute` exige en plus des relecteurs auteurs d'au moins un commit du dépôt (reconnus par login ou par identifiant de compte, parmi les commits en base à la lecture des relectures). Approximation : sur GitHub, écarter une relecture ne crée pas de relecture, la relecture `APPROVED` d'origine passe à l'état `DISMISSED` et garde sa date de dépôt ; `DISMISSED` est donc l'état actuel de la relecture, daté de son dépôt et non de son retrait : une approbation retirée après la fusion ne compte plus si les relectures sont lues après le retrait, et compte encore si elles l'ont été avant. `COMMENTED` et `PENDING` ne changent pas l'état d'un relecteur ; un relecteur sans compte (compte supprimé) n'est jamais compté, rien ne le distinguant de l'auteur. Seuls des compteurs sont stockés, pas les relecteurs. Vue dérivée : « fusionnées sans relecture » (numéro et auteur, jamais le titre). Mêmes exclusions que PR1 (fermées sans fusion, brouillons, bots, forks, ouvertes avant la date de début de l'espace ou du dépôt, selon `created_at` : ni lues ni évaluées, aucune requête). Une PR fusionnée dont les relectures n'ont jamais été lues, ou l'ont été avant sa fusion (quand elle était ouverte), est « inconnue », comptée à part (« pas encore lues »), jusqu'à ce qu'une synchro suivante les lise ; clone seul : « inconnu » (`no_api`). Pour une PR ouverte, les compteurs sont donnés en information (« approuvée à la dernière lecture ») ; PR4 ne juge que les PR fusionnées | PR, dépôt ; relectures reçues par l'auteur de la PR | API, 1 requête par PR | API | H | P1 |
| PR5 | Corps structuré et `Closes #n` | Corps lu en mémoire (comme pour D2) : titres de section, ou texte en gras, parmi des synonymes FR et EN versionnés (contexte, ce qui change, comment tester, captures) ; une section vide (ou réduite à un commentaire HTML) ne compte pas. Mot-clé de fermeture cherché à part. Seul un masque de bits est stocké (NULL tant qu'il est inconnu). Faux négatif connu : issue liée par la barre latérale « Development » | PR, dépôt | API, 0 requête | API | M | P2 |
| PR6 | Fusion en squash | Commit de `merge_commit_sha` à un parent avec le suffixe « (#n) » : squash ; deux parents : merge commit. Conforme si squash **et** message conforme au parseur | PR | base | API | M | P2 |
| PR7 | Gabarit de PR | Voir TL3 | dépôt | clone | les deux | H | P2 |

**Phase 8.5 : fusions, réalisation et limites.** PR3, PR5, PR6, BR2 et BR5 sont calculés à la lecture,
depuis la base (colonnes `merge_commit_sha`, `from_fork`, `body_sections` des PR et `allow_*` du dépôt,
remplies par la synchro sans requête de plus, voir la [section 9](#9-impact-technique)) et, pour PR3, le
clone existant (sans fetch). Les PR issues d'un fork (`from_fork` vrai) sont écartées de PR1, PR2, PR5 et
PR6 (motif `fork`) et leurs branches de tête de BR1 et BR2 ; une PR d'origine inconnue (NULL) est évaluée.
Aucun score : des comptes `n/m`. Approximations retenues :

- **PR3** : `git rev-list --first-parent --parents` de `origin/<défaut>` dans le clone, filtré par la **date
  du committer** (`--since`, celle qui approche le mieux l'arrivée sur la branche ; un commit rebasé par
  GitHub a la date de la fusion), sans le commit racine (aucun parent). Ordre des tests en mode API :
  `merge_commit_sha` d'une PR fusionnée vers la branche par défaut (`merge_commit_sha`) ; committer inconnu
  (`committer_unknown`, indéterminé) ; committer humain : hors PR (`human_committer`, ou `local_merge` pour
  une fusion à deux parents) ; fusion du committer GitHub (`github_merge`) ; commit de GitHub au suffixe
  « (#n) » d'une PR fusionnée dont le `merge_commit_sha` est inconnu (`squash_suffix`) ; commit de GitHub au
  suffixe « (#n) » d'une PR **absente** de la liste : « indéterminé » (`unmatched`), même liste relue en
  entier, car la liste des PR peut être en retard sur les commits (reportée pour quota bas ou en échec à cette
  synchro) ; sinon commit de GitHub sans PR : **édition web** hors PR (`web_edit`) seulement si la liste des
  PR a été relue en entier (`repositories.conventions_prs_version`), « indéterminé » (`unmatched`) sinon. Les
  **suites rebasées** par GitHub (`rebase_run`) sont reconnues ensuite, à partir de leur commit le plus
  récent : quand un commit de GitHub (hors fusion) est le `merge_commit_sha` d'une PR, les commits qui le suivent sur
  le premier parent, consécutifs, du committer GitHub, hors fusions, au **même `committed_at` que lui** (à la
  seconde) et classés `web_edit` ou `unmatched` sont rattachés à cette PR ; l'écart d'une ou deux secondes que
  GitHub laisse parfois entre `committed_at` et `merged_at` est donc sans effet. Repli pour une PR sans
  `merge_commit_sha` connu : une suite de tels commits au même `committed_at` est rattachée à la PR dont le
  `merged_at` est le plus proche, à ±2 s, du `committed_at` de son commit le plus récent (la plus petite PR en
  cas d'égalité). Une édition web isolée faite à la seconde d'une fusion sans lien dont le `merge_commit_sha` est connu, ou
  séparée de la tête par un autre commit, reste donc hors PR ; le repli, lui, ne vérifie que la seconde. Commit absent de la base : par PR s'il est le `merge_commit_sha` d'une
  PR, sinon indéterminé (`not_in_db`). **Clone seul** (dégradé) : committer inconnu indéterminé
  (`committer_unknown`) ; committer humain hors PR (`human_committer`, ou `local_merge` pour une fusion locale
  poussée) ; committer GitHub (adresse `noreply@github.com`) avec « Merge pull request #n » (`github_merge`)
  ou « (#n) » (`squash_suffix`) par PR ; tout autre commit de GitHub indéterminé (`clone_only`).
  Le committer GitHub n'est reconnu qu'à son login `web-flow` ou à son adresse : une fusion faite sur GitHub
  par un compte dont l'adresse est publique est un commit de GitHub comme un autre. L'attribution est celle de l'**auteur du
  commit**, jamais de la personne qui l'a poussé ; un rapport restreint à des contributeurs ne garde que
  leurs commits. Sans clone, clone illisible ou branche par défaut absente du clone : « inconnu »
  (`no_clone`, `clone_unreadable`).
- **PR5** : le corps est lu en mémoire à la synchro et réduit à un masque de bits (contexte, ce qui change,
  comment tester, captures, mot-clé de fermeture) ; règles de lecture versionnées par
  `conventions.PRRulesVersion` (la monter fait relire une fois la liste complète des PR). Une section est
  ouverte par un titre ATX (`#` à `######`) **contenant** un synonyme normalisé (minuscules, sans accent ni
  ponctuation), ou par un libellé en gras en début de ligne (`**Contexte**`, `__Tests :__`) **égal** à un
  synonyme (une phrase en gras ne compte pas) ; elle s'arrête au titre suivant de niveau inférieur ou égal
  (ou, ouverte en gras, au titre ou au gras suivant). Elle compte si elle a du contenu : texte après le
  libellé en gras, ou ligne non vide hors séparateur thématique ; le code clôturé compte comme contenu, les
  commentaires HTML (sur une ou plusieurs lignes) sont retirés avant, une case à cocher non cochée d'un
  gabarit (`- [ ] …`) compte comme contenu. Les titres soulignés (Setext) ne sont pas reconnus. Synonymes
  intégrés au binaire : contexte (contexte, context, pourquoi, why, motivation, problème, problem,
  description, objectif, goal, background, résumé, summary, enjeux…), ce qui change (ce qui change, changes,
  changements, what changed, modifications, solution, implementation, approche, approach…), comment tester
  (comment tester, how to test, testing, tests, test plan, plan de test, vérification, validation, recette…),
  captures (captures, capture d'écran, screenshots, images, démo, preview, gif, vidéo, rendu…). Mot-clé de fermeture : `close`,
  `closes`, `closed`, `fix`, `fixes`, `fixed`, `resolve`, `resolves`, `resolved`, avec `:` facultatif, suivi de
  `#n` ou `owner/repo#n`, hors code et commentaires. Conforme si les sections de `pr.body_sections` sont
  présentes et, avec `pr.require_closes`, le mot-clé aussi ; corps vide : masque connu à 0. Faux négatif
  connu : une issue liée par la barre latérale « Development » de GitHub n'apparaît pas dans le corps. Une PR
  synchronisée avant la phase 8.5 reste « inconnue » jusqu'à la relecture complète.
- **PR6** : sur les PR fusionnées évaluées ; commit de `merge_commit_sha` lu en base : deux parents ou plus,
  `merge` ; un parent et le suffixe « (#n) » de **cette** PR, `squash` (conforme si le message est aussi
  conforme au parseur) ; un parent sans suffixe, `rebase` (approximation : un squash dont le sujet a été
  réécrit sans le suffixe est classé ici) ; commit absent de la base, parents inconnus, ou suffixe d'une
  autre PR, `unknown`. Ni le titre ni le message ne sont affichés.
- **BR2** : branches de tête des PR fusionnées vers la branche par défaut, hors forks, bots et branches sans
  nom ; restituée par dépôt (rapport restreint : seulement les PR des contributeurs choisis).
- **BR5** : affiché tel quel (méthodes autorisées), conforme si **seul** le squash est autorisé ; « inconnu »
  (`no_push_access`) si GitHub ne renvoie pas les réglages (compte sans droit push, ou dépôt pas encore
  resynchronisé depuis la phase 8.5) ; en information, sans pastille dans la revue de jalon.

### 5.4 Tags et SemVer (SV)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SV1 | Nom de tag SemVer | `v` minuscule puis version SemVer 2.0.0 (comparateur extrait de `internal/selfupdate` vers `internal/semver`). Codes (`--json`) : bloquants `no_v` (sans `v`), `upper_v` (`V`), `missing_component` (composant manquant), `leading_zero` (zéro en tête), `empty_prerelease` (pré-version vide), `invalid` (tout autre nom) ; avertissement `build_metadata` (`+build`) | tag, dépôt | base | les deux | H | P1 |
| SV2 | Jalons présents | Pour chaque jalon configuré : présent, présent sous un nom non conforme, absent ; une pré-version ne vaut pas jalon. Avec une échéance : le tag de nom exact est « à l'heure » si sa première observation précède la fin de l'échéance, « en retard » si une lecture complète des tags faite à la fin de l'échéance ou après, avant sa première observation, ne l'a pas vu, sinon « indéterminé » (affichage « posé entre X et Y ») ; preuve d'une lecture : voir la phase 8.4 ci-dessous | jalon × dépôt | base | les deux | H (présence), M (échéance) | P1 |
| SV3 | Tag réécrit ou supprimé | Table `tag_observations`, alimentée seulement si les conventions sont activées et que la lecture des tags a réussi, n'est pas tronquée (moins de 1 000 tags) et n'a ignoré aucun tag (un tag absent d'une liste incomplète serait fermé à tort) : même nom avec un autre SHA (réécrit), disparu (supprimé) ; seul un changement de commit visé est vu (les deux sources donnent le SHA du commit visé, pas celui de l'objet tag : un tag annoté recréé sur le même commit est invisible). Libellé : « aucune réécriture observée depuis le … », jamais « jamais réécrit » ; aucune attribution à une personne | tag, dépôt | base | les deux | M | P1 |
| SV4 | Pré-versions | Forme `-rc.N`, `-beta.N`, `-alpha.N` ; les formes libres (`-rc1`, `-RC.1`) donnent un avertissement, pas un écart (décision du mainteneur) ; avertissement aussi si une rc est posée après la version finale ou si la précédence contredit l'ascendance | tag | base + clone | les deux | M | P2 |
| SV5 | Saut cohérent avec les commits | Pour chaque tag final F, base B = plus grand tag final conforme inférieur à F qui en est ancêtre ; commits de `B..F` hors fusions et bots. Rupture : MAJOR attendue (MINOR avant 1.0.0) ; `feat` : au moins MINOR. **Seul le sous-versionnement est signalé** ; « indéterminé » si moins de 80 % des commits de l'intervalle sont conformes ; premier tag non évalué | paire de tags | clone + base | les deux | M | P2 |
| SV6 | Release associée | Hors brouillons : jalon sans release (`milestone_without_release`), release sans tag (`release_without_tag`), drapeau `prerelease` incohérent (`prerelease_mismatch`). **Information seule** : la charte ne parle que de tags, SV6 ne compte pas dans la conformité | tag, release | base | API | H | P2 |
| SV7 | Précédence et historique | Version supérieure posée sur un ancêtre d'une version inférieure ; saut d'incrément (v1 → v3) en information | paire de tags | base + clone | les deux | M | P3 |

**Phase 8.2 : mesure à la volée et limites.** BR1, PR1, PR2, SV1, SV2 (présence), SV6, CC5 à CC7, CC1 par
intervalle entre tags et la tendance de CC3 sont calculés à la lecture, depuis la base et le clone, sans
migration ni requête de plus. Approximations retenues :

- **Forks** : en 8.2, la base ne gardait pas le dépôt d'origine d'une PR ; les branches de tête des PR issues
  d'un fork étaient donc comptées dans BR1, et ces PR dans PR1. Depuis la phase 8.5 (`from_fork`), elles sont
  écartées (motif `fork`) ; une PR d'origine inconnue reste comptée.
- **Mode API** : il n'est pas stocké. PR1, PR2 et SV6 sont « inconnus » pour un dépôt que l'API n'a jamais vu
  (identifiant de forge absent) et pour un espace synchronisé en clone seul ; sinon les PR et releases en base
  sont celles de la dernière synchro par l'API. Les branches de tête des PR ne s'ajoutent à BR1 qu'en mode API.
- **PR1 et PR2** : PR hors brouillons, hors bots et hors PR fermées sans fusion (hors forks depuis la phase
  8.5), ouvertes depuis la date de début. Une PR est rattachée à un contributeur par son compte de forge. La
  comparaison du titre avec le sujet du seul commit de la PR n'est **pas mesurée** : la base ne relie pas les
  commits aux PR, et il faudrait une requête par PR (`GET /pulls/{n}/commits`).
- **BR1** : branches du distant lues dans le clone (`refs/remotes/origin/`, sans mise à jour) et branches de
  tête des PR (hors bots), chaque nom une fois. La règle intégrée donne aussi `pattern` à un nom qui ne la
  suit pas sans autre motif nommé (`feat/a.b`, `feat/-x`). Sans clone, ou clone illisible, repli sur les branches
  **suivies** en base : la liste est partielle (une branche exclue par `branches.exclude` ou par les motifs du
  dépôt n'y figure jamais). Une branche n'est attribuée à une personne que par l'auteur de la PR dont elle est
  la branche de tête.
- **Tags** : lus en base (la synchro les écrit dans les deux modes, avec le SHA du commit visé). Un tag n'a pas
  de date : une échéance borne les commits par leur **date d'écriture** (`authored_at`).
- **CC1 par intervalle** : `git rev-list <tag> ^<tag final précédent>` dans le clone, sans mise à jour ;
  seuls les tags finals conformes bornent un intervalle. Les commits de l'intervalle absents de la base
  (branches non suivies) n'ont pas de message : ils sont comptés à part (« absents de la base ») et, dans la
  revue de jalon, rendent le critère « inconnu » s'il n'y a pas d'autre écart. Un commit de tag absent du clone
  rend l'intervalle « inconnu », jamais mal borné. Sans clone : « inconnu ». Seuls les tags finals bornent un
  intervalle : conformes, sans pré-version **ni métadonnées de build** (`v1.0.0+build`, conforme avec un
  avertissement, n'en borne aucun). Intervalle d'un jalon borné par l'échéance : un commit absent de la base
  n'a pas de date, il est compté parmi les absents même s'il est postérieur à l'échéance (choix prudent :
  « inconnu » plutôt qu'un 100 % non vérifié) ; si aucune des branches suivies n'est dans le clone,
  l'intervalle est illisible (`clone_unreadable`), jamais vide.
- **Rapport restreint à des contributeurs** (`--contributor`) : intervalles non calculés (indicateur de
  dépôt) ; PR1, PR2 et BR1 ne portent que sur les PR de ces contributeurs et leurs branches de tête, les
  branches non attribuées sont écartées : aucune autre personne n'apparaît.
- **Tendance de CC3** : deux taux par contributeur, sur les commits atteignables depuis le dernier tag final
  de leur dépôt et sur ceux arrivés depuis ; un commit d'un dépôt sans clone lisible ni tag final n'est pas
  placé (compté à part).
- **CC7** : heuristique sans dépendance (mots-outils fermés, lettres accentuées) ; beaucoup de sujets courts
  restent « inconnus ».

**Phase 8.4 : historique et versions, réalisation et limites.** BR4, SV3 et les échéances de SV2 sont lus
dans les tables `branch_rewrites` et `tag_observations`, alimentées par la synchro **seulement quand les
conventions sont activées** (minimisation des données) ; SV4, SV5, SV7 et CL10 sont calculés à la lecture,
depuis la base et le clone existant (sans fetch), et ne sont pas stockés. Aucun de ces indicateurs n'est
attribué à une personne. Approximations retenues :

- **Échéances de SV2** : seul le tag de **nom exact** est jugé (un nom non conforme reste « nom non
  conforme »). Fin de l'échéance : minuit du lendemain dans le fuseau de l'espace, borne exclue. Aucune
  donnée existante ne prouve qu'une synchro a lu les tags à un instant donné (`sync_runs` ne dit ni quelle
  liste a été lue, ni si elle était tronquée, ni si les conventions étaient activées) : **la preuve d'une
  lecture complète est une date de `tag_observations`** du dépôt (première observation, dernière
  observation ou fin d'une ligne quelconque), chacune étant l'instant d'une lecture réussie, non tronquée,
  faite avec les conventions activées. « En retard » demande une telle date dans `[fin de l'échéance,
  première observation du tag)` : cette lecture prouve que le tag manquait encore après l'échéance. Une
  lecture antérieure à la fin de l'échéance ne prouve rien (le tag a pu être posé entre elle et l'échéance) ;
  la date du commit visé ne sert pas au jugement. Sinon « indéterminé », affiché « posé entre le X et le Y »
  (Y : première observation ; X : la plus tardive, parmi les dates antérieures à Y, entre la date du commit
  visé et la dernière lecture complète qui n'a pas vu le tag ; la date du commit visé ne compte que si le
  commit est en base et si c'est encore celui de la première observation, un tag réécrit depuis ayant
  d'abord désigné un autre commit ; « une date inconnue » à défaut). Limites : la preuve du retard est rare,
  car la dernière observation d'une ligne est réécrite à chaque lecture et les lectures intermédiaires se
  perdent ; un dépôt sans aucun autre tag, ou dont les lectures n'ont laissé aucune date dans la fenêtre,
  donne « indéterminé », comme un tag jamais observé. On suppose aussi qu'un tag vu manquant après
  l'échéance n'a pas été posé avant puis supprimé entre deux lectures.
- **SV3** : quatre états par tag, `stable` (aucune réécriture observée), `restored` (supprimé puis reposé sur
  le même commit : une suppression a été observée, donc **non conforme** comme `rewritten` et `deleted`,
  décompté à part), `rewritten` (le nom a
  désigné un autre commit) et `deleted` (disparu et pas revenu, l'emporte sur une réécriture antérieure). Une
  ligne de `tag_observations` par période : un couple (nom, SHA) qui revient après une suppression ouvre une
  nouvelle ligne. Libellé : « aucune réécriture observée depuis le <première observation> ». Tag jamais vu par
  une lecture complète (conventions activées après la dernière synchro, lectures tronquées) : « inconnu »
  (`not_observed`).
- **BR4** : restitution par dépôt, nombre d'événements, date de chacun, ancien et nouveau SHA courts ;
  jamais « a poussé ».
- **SV4** : codes `free_form` (ni `-<forme>.N` ni une forme de `tags.prerelease_forms`), `after_final`
  (pré-version posée après la version finale de mêmes MAJOR.MINOR.PATCH : la finale est un ancêtre strict de
  la pré-version dans le clone ; si l'ascendance ne tranche pas, même commit, commits sans lien ou clone
  absent, la pré-version a été observée pour la première fois après la finale) et `order` (de deux
  pré-versions successives par précédence d'une même version, la plus grande est posée sur un ancêtre strict
  de la plus petite ; clone seulement). Tous sont des **avertissements**, jamais des écarts. Seules les
  pré-versions de nom conforme (SV1) sont évaluées.
- **SV5** : bases cherchées par précédence décroissante, arrêt à la première ascendance illisible
  (`clone_unreadable`) ; aucun ancêtre : `no_base`. Commits de `B..F` lus dans le clone puis en base ; sont
  écartés les fusions (nombre de parents, ou message de fusion par défaut s'il est inconnu) et les bots,
  **pas** les comptes de `excluded_accounts` ni les commits antérieurs à la date de début. Seuls les commits
  conformes au parseur strict déclarent un type ou une rupture. La règle « MINOR suffit pour une rupture »
  s'applique quand la base est en `0.y.z`. « Indéterminé » si des commits de l'intervalle manquent en base
  (`partial`) ou si moins de 80 % des commits évalués sont conformes (`low_conform`) ; un intervalle sans
  commit évalué est « cohérent ». Le sur-versionnement n'est jamais signalé. Sans clone : « inconnu »
  (`no_clone`).
- **SV7** : seules les versions finales conformes **successives** par précédence sont comparées (une
  incohérence entre versions non successives, toutes deux sans lien avec la version qui les sépare, n'est
  pas vue) ; `precedence_ancestry` (version supérieure posée sur un ancêtre strict de la version inférieure)
  est l'écart, `increment_skip` une information, calculée aussi sans clone et étendue aux sauts de PATCH et
  de MINOR (`v1.0.0` → `v1.0.2`). Deux tags sur le même commit sont cohérents.
- **CL10** : dernier commit de l'historique de la tête connue de la branche par défaut qui a touché
  `CHANGELOG.md` à la racine (`git --literal-pathspecs log -1 --format=%H --end-of-options <tête> --
  CHANGELOG.md`), puis commits de la tête non atteignables depuis lui ; mêmes exclusions que SV5. Le décompte
  est un minimum quand des commits manquent en base (`missing`). **Grisé** (information) si CL9 du dernier
  jalon posé (plus grande version parmi les jalons dont un tag existe) est conforme dans le cache de la
  synchro. Inconnu sans clone (`no_clone`), sans `CHANGELOG.md` à la racine de la tête ou sans commit qui
  l'a touché (`no_file` ; présence lue d'abord par `git --literal-pathspecs ls-tree -z --end-of-options
  <tête> -- CHANGELOG.md` : un CHANGELOG supprimé n'est pas compté depuis sa suppression) ou si la tête
  n'est pas connue en base (`clone_unreadable`). Calculé à la lecture : `FileRulesVersion` ne change pas.
- **Lecture du clone** : par une interface optionnelle (`HistoryReader` : `IsAncestor`, `LastTouch`), sans
  mise à jour ; chaque test d'ascendance est mémorisé pour un calcul. Une erreur (commit absent du clone,
  clone illisible) vaut « inconnu », jamais « non ».
- **Rapport restreint à des contributeurs** (`--contributor`) : SV4, SV5, SV7 et CL10 ne sont pas calculés
  (`filtered`) ; BR4, SV3 et les échéances, lus en base, restent dans le JSON ; le texte de la CLI et la page
  d'un contributeur ne les affichent pas.

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
| CL11 | Liens de comparaison | Définition `[X]: url` pour chaque titre ; piste, hors total | dépôt | clone | les deux | M | P3 |
| CL12 | Recopie de `git log` | Au moins 10 puces, dont au moins 60 % identiques à un sujet de commit, ou au moins 20 % de SHA ou de « Merge » ; seuls les pourcentages et deux effectifs sont stockés ; piste « à relire » | dépôt | clone + base | les deux | F | P3 |

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
| DOC6 | En-têtes de document | Titre, objectif, périmètre, auteur, date, version : front matter ou lignes « Clé : valeur » dans les 40 premières lignes ; compteurs par champ, jamais les valeurs ; piste | dépôt | clone | les deux | M | P2 |
| DOC7 | Schémas et images | Images relatives présentes ou cassées, images externes (badges exclus), blocs `mermaid`, dossier `Media/` ; piste | dépôt | clone | les deux | H | P2 |
| DOC8 | Archives | `.zip`, `.rar`, `.7z`, `.tar`, `.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz` (casse ignorée) **dans tout le dépôt** (décision du mainteneur) ; les `.docx` et `.pdf` sous `documentation/` sont signalés en information | dépôt, tag | clone | les deux | H | P1 |
| DOC9 | Fraîcheur | Dates du dernier commit sur `documentation/`, l'INDEX, le CHANGELOG et le code ; commits hors documentation depuis ; piste | dépôt | clone | les deux | M | P2 |
| DOC10 | Dates déclarées | Date de la colonne de l'INDEX comparée au dernier commit du document ; au plus 50 documents ; piste | document | clone | les deux | F | P3 |

Les listes de DOC3, DOC4 et DOC5 (chemins) et celle des images cassées de DOC7 sont recalculées à
l'affichage ; seuls les compteurs sont stockés.

### 5.8 Outillage (TL)

| id | Indicateur | Règle | Granularité | Source | Mode | Fiab. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TL1 | commitlint configuré | Fichier de configuration (profondeur ≤ 2) ou clé `commitlint` / `@commitlint/cli` dans `package.json` ; sous-état « étend config-conventional » ; date d'introduction | dépôt | clone | les deux | H | P2 |
| TL2 | Hook `commit-msg` versionné | `.husky/commit-msg` (avec `prepare: husky`), lefthook, pre-commit en `commit-msg`, cz, cog. Statut « configuré », **jamais « actif »** | dépôt | clone | les deux | M | P2 |
| TL3 | Gabarit de PR | Emplacements reconnus par GitHub, casse ignorée ; nombre de sections de la charte présentes. Absent : « non trouvé dans le dépôt » (un gabarit d'organisation est invisible) | dépôt | clone | les deux | H | P2 |
| TL4 | CI vérifiant les conventions | Un workflow cite une action connue (commitlint, semantic-pull-request…) ; seul l'identifiant du motif est gardé ; piste | dépôt | clone | les deux | M | P3 |
| TL5 | Effet de l'outillage | CC1 avant et après la plus ancienne introduction parmi TL1, TL2, TL4 ; « n/a » si un effectif est inférieur à 10 ; piste | dépôt | base + clone | les deux | M | P3 |

**Phase 8.3 : précisions des critères de fichiers.** Les fonctions pures `CheckChangelog`, `CheckReadme`,
`CheckIndex`, `CheckDocsTree` et `DetectTooling` lisent la liste des chemins de l'arbre et le contenu des seuls
fichiers qu'elles demandent (CHANGELOG, README, INDEX, documents `.md` de la documentation, au plus 300 pour
DOC4, fichiers d'outillage et gabarits de PR). Un résultat ne garde que des identifiants, des états, des codes
et des compteurs ; les listes de chemins (DOC3, DOC4, DOC5, DOC8) sont rendues à part. Leur version de règles,
`FileRulesVersion`, est distincte de celle des commits : un changement des règles de commit n'invalide pas le
cache des fichiers. Choix de détail :

- **Fichier absent** : le critère de présence (CL1, RD1, DOC1) est non conforme et les critères qui lisent le
  fichier sont « inconnus » (`no_file`), pour ne compter l'absence qu'une fois. Un fichier mal nommé ou placé
  ailleurs (`misnamed`, `elsewhere`) est quand même analysé. Arbre tronqué au plafond ou dépôt vide : tous les
  critères sont « inconnus » ; fichier au-delà du plafond de lecture ou non lu : « inconnu » aussi.
- **Analyse Markdown** : titres ATX seulement (pas de titres soulignés), hors blocs de code clôturés et
  commentaires HTML ; liens `[x](y)`, définitions `[x]: y` et tableaux GFM (ligne de délimitation du même
  nombre de colonnes que l'en-tête). Titres comparés après normalisation (casse, accents, ponctuation,
  numérotation de tête) à des synonymes français et anglais versionnés avec `FileRulesVersion`.
- **CL6** : l'ordre des catégories est un **avertissement** (`category_order`), pas un écart : le CHANGELOG de
  Watchoo, cas conforme des tests, a une version ancienne aux catégories dans un autre ordre, et Keep a
  Changelog ne l'impose pas. Inconnues, doublons et puces hors catégorie restent des écarts. Les libellés
  français (catégories, H1, `[Non publié]`) donnent l'avertissement `french_label` quand
  `changelog.categories_fr` est vrai, un écart sinon ; `[v1.2.0]` donne l'avertissement `v_prefix`.
- **CL7** : `[Unreleased]` n'est pas évalué du tout (catégorie vide ou « TODO » compris).
- **CL8** : seuls les noms des tags sont lus (tag qui désigne une version finale, même sous un nom non
  conforme) ; l'écart de date entre un tag et sa section n'est pas encore affiché. Comme les tags changent sans
  que la tête change, leurs noms entrent dans l'empreinte des réglages du cache. Sur l'arbre d'un jalon, seuls
  les tags et sections de précédence inférieure ou égale comptent.
- **CL4** : une date future l'est par rapport au jour du calcul ; le résultat en cache n'est revu qu'au
  prochain calcul.
- **CL9** n'est mesuré que sur l'arbre du tag d'un jalon (la section est cherchée par sa version, quelle que
  soit la forme du titre, jugée par CL4).
- **RD2** : un fichier de licence à la racine vaut section licence (`license_file`) : `LICENSE`, `LICENCE` ou
  `COPYING`, casse ignorée, sans extension ou avec l'extension `.md` ou `.txt`. Seuls un
  masque des sections trouvées et des nombres sont gardés.
- **RD3** : un lien vers un `index.md` à la racine ou dans un dossier `doc`, `docs` ou `documentation` compte
  comme lien vers l'INDEX ; un lien qui ne vise la cible qu'à la casse près est mort (GitHub distingue la
  casse). Une URL du même dépôt est `https://github.com/<owner>/<name>` (page du dépôt) ou
  `…/blob|tree|raw/<ref>/<chemin>` (référence d'un seul segment), comparée au nom actuel du dépôt, casse
  ignorée : exception documentée à la règle « rien de propre à GitHub hors `internal/provider/` »
  (comparaison de texte, sans accès à la forge), comme le catalogue d'`internal/aisignature`.
- **DOC3** : les lignes en double ne sont comptées que dans les tableaux complets (les cinq colonnes de DOC2).
  Sans INDEX à l'emplacement attendu, la documentation évaluée est le dossier de l'INDEX trouvé.
- **DOC4** : les images (`![x](y)`) relèvent de DOC7, pas de DOC4. Aucun lien mort parmi les fichiers lus,
  mais des fichiers non lus : « inconnu » (`unread_documents`).
- **DOC5** : tous les sous-dossiers de la documentation suivent `NN-thème`, sauf le dossier des médias (casse
  ignorée) et son contenu ; les fichiers des médias suivent la règle des fichiers. Fichiers cachés exemptés.
- **TL1** : profondeur au plus 2 dossiers, hors `node_modules/` et `vendor/` ; la date d'introduction (qui
  demande l'historique) n'est pas encore mesurée. **TL2** : `.husky/commit-msg` sans script `prepare` qui
  installe husky donne l'avertissement `husky_no_prepare`. **TL3** : seuls les `.md` sont reconnus.

**Codes, compteurs et raisons des critères de fichiers (`--json`).** Liste complète, figée par
`FileRulesVersion` ; un même code garde le même sens d'un critère à l'autre. Un compteur dont la clé est un
code compte les éléments (versions, catégories, puces, liens, chemins) qui portent ce code.

| Critère | Codes bloquants | Avertissements | Compteurs |
| --- | --- | --- | --- |
| CL1 | `absent`, `misnamed` (autre casse ou extension à la racine), `elsewhere` (ailleurs dans le dépôt) | — | — |
| CL2 | `no_h1`, `h1_title` (titre de niveau 1 autre que « Changelog »), `no_keepachangelog`, `no_semver` | `french_label` | — |
| CL3 | `unreleased_absent`, `unreleased_misplaced`, `unreleased_duplicate`, `unreleased_no_brackets` | `french_label` | — |
| CL4 | `no_brackets`, `date_fr`, `no_date`, `invalid_date`, `future_date`, `invalid_version` | `v_prefix` | `versions` ; un compteur par code |
| CL5 | `version_order`, `date_order` | — | `versions`, `version_order`, `date_order` |
| CL6 | `unknown_category`, `duplicate_category`, `bullet_outside_category` | `french_label`, `category_order` | `sections` ; un compteur par code |
| CL7 | `empty_version`, `empty_category`, `empty_bullet` | — | `versions` ; un compteur par code |
| CL8 | `tag_without_section`, `section_without_tag` | — | `tags`, `tag_without_section`, `section_without_tag` |
| CL9 | `version_missing` | — | — |
| RD1 | `absent`, `misnamed`, `elsewhere` | `translation_only` | `translations` |
| RD2 | `missing_section` | — | `expected`, `found`, `expected_mask`, `found_mask`, `license_file` (1 si un fichier de licence est à la racine), `missing_section` |
| RD3 | `index_link_absent`, `index_link_dead`, `index_link_folder`, `changelog_link_absent`, `changelog_link_dead` | — | — |
| RD4 | `too_short` | — | `words`, `min_words` |
| DOC1 | `absent`, `misnamed`, `elsewhere` | — | — |
| DOC2 | `no_table`, `missing_column`, `no_rows` | — | `tables`, `rows`, `columns`, `missing_column` |
| DOC3 | `unreferenced`, `duplicate_row` | — | `documents`, `unreferenced`, `duplicate_row` |
| DOC4 | `dead_link`, `case_mismatch` | — | `links`, `sources`, `unread`, `dead_link`, `case_mismatch` |
| DOC5 | `dir_name`, `file_name` | — | `dirs`, `files`, `dir_name`, `file_name` |
| DOC8 | `archive` | `office_document` | `archive`, `office_document` |
| TL1 | `not_found` | — | `configs`, `extends_conventional` (1 si une configuration étend config-conventional), `unread` |
| TL2 | `not_found` | `husky_no_prepare` | `tools_mask`, `unread` |
| TL3 | `not_found` | — | `templates`, `expected`, `sections` (le plus grand nombre de sections de la charte trouvées dans un gabarit), `unread` |

- **Masques** : bit `i` (valeur `1 << i`) pour le `i`-ième élément, à partir de 0. `expected_mask` et
  `found_mask` suivent l'ordre des sections du README : `presentation`, `group`, `stack`, `install`, `run`,
  `env`, `tree`, `license`. `tools_mask` suit l'ordre des outils de TL2 : `husky`, `lefthook`, `pre-commit`,
  `commitizen`, `cocogitto`.
- **Raisons d'une famille** (`reason` de `changelog`, `readme`, `docs`, `tooling`, reprise par tous ses
  critères) : `not_computed` (pas encore calculé : lancez `watchoo sync`), `stale` (calculé pour une autre
  révision, une autre version des règles ou d'autres réglages : à recalculer), `no_clone`, `filtered`
  (rapport restreint à des contributeurs).
- **Raisons d'un critère** dans une famille calculée : `empty_repository` (dépôt vide), `tree_truncated`
  (liste des chemins tronquée au plafond), `no_file` (le fichier lu par le critère est absent, son absence est
  comptée par CL1, RD1 ou DOC1), `blob_too_large` (fichier au-delà de 1 Mio), `blob_unreadable` (fichier
  listé mais non lu), `no_docs_dir` (dossier de documentation absent), `unread_documents` (DOC4 : aucun lien
  mort parmi les fichiers lus, d'autres non lus).
- **Raisons des listes** (`docs.lists.reason`, vue détaillée d'un dépôt) : `not_computed`, `stale`,
  `no_clone`, `clone_unreadable` (arbre illisible dans le clone), `tree_truncated`.
- **Revue de jalon** (`milestone.repositories[].files.reason` et `reason` de ses critères de fichiers) : les
  raisons de SV1 quand le tag du jalon est absent (`no_tag`, `not_due`, `tag_absent`), puis `not_computed`,
  `stale`, `no_clone`, et dans une révision calculée les raisons d'un critère ci-dessus.

**Phase 8.7 : pistes.** CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4 et TL5 sont des **pistes** : elles
invitent à relire et ne jugent pas. Elles ne comptent jamais dans les grilles « k/n critères conformes », ni
dans la revue de jalon, ni dans aucun taux ; elles n'ont ni code bloquant ni avertissement, ne sont attribuées
à personne et s'affichent dans un bloc « Pistes » séparé, seulement quand `conventions.enabled` est vrai.
Aucun réglage nouveau, aucune requête API, aucun `fetch`. États (`--json`) : `none` (rien à relire), `review`
(à relire), `not_applicable` (sans objet ou effectif trop petit) et `unknown`, avec une raison.

- **Où et quand.** CL11, CL12, RD5, DOC6, DOC7 et TL4 sont calculées à la synchro sur l'arbre de la tête de
  la branche par défaut (jamais sur l'arbre d'un tag de jalon) et rejoignent le cache `repository_conventions`
  de la tête : `FileRulesVersion` passe à 2 et le cache ne garde que des identifiants, des codes et des
  compteurs. DOC9, DOC10 et TL5 sont calculées à la lecture sur l'historique du clone, seulement dans la vue
  détaillée d'un dépôt (`watchoo conventions owner/repo`, page du dépôt) ; leurs dates ne sont jamais
  stockées. Lectures du clone, en lecture seule et avec `--end-of-options` : `git log -1` sur des chemins
  (pathspecs littéraux, avec exclusions), `git rev-list --count` et `git log --diff-filter=A` (premier ajout
  d'un fichier) ; les variables `GIT_*_PATHSPECS` héritées sont retirées de l'environnement de git. Une erreur du clone ne recopie jamais stderr ni un chemin ; elle rend la piste inconnue
  (`clone_unreadable`) sans être affichée ni journalisée.
- **CL11** : sont comptés les titres `[Unreleased]` et les titres de version lisibles ; un titre a son lien
  quand une définition `[X]: url` non vide porte le même libellé (casse et préfixe `v` ignorés) ou quand le
  titre est lui-même un lien (`## [1.0.0](url)`). Un titre sans lien : `missing_link`. Aucun titre de
  version ni `[Unreleased]` : `not_applicable` (`no_version`), rien à comparer.
- **CL12** : les sujets de commit sont lus dans la base à la synchro, en mémoire, jamais stockés ni
  journalisés ; normalisation : suffixe `(#n)` retiré, puis casse, accents et ponctuation. Un SHA fait 7 à 40
  caractères hexadécimaux, avec au moins un chiffre et une lettre ; « Merge » est reconnu sur le texte
  normalisé. Sous 10 puces : `not_applicable` (`too_few`). Seuls des nombres sont stockés : les deux
  pourcentages, arrondis à l'entier inférieur, et deux effectifs (`bullets`, `subjects`), aucun texte.
- **RD5** : un bloc de commandes est un bloc de code sans info-string ou d'info-string shell, sous un titre
  installation ou lancement (mêmes synonymes que RD2) ou sous l'un de leurs sous-titres. Des variables
  d'environnement sont annoncées par la section de RD2 ou par la mention de `.env` hors code (code en ligne
  compris), comme mot à part : `.env.example` compte, pas `.envrc`, `process.env`, `import.meta.env` ni une
  URL qui contient `/.env` ; un modèle
  `.env.example` (ou `.sample`, `.dist`, `.template`, éventuellement `.env.<x>.example`) est cherché à une
  profondeur d'au plus 2.
- **DOC6** : documents du dossier de documentation de DOC4, hors README et INDEX (au plus 300). Les 40
  premières lignes sont lues en octets bruts, hors blocs de code clôturés (```` ``` ```` ou `~~~`) ; sont
  reconnus un titre de niveau 1 (champ titre), le front matter YAML (clé suivie d'une valeur ou d'une liste),
  les lignes « Clé : valeur » (en gras, en puce ou en citation) et les lignes de tableau `| Clé | valeur |`,
  avec des clés françaises et anglaises. Seuls des compteurs par champ sont gardés, **jamais les valeurs**
  (l'auteur est une donnée personnelle).
- **DOC7** : sources de DOC4 ; images `![x](y)`, images incluses dans un lien et `<img src>`. Une image
  relative (ou une URL du même dépôt) absente, ou présente à la casse près, est cassée (`broken_image`) ; une
  image externe qui n'est pas un badge (hôte connu ou « badge » dans l'URL) est `external_image` ; sont
  comptés aussi les blocs `mermaid` et l'existence du dossier des médias (casse ignorée). La liste des images
  cassées (source, cible) est recalculée en vue détaillée, comme les listes de DOC3 et DOC4, jamais stockée.
- **TL4** : workflows `.github/workflows/*.yml` et `*.yaml` (sans sous-dossier), lignes de commentaire
  ignorées. Motifs reconnus, dans l'ordre des bits de `ci_mask` : `commitlint`, `semantic-pull-request`,
  `commitizen`, `cocogitto`, `conventional-commits` ; seul ce bit est gardé. Aucune vérification trouvée :
  `not_found`.
- **DOC9** : la documentation comprend le dossier évalué (celui de l'INDEX, sinon `docs.dir`), l'INDEX, le
  CHANGELOG et le README ; le code est tout le reste. Dates gardées à l'affichage : `docs`, `index`,
  `changelog`, `code`. À relire (`docs_behind`) si au moins 20 commits hors documentation ont suivi son
  dernier commit, ou si le dernier commit du code a 30 jours d'avance ou plus. Aucun commit de documentation :
  inconnu (`no_file`).
- **DOC10** : tableaux de l'INDEX qui ont une colonne document et une colonne date (synonymes de DOC2) ;
  dates `AAAA-MM-JJ` ou `JJ/MM/AAAA` (séparateurs `-`, `/` ou `.` admis dans les deux formes) ; une ligne
  dont la cellule date est vide n'est pas comptée (DOC2 relève les cellules manquantes). Le document visé est le premier lien de
  la cellule, résolu et présent dans l'arbre ; un document n'est comparé qu'une fois, 50 au plus
  (`over_limit` au-delà). Comparaison au jour près, dans le fuseau de l'espace : `declared_newer` (date
  déclarée plus récente que le dernier commit), `declared_older` (plus ancienne de plus de 30 jours),
  `date_unreadable`. La cellule n'est jamais gardée. Sans tableau à ces deux colonnes : `not_applicable`
  (`no_date_column`).
- **TL5** : l'introduction est le plus ancien premier ajout (`git log --diff-filter=A`, le plus ancien en
  cas de suppression puis de réajout) parmi les fichiers sources de TL1, TL2 et TL4 : configuration
  commitlint, `.husky/commit-msg`, lefthook ou pre-commit avec une entrée `commit-msg`, `.cz*`, `cog.toml`,
  workflows qui citent une vérification connue. Commits pris en compte : les commits évalués du périmètre
  (toutes les branches suivies), datés par le committer, à défaut par l'auteur ; taux CC1 avant et depuis.
  Aucune source : `not_applicable` (`no_tooling`) ; aucune source datée : inconnu (`introduction_unknown`) ;
  un effectif sous 10 : `not_applicable` (`too_few`), compteurs donnés ; taux après pas plus haut qu'avant :
  `no_gain`. Rien n'est attribué à une personne.
- **Raisons propres aux pistes** : `detail_only` (DOC9, DOC10 et TL5 hors vue détaillée), `too_few`, `no_version`
  (CL11), `introduction_unknown`, `no_tooling`, `no_date_column` ; s'y ajoutent les raisons des critères de fichiers
  (`not_computed`, `stale`, `no_clone`, `clone_unreadable`, `filtered`, `no_file`…). Une piste de l'arbre
  absente du cache prend la raison de sa famille, sinon `stale`.

| Piste | Codes | Compteurs | Dates |
| --- | --- | --- | --- |
| CL11 | `missing_link` | `titles`, `linked`, `missing_link` | — |
| CL12 | `copied_subjects`, `sha_or_merge` | `bullets`, `subjects`, `subject_percent`, `sha_merge_percent` | — |
| RD5 | `no_install_section`, `no_command_block`, `no_env_example` | `command_blocks`, `env_announced`, `env_example` (1 ou 0) | — |
| DOC6 | `missing_headers` | `documents`, `complete`, `header_<champ>` (`title`, `objective`, `scope`, `author`, `date`, `version`), `missing_headers`, `unread` | — |
| DOC7 | `broken_image`, `external_image` | `sources`, `images`, `badges`, `mermaid`, `media_dir` (1 ou 0), `broken_image`, `external_image`, `unread` | — |
| DOC9 | `docs_behind` | `commits_since`, `days_behind` | `docs`, `index`, `changelog`, `code` |
| DOC10 | `declared_newer`, `declared_older`, `date_unreadable` | `rows`, `compared`, `unresolved`, `over_limit` ; un compteur par code | — |
| TL4 | `not_found` | `workflows`, `ci_mask`, `unread` | — |
| TL5 | `no_gain` | `sources`, `undated_sources`, `before`, `before_conform`, `after`, `after_conform`, `before_percent`, `after_percent` | `introduced` |

**Approximations de la phase 8.7.**

1. **DOC9, DOC10 et TL5 seulement en vue détaillée** : elles demandent l'arbre et l'historique de la tête au
   moment de la lecture ; ailleurs (`watchoo conventions` sans dépôt, filtre contributeur) elles valent
   « inconnu » (`detail_only`, `filtered`).
2. **Date d'introduction de TL5** : elle ne se mesure que pour un fichier. Une clé de `package.json` ou de
   `pyproject.toml` n'est pas datée (`undated_sources`) ; pour un workflow ou un fichier lefthook, la date est
   celle du premier ajout du fichier, donc au plus tôt (la vérification a pu être ajoutée plus tard).
3. **CL12 calculé à la synchro** : les sujets viennent de la base ; si la branche par défaut n'est pas suivie,
   des sujets manquent et les pourcentages sont sous-estimés. La valeur est figée jusqu'au prochain mouvement de la tête. De même, branche par défaut non suivie, DOC9,
   DOC10 et TL5 prennent pour tête le commit dont la synchro a mesuré l'arbre (celui du cache) ; sans
   résultat en cache, elles valent « pas encore calculé » (`not_computed`).
4. **Montée de `FileRulesVersion`** : après la mise à jour, toutes les grilles CL, RD, DOC et TL d'un espace
   passent à « à recalculer » (`stale`) jusqu'à la synchro suivante. Les résultats des tags de jalon partagent
   cette version : les critères de fichiers de la revue de jalon (CL9, CL8, RD1 à RD3, DOC1, DOC3) sont
   aussi « à recalculer » jusqu'à la synchro suivante, bien que les tags n'aient pas de pistes.
5. **Pas de compteur de pistes dans `/conventions`** : DOC9, DOC10 et TL5 y vaudraient toujours
   `detail_only`, un total serait trompeur ; les pistes ne sont que sur la page d'un dépôt et dans
   `watchoo conventions owner/repo` (titre, pistes à relire et renvoi vers la vue détaillée en vue d'ensemble).

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
détail et ne trie ni ne classe rien. Les pistes de la phase 8.7 (CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10,
TL4, TL5) n'entrent dans aucune grille ni aucun taux : elles forment un bloc « Pistes » à part.

- **Par dépôt** : CC1, BR1, PR1, PR3, PR4, SV1, SV2, puis les grilles CHANGELOG (CL1 à CL8 ; CL9 sur le tag
  d'un jalon), README (RD1 à RD4), documentation (DOC1 à DOC5, DOC8) et outillage (TL1 à TL3).
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

**Phase 8.2 : première revue de jalon.** Pour un jalon configuré (`tags.milestones`), chaque dépôt reçoit
une liste de critères `conform`, `nonconform` ou `unknown` (conforme, non conforme, inconnu), sans note globale ni tri (« k/n critères
conformes, u inconnus ») ; ce sont des indicateurs de dépôt, attribués à personne. Seuls les critères mesurés
en 8.2 apparaissent :

- **CC1** sur l'intervalle du jalon : du tag final précédent (précédence SemVer) au tag qui désigne le jalon
  (nom exact ou non conforme) ; tag absent : commits des branches suivies depuis le tag final précédent écrits
  avant la fin de l'échéance, « inconnu » avant cette fin (`not_due`), « inconnu » (`no_tag`) sans échéance. Conforme si tous
  les commits évalués le sont ; « inconnu » sans clone, sans commit évalué, ou si des commits de l'intervalle
  sont absents de la base ;
- **BR1**, état courant : conforme si aucune branche n'a d'écart bloquant ; « inconnu » sans branche à évaluer ;
- **SV1** du tag qui désigne le jalon ; tag absent : « inconnu » (`no_tag` sans échéance, `not_due` avant la
  fin de l'échéance, `tag_absent` après : aucun nom à juger, l'absence est comptée par SV2) ;
- **SV2** : conforme si le tag de nom exact existe ; tag absent avant la fin de l'échéance : « inconnu ».

Sans date de début (`conventions.since` de l'espace ou du dépôt), le dépôt est « non évaluable : date de début
manquante ». PR3 s'ajoute avec la phase 8.5, PR4 avec la phase 8.6 (ci-dessous).

**Phase 8.4 : échéance de SV2 et SV3 dans la revue.** Avec une échéance, **SV2** juge aussi la pose du tag de
nom exact : « à l'heure » conforme, « en retard » non conforme, « indéterminé » inconnu
(`deadline_undetermined`). **SV3** suit SV2 (le critère « tags de jalon jamais réécrits » combine SV1, SV2 et
SV3) : conforme si aucune réécriture ni suppression du tag qui désigne le jalon n'a été observée (stable), non
conforme s'il a été réécrit ou supprimé, même reposé ensuite sur le même commit ; tag absent : « inconnu » avec la
raison de SV1 ; tag jamais observé : « inconnu » (`not_observed`).

**Phase 8.5 : PR3 dans la revue.** Le critère **PR3 = 0** suit BR1 (critère « branches par fonctionnalité,
PR relues ») : premier parent du tag qui désigne le jalon (tag absent : de la branche par défaut, committé
avant la fin de l'échéance), moins les commits du tag final précédent, depuis la date de début, tous auteurs.
Conforme si aucun commit n'est arrivé hors PR et qu'aucun n'est indéterminé ; non conforme dès qu'un commit
est arrivé hors PR ; « inconnu » sans clone (`no_clone`), clone illisible (`clone_unreadable`), sans commit
(`no_commits`), avec des commits d'arrivée indéterminée (`undetermined`, fréquent en clone seul), ou dans les
cas `no_tag` et `not_due` de CC1. La revue compte donc treize critères.

**Phase 8.6 : PR4 dans la revue.** Le critère **PR4** suit PR3 (même critère « branches par
fonctionnalité, PR relues ») : toutes les pull requests fusionnées dans l'intervalle du jalon sont relues
(règle de [5.3](#53-pull-requests-pr), réglages appliqués à la lecture), sur les PR fusionnées évaluées (ni
fermées sans fusion, ni brouillons, ni bots, ni forks, ni ouvertes avant la date de début). L'intervalle
est borné **par des dates**, ce qui fonctionne avec l'API seule : fusion postérieure à la date du commit
visé par le tag final précédent et à la date de début, jusqu'à la date du commit visé par le tag du jalon
(comprise) ou, tag absent, avant la fin de l'échéance. Approximation : la date du commit visé (date du
committer, lue en base) tient lieu de date de pose du tag ; la PR dont la fusion a produit le commit du
tag du jalon est comptée même si son `merged_at` suit cette date de quelques secondes, et celle dont la
fusion a produit le commit du tag précédent est laissée au jalon précédent. Non conforme dès qu'une PR
fusionnée de l'intervalle est connue et non relue (ses numéros sont donnés, jamais un titre) ; « inconnu »
en clone seul ou pour un dépôt jamais vu par l'API (`no_api`), tant que des relectures de PR de
l'intervalle ne sont pas lues (`reviews_unchecked`), sans PR fusionnée (`no_merged_pr`), si la date du
commit d'un tag est absente de la base (`no_tag_date`), ou dans les cas `no_tag` et `not_due` de CC1. La
revue compte donc quatorze critères.

**Phase 8.3 : critères de fichiers de la revue.** Sept critères suivent SV2, lus dans le cache de la
synchro pour **l'arbre du tag** qui désigne le jalon (même règle que SV1 : nom exact, sinon premier nom non
conforme) : **CL9** et **CL8** (« CHANGELOG à jour au jalon »), **RD1**, **RD2** et **RD3** (« README avec
installation » ; dans la revue, RD2 ne juge **que la section installation** : conforme si son titre est
trouvé ; les autres sections trouvées sont données en information, `readme_found`), **DOC1** et **DOC3**
(« INDEX exhaustif »). Tag absent : les sept critères sont « inconnus » avec la raison de SV1 (`no_tag`,
`not_due`, `tag_absent`). Aucun résultat pour le tag : « inconnu » (`not_computed` : pas encore calculé,
lancez `watchoo sync` ; `no_clone` sans clone local) ; résultat d'un autre commit (tag déplacé), d'une autre
`FileRulesVersion` ou d'une autre empreinte des réglages : « inconnu » (`stale`, à recalculer). Un fichier
absent de l'arbre compte une fois (RD1 ou DOC1 non conforme), les critères qui le lisent étant « inconnus »
(`no_file`).

### 7.2 Interfaces

- **Web** : route `/conventions` (vue d'ensemble et revue de jalon), entrée de navigation seulement si
  `conventions.enabled`. Section « Conventions » sur la page du dépôt (badges par PR, pastille par tag, bloc
  « Outillage ») ; ligne « Commits conformes 18/25 (72 %) » sur la page du contributeur ; pastille compacte
  sur le tableau de bord, sans tri par cette valeur. Icônes toujours accompagnées d'un texte.
  Phase 8.2 : `/conventions` montre la vue d'ensemble par dépôt (ordre de l'espace) et par contributeur (par
  nom), jamais triée par conformité, et la revue du jalon choisi par `?milestone=vX.Y.Z` (premier jalon
  configuré par défaut, 400 pour un jalon non configuré) ; désactivée, elle explique comment activer les
  conventions, sans aucune donnée. La section du dépôt ajoute les branches non conformes, un badge par PR
  (numéro et codes, jamais le titre), une pastille par tag, les jalons, les releases (SV6, information) et
  CC1 par intervalle entre tags ; la page du contributeur ajoute les titres de PR conformes et la tendance
  de CC3 (deux taux). La pastille compacte du tableau de bord n'est pas encore là. Détail dans
  [06-interface-web.md](06-interface-web.md#7-conventions-180-phase-82).
- **CLI** : une seule commande, sans sous-commande (un dépôt nommé comme une sous-commande serait ambigu) :
  `watchoo conventions [repo] [--commits] [--failing] [--contributor <x>] [--milestone vX.Y.Z] [--json]`.
  `--commits --failing` liste le SHA court, l'auteur et les codes d'écart, jamais le message. Pas d'option
  `--strict` (code de sortie selon la conformité) pour l'instant : ajoutable plus tard en MINOR.
  Phase 8.2 : les pull requests sont données par **numéro, auteur, branche de tête et codes**, jamais par
  leur titre ; `--milestone vX.Y.Z` affiche la revue de jalon à la place de la vue d'ensemble (un jalon non
  configuré est une erreur d'usage) ; détail dans [05-cli.md](05-cli.md#commandes).
- **JSON** : clés en anglais (`commits.rate`, `commits.issues[{code,count,shas}]`, `semver_checks[]`,
  `changelog.rules[]`…), figées par `internal/cli/json_schema_test.go`. Aucune clé ajoutée à `prs` ou
  `releases` dans un premier temps. La phase 8.2 ne fait qu'ajouter des clés (`traits`, `language`,
  `branches`, `pull_requests`, `tags`, `intervals` par dépôt ; `traits`, `trend`, `pull_requests`,
  `branches` par contributeur ; `milestone` au premier niveau, `null` sans `--milestone`), les mêmes clés
  de premier niveau quelles que soient les options.
  La phase 8.3 ne fait qu'ajouter des clés : `changelog`, `readme`, `docs` et `tooling` par dépôt (`state`,
  `reason`, `sha`, `computed_at`, `conform`, `unknown`, `total`, `criteria[{id,status,reason,codes,counts}]` ;
  `docs` ajoute `lists`, remplies seulement dans la vue détaillée d'un dépôt et vides sinon, pour garder les
  mêmes clés) et `files` par dépôt de la revue de jalon.
- **JSON, phase 8.4** : ne fait qu'ajouter des clés : `rewrites`, `tag_history`, `prereleases`, `bumps`,
  `precedence` et `changelog_freshness` par dépôt, `deadline` par jalon de `tags.milestones`, `deadline` et
  `tag_history` par dépôt de la revue de jalon ; détail dans [05-cli.md](05-cli.md#sortie-json).
- **Phase 8.5** : la page du dépôt ajoute deux pastilles par PR (description PR5, fusion PR6 ; la liste
  montre les PR au titre, à la description ou à la fusion à regarder) et un bloc « Fusions » (commits hors
  PR : SHA court et auteur du commit ; descriptions ; fusions en squash ; branches réutilisées ; méthodes de
  fusion autorisées) ; `/conventions` ajoute une colonne « Hors PR, méthodes de fusion » par dépôt, « Commits
  hors PR » par contributeur et le critère PR3 de la revue ; la page d'un contributeur, ses commits arrivés
  hors PR. Le JSON ne fait qu'ajouter des clés : `arrivals` et `merge_methods` par dépôt, `branches.reused`,
  `pull_requests.methods`, `body` et `merge` par PR, `body_known`, `body_conform`, `merge_known`, `squash`,
  `merge_conform` dans les comptes de PR, `skipped.fork`, `arrivals` par contributeur et par dépôt de la
  revue de jalon.
- **Phase 8.3 : lecture des critères de fichiers.** `Build` et `ReviewMilestone` ne lisent que le cache de
  la synchro (`repository_conventions`), jamais le clone, sauf pour les listes de chemins. La tête de la
  branche par défaut est comparée au dernier commit connu de la branche en base, un tag de jalon à son
  commit en base ; un résultat n'est pris que pour le même commit, la même `FileRulesVersion` et le même
  `config_hash`, sinon « à recalculer » (`stale`) ; sans ligne : « pas encore calculé, lancez watchoo
  sync » (`not_computed`), ou « pas de clone local » (`no_clone`). Aucun score : une grille par famille,
  « k/n critères conformes, u inconnus », attribuée à personne (avec `--contributor`, elles sont « inconnues »
  avec `filtered` ; la page d'un contributeur ne les affiche pas). Les listes de chemins (DOC3 documents non
  référencés et lignes en double, DOC4 liens morts, DOC5 chemins mal nommés, DOC8 archives et documents
  bureautiques) sont recalculées depuis le clone, au commit du résultat en cache, seulement dans la vue
  détaillée d'un dépôt (`watchoo conventions owner/repo`, page du dépôt) ; elles ne sont ni stockées ni
  journalisées. CLI et web affichent les mêmes codes, avec un libellé commun (clés `conv.*` du catalogue).
  La vue d'ensemble de `/conventions` donne une pastille de synthèse par famille ; la page du dépôt donne
  les grilles et, repliées, les listes de chemins.
- **Phase 8.7 : pistes.** La page du dépôt ajoute, après les fichiers, un bloc « Pistes » replié (`details`)
  qui rappelle qu'elles invitent à relire sans juger : une pastille neutre par piste (`badge-ghost`, ou
  `badge-info` pour « à relire », jamais les couleurs de conformité), ses codes, ses compteurs et ses dates,
  puis la liste des images cassées. `/conventions` et la page d'un contributeur n'affichent aucune piste.
  `watchoo conventions owner/repo` donne une section « Pistes » complète ; sans dépôt, seules les pistes à
  relire et un renvoi vers la vue détaillée ; rien avec `--contributor`. Le JSON ne fait qu'ajouter la clé
  `leads` par dépôt (`to_review`, `items[{id,status,reason,codes,counts,dates}]`, `broken_images[]`, vide
  hors vue détaillée) ; détail dans [05-cli.md](05-cli.md#sortie-json) et
  [06-interface-web.md](06-interface-web.md).
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
| `commit.language` | `any` | `fr`, `en` ou `any` ; information seulement (CC7), sans effet sur la conformité |
| `branch.pattern` | vide (règle intégrée `^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$`, construite depuis `commit.types`) | Nommage des branches (BR1) : une expression RE2, validée au chargement, qui remplace la règle intégrée |
| `branch.exclude` | `gh-pages`, `dependabot/*`, `renovate/*` | Branches hors évaluation (BR1) : motifs glob (`*`, `?`, `[...]`, syntaxe de `path.Match`), chacun désignant le nom lui-même ou l'un de ses préfixes de chemin (`dependabot/*` exclut `dependabot/npm_and_yarn/lodash-4.17.21`) ; la branche par défaut est toujours exclue |
| `pr.body_sections`, `pr.require_closes` | `context`, `changes`, `testing`, `screenshots`, `true` | Corps de PR (PR5, phase 8.5) : sections attendues, clés parmi `context`, `changes`, `testing`, `screenshots` (synonymes FR et EN intégrés et versionnés ; liste vide : aucune section attendue) ; mot-clé de fermeture `Closes #n` exigé |
| `pr.min_reviews`, `pr.reviewer_must_contribute` | 1, `false` | Relectures (PR4, phase 8.6) : nombre de relecteurs distincts, autres que l'auteur et non exclus, dont l'approbation est exigée à la fusion (au moins 1) ; un autre compte suffit, sauf si le relecteur doit être contributeur (auteur d'au moins un commit du dépôt) |
| `tags.milestones` | `v1.0.0`, `v2.0.0`, `v3.0.0` (sans échéance) | Jalons (SV2, revue de jalon) : chaque entrée `vX.Y.Z`, ou `vX.Y.Z@AAAA-MM-JJ` pour une échéance |
| `tags.prerelease_forms` | `rc`, `beta`, `alpha` | Formes de pré-version conformes `-<forme>.N` (SV4, phase 8.4) ; les autres formes donnent un avertissement, jamais un écart |
| `changelog.categories_fr` | `true` | Admet les libellés français (catégories, H1, `[Non publié]`) avec un avertissement ; `false` : ce sont des écarts (CL2, CL3, CL5) |
| `readme.sections` | les huit sections de la charte | Sections attendues du README (RD2) : clés parmi `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`, `license` ; leurs synonymes FR et EN sont intégrés et versionnés |
| `readme.min_words` | 150 | Nombre de mots en deçà duquel le README est trop court (RD4), au moins 1 |
| `docs.dir`, `docs.media_dir`, `docs.archives_scope` | `documentation`, `Media`, `repository` | Dossier de la documentation (relatif à la racine), dossier des médias (un seul segment sous `docs.dir`) ; archives interdites dans tout le dépôt (`repository`) ou sous `docs.dir` seulement (`documentation`) |
| `excluded_accounts` | vide | Comptes exclus (formateur, bots) : login, nom ou e-mail de l'auteur, casse ignorée ; propres aux conventions, distincts de `diagnostic.exclude` |

Le bloc se partage par [`configshare`](02-fonctionnalites.md) (décision du mainteneur), sauf `enabled`,
`excluded_accounts` (qui contient des logins) : partager une charte est l'usage visé. La date propre à un
dépôt est exportée avec lui, comme ses seuils d'activité. Les clés sont ensuite figées par SemVer.

**Phase 8.1** : seules `enabled`, `since`, `commit.*` et `excluded_accounts` existent ; les autres clés
arrivent avec leur phase (voir **Phase 8.2** plus bas). Variables d'environnement : `WATCHOO_CONVENTIONS_ENABLED`,
`WATCHOO_CONVENTIONS_SINCE`, `WATCHOO_CONVENTIONS_COMMIT_TYPES`… (listes séparées par des virgules). Règles
vérifiées au chargement : `since` vide ou de la forme `AAAA-MM-JJ` (une date YAML sans guillemets,
`since: 2026-09-01`, est acceptée ; une date avec heure est refusée, sauf un horodatage YAML sans
guillemets à minuit UTC pile, `2026-09-01T00:00:00Z`, que le décodeur YAML ne distingue pas de la date
seule et qui est lu comme `2026-09-01`), lue à minuit dans le fuseau de
l'espace ; au moins un type, chacun en lettres minuscules ; chaque scope de la forme `[a-z0-9][a-z0-9._/-]*` ;
`commit.max_subject_soft` au moins 1 et `commit.max_subject_hard` strictement supérieur ; aucune entrée
vide dans `commit.forbidden` ni `excluded_accounts` ; `commit.language` parmi `fr`, `en`, `any` (sans effet
sur l'analyse en 8.1 ; information de CC7 depuis 8.2). Dans le fichier de partage, le bloc s'écrit `settings.conventions` (`since`,
`commit`) : voir [Partage de la configuration](05-cli.md#partage-de-la-configuration--watchoo-config).

**Phase 8.2** : s'ajoutent `branch.pattern`, `branch.exclude` et `tags.milestones` (1.8.0), partagés comme
`commit.*`. Variables d'environnement : `WATCHOO_CONVENTIONS_BRANCH_PATTERN`,
`WATCHOO_CONVENTIONS_BRANCH_EXCLUDE` et `WATCHOO_CONVENTIONS_TAGS_MILESTONES` (listes séparées par des
virgules pour les deux dernières ; le motif n'est jamais découpé : `{2,40}` y reste entier). Règles vérifiées
au chargement :

- `branch.pattern` vide (règle intégrée) ou expression régulière **RE2** valide (syntaxe Go : pas
  d'assertion arrière ni avant, `(?!…)` est refusé) ; un nom de branche non conforme au motif donné reçoit le
  seul code `pattern`. Le motif est appliqué tel quel, **sans ancrage** : il suffit qu'il trouve une
  correspondance dans le nom (`(feat|fix)/[a-z-]+` accepte `WIP_Feat/feat/x`) ; ajoutez `^…$` pour exiger
  que le nom entier le suive, comme la règle intégrée ;
- `branch.exclude` : chaque motif non vide et valide pour `path.Match`. Un motif désigne le nom lui-même ou
  l'un de ses **préfixes de chemin**, à la manière de `.gitignore` : `path.Match` seul ne franchit pas `/`,
  et `dependabot/*` n'aurait pas reconnu les vraies branches de Dependabot (`dependabot/<écosystème>/<dépendance>`).
  Cette lecture est propre à `conventions.branch.exclude` : `branches.exclude` (branches suivies par la
  synchro) garde la lecture `path.Match` stricte ;
- `tags.milestones` : chaque entrée est un nom de tag conforme (SV1) de **version finale**, `v` minuscule
  suivi de `MAJEUR.MINEUR.CORRECTIF` sans pré-version ni métadonnées de build (`v1.0.0` ; `1.0.0`, `V1.0.0`,
  `v1.0`, `v1.0.0-rc.1` et `v1.0.0+build` sont refusés), suivie au besoin de `@AAAA-MM-JJ`, une date du
  calendrier valide (`v1.0.0@2026-12-18` ; l'échéance se lit jusqu'à la fin du jour, dans le fuseau de
  l'espace) ; une même version ne figure qu'une fois ; une liste vide ne déclare aucun jalon.

`tags.prerelease_forms` n'existait pas en 8.2 : il arrive avec SV4 (phase 8.4, voir plus bas). SV1 admet
toujours toute pré-version SemVer valide, quelle que soit sa forme (`-rc1` comme `-rc.1`) ; c'est SV4 qui
avertit d'une forme libre.

Dans le fichier de partage, ces clés s'écrivent `settings.conventions.branch` (`pattern`, `exclude`) et
`settings.conventions.tags` (`milestones`) ; un fichier qui les contient est refusé par les versions
antérieures à 1.8.0, qui n'acceptent aucune clé inconnue.

**Phase 8.3** : s'ajoutent `changelog.categories_fr`, `readme.sections`, `readme.min_words`, `docs.dir`,
`docs.media_dir` et `docs.archives_scope` (1.9.0), partagés comme `commit.*`. Syntaxe exacte :

```yaml
conventions:
  changelog:
    categories_fr: true          # booléen
  readme:
    sections: ["presentation", "group", "stack", "install", "run", "env", "tree", "license"]
    min_words: 150               # entier, au moins 1
  docs:
    dir: "documentation"         # chemin relatif à la racine du dépôt
    media_dir: "Media"           # un seul nom de dossier, sous docs.dir
    archives_scope: "repository" # "repository" ou "documentation"
```

Variables d'environnement : `WATCHOO_CONVENTIONS_CHANGELOG_CATEGORIES_FR`,
`WATCHOO_CONVENTIONS_README_SECTIONS` (liste séparée par des virgules), `WATCHOO_CONVENTIONS_README_MIN_WORDS`,
`WATCHOO_CONVENTIONS_DOCS_DIR`, `WATCHOO_CONVENTIONS_DOCS_MEDIA_DIR` et
`WATCHOO_CONVENTIONS_DOCS_ARCHIVES_SCOPE`. Règles vérifiées au chargement :

- `readme.sections` : chaque clé parmi les huit ci-dessus (en minuscules, telle quelle), une seule fois ;
  une liste vide n'attend aucune section (RD2 est alors conforme d'office). Les synonymes reconnus pour
  chaque clé (« Installation », « Getting started »…) ne sont pas réglables : ils sont intégrés au binaire
  et versionnés avec les règles ;
- `readme.min_words` : entier au moins égal à 1 ;
- `docs.dir` : chemin relatif à la racine, segments non vides séparés par `/`, sans `/` au début ni à la
  fin, sans segment `.` ni `..`, sans espace au début ni à la fin d'un segment, sans `\` ni caractère de
  contrôle, 255 octets au plus (`documentation`,
  `docs/projet`) ;
- `docs.media_dir` : un seul segment de même forme (`Media`, `images`), cherché sous `docs.dir` ;
- `docs.archives_scope` : `repository` (archives interdites dans tout le dépôt, défaut) ou `documentation`
  (sous `docs.dir` seulement).

Ces réglages entrent dans l'empreinte des résultats en cache (`config_hash`) : en changer un fait recalculer
les critères de fichiers à la synchro suivante. Dans le fichier de partage, ils s'écrivent
`settings.conventions.changelog` (`categories_fr`), `settings.conventions.readme` (`sections`, `min_words`)
et `settings.conventions.docs` (`dir`, `media_dir`, `archives_scope`) ; un fichier qui les contient est
refusé par les versions antérieures à 1.9.0, qui n'acceptent aucune clé inconnue (de même qu'un `config.yaml`
qui les contient).

**Phase 8.4** : s'ajoute `tags.prerelease_forms` (1.10.0), partagé comme `commit.*`. Syntaxe exacte :

```yaml
conventions:
  tags:
    prerelease_forms: ["rc", "beta", "alpha"] # formes conformes « -<forme>.N »
```

Variable d'environnement : `WATCHOO_CONVENTIONS_TAGS_PRERELEASE_FORMS` (liste séparée par des virgules,
`rc,preview`). Règles vérifiées au chargement : **au moins une forme** ; chaque forme est un mot de 1 à
32 lettres ASCII minuscules (`rc`, `preview` ; `RC`, `rc1`, `pre-release` sont refusés) ; une même forme ne
figure qu'une fois. Une pré-version est de forme conforme si elle s'écrit exactement `-<forme>.N`, la forme
dans la liste et `N` un entier sans zéro en tête (`v1.0.0-rc.1`) ; toute autre forme (`-rc1`, `-RC.1`,
`-pre`) reçoit un avertissement de SV4, jamais un écart. Dans le fichier de partage, la clé s'écrit
`settings.conventions.tags.prerelease_forms` ; un fichier qui la contient est refusé par les versions
antérieures à 1.10.0 (de même qu'un `config.yaml` qui la contient).

**Phase 8.5** : s'ajoutent `pr.body_sections` et `pr.require_closes` (1.11.0), partagés comme `commit.*`.
Syntaxe exacte :

```yaml
conventions:
  pr:
    body_sections: ["context", "changes", "testing", "screenshots"] # sections attendues du corps (PR5)
    require_closes: true # mot-clé de fermeture « Closes #n » exigé
```

Variables d'environnement : `WATCHOO_CONVENTIONS_PR_BODY_SECTIONS` (liste séparée par des virgules,
`context,testing`) et `WATCHOO_CONVENTIONS_PR_REQUIRE_CLOSES`. Règles vérifiées au chargement : chaque clé
de `pr.body_sections` parmi les quatre ci-dessus (en minuscules, telle quelle ; `closes` n'en est pas une,
le mot-clé de fermeture a son propre réglage), une seule fois ; une liste vide n'attend aucune section. Les
synonymes reconnus pour chaque clé (« Contexte », « Why », « How to test »…) ne se règlent pas : ils sont
intégrés au binaire et versionnés avec les règles de PR. Ces réglages ne changent que la lecture du masque
`body_sections` déjà calculé à la synchro : les modifier n'oblige pas à relire les PR. Dans le fichier de
partage, ils s'écrivent `settings.conventions.pr` (`body_sections`, `require_closes`) ; un fichier qui les
contient est refusé par les versions antérieures à 1.11.0 (de même qu'un `config.yaml` qui les contient).

**Phase 8.6** : s'ajoutent `pr.min_reviews` et `pr.reviewer_must_contribute` (1.12.0), partagés comme
`commit.*`. Syntaxe exacte :

```yaml
conventions:
  pr:
    min_reviews: 1 # relecteurs distincts approuvant à la fusion (PR4), au moins 1
    reviewer_must_contribute: false # true : le relecteur doit être auteur d'un commit du dépôt
```

Variables d'environnement : `WATCHOO_CONVENTIONS_PR_MIN_REVIEWS` et
`WATCHOO_CONVENTIONS_PR_REVIEWER_MUST_CONTRIBUTE`. Règle vérifiée au chargement : `pr.min_reviews` vaut **au
moins 1** (`0` et les valeurs négatives sont refusés). La synchro ne stocke, par PR fusionnée, que des
compteurs, jamais le login d'un relecteur : les relectures déposées par d'autres comptes, les relecteurs
distincts approuvant à la fusion et, parmi eux, ceux qui sont auteurs d'au moins un commit du dépôt (connus
de la base au moment du calcul, par login ou par identifiant). `pr.min_reviews` et
`pr.reviewer_must_contribute` s'appliquent **à la lecture** depuis ces compteurs : les modifier ne demande
aucune requête ni nouvelle lecture des relectures. `excluded_accounts`, en revanche, s'applique **à la
lecture des relectures** : le modifier ne touche que les PR dont les relectures sont lues ensuite (une PR
déjà lue garde ses compteurs). Dans le fichier de partage, ils s'écrivent `settings.conventions.pr`
(`min_reviews`, `reviewer_must_contribute`) ; un fichier qui les contient est refusé par les versions
antérieures à 1.12.0 (de même qu'un `config.yaml` qui les contient).

## 9. Impact technique

- **Packages** :
  - `internal/conventions` : le parseur et l'agrégation sont des fonctions pures, sur le modèle
    d'`internal/aisignature`, qui renvoient des codes stables et ne lisent ni la base ni la forge ;
    `Build` lit la base et le clone uniquement par les interfaces `Reader` et `CloneReader`, implémentées
    par `internal/store` et `internal/provider/git`, jamais la forge (`ParseCommit`, `CheckBranch`, `CheckPRTitle`,
    `CheckPRBody`, `CheckTag`, `ExpectedBump`, `CheckChangelog`, `CheckReadme`, `CheckIndex`,
    `CheckDocsTree`, `DetectTooling`) ;
  - `internal/semver` : comparateur extrait d'`internal/selfupdate`, sans changement de surface ;
  - la reconnaissance des bots passe d'`internal/diagnostic` à `internal/aisignature` (catalogue de comptes).
- **Synchro** : `internal/sync/conventions.go` (fichiers, phase 8.3) et `internal/sync/history.go`
  (`tag_observations` et BR4, phase 8.4), appelés quand `conventions.enabled`. Le clone est lu par des
  interfaces optionnelles du provider git (`TreePaths`, `ReadBlobs`, `FirstParent`, `IsAncestor`,
  `LastTouch` (CL10, à la lecture), `RevList`), sans ajout à l'interface `Provider`. Côté `Provider`, réalisé en phase 8.5 :
  `PullRequest.MergeCommitSHA` (rempli seulement si la PR est fusionnée), `PullRequest.FromFork`,
  `PullRequest.BodySections` (masque calculé en mémoire), `Repository.AllowSquashMerge`, `AllowMergeCommit`
  et `AllowRebaseMerge` (nil = inconnu ; « inconnu » en clone) ; réalisé en phase 8.6 : `PullRequestReviews`
  (`GET /repos/{o}/{r}/pulls/{n}/reviews`, paginé, au plus 1 000 relectures : login et identifiant du
  relecteur, état, date, jamais le corps ; `ErrUnsupported` en clone). Le calcul n'est relancé que si la tête de la
  branche par défaut, `RulesVersion` ou l'empreinte de la configuration ont changé. Échecs journalisés en
  Debug, sans message ni chemin.
- **Synchro, réalisé en phase 8.3** (critères de fichiers) :
  - le calcul n'a lieu que si `conventions.enabled` et si le clone a été mis à jour pendant cette synchro
    (sinon le résultat précédent reste) ; aucune requête à l'API, aucun fetch supplémentaire ;
  - révisions mesurées : la tête de la branche par défaut (SHA lu dans la liste des branches de la
    source de la synchro) et, pour chaque jalon configuré, le tag qui le désigne (même règle que la revue de
    jalon : nom exact, sinon premier nom non conforme), dont le SHA est lu **dans la base** ; aucun nom de
    tag n'est passé à git. En mode API, si la liste des tags n'a pas été relue (quota bas), ce sont les
    tags déjà en base : approximation assumée, cohérente avec la revue de jalon ;
  - lecture par les objets Git seulement : `git ls-tree -r -l -z --end-of-options <sha>` (au plus 20 000
    fichiers ; au-delà, liste « tronquée » et critères inconnus), puis `git cat-file --batch` sur les
    **identifiants d'objets** (jamais les chemins), sans filtre ni conversion : fichier de plus de 1 Mio
    « trop gros » (inconnu), au plus 32 Mio lus par révision (au-delà, fichiers « illisibles »). Liens
    symboliques et sous-modules ignorés ; un fichier LFS est lu comme son pointeur ; un chemin qui n'est
    pas de l'UTF-8 valide ou qui contient un caractère de contrôle est ignoré ;
  - recalcul seulement si le commit mesuré, `FileRulesVersion` (version des règles de fichiers, distincte
    de `RulesVersion` des messages de commit) ou `config_hash` ont changé ; `config_hash` couvre les
    réglages `conventions.changelog.*`, `conventions.readme.*`, `conventions.docs.*` **et** les noms des
    tags qui désignent une version (CL8 les compare aux sections) : un nouveau tag de version fait donc
    recalculer. Le dépôt (`owner/name`, sans casse) y entre aussi : RD3, DOC3 et DOC4 reconnaissent les URL
    du même dépôt, un renommage ou un transfert fait donc recalculer ;
  - lignes des tags qui ne désignent plus de jalon configuré (jalon retiré, tag supprimé du dépôt)
    supprimées ; la ligne de la tête reste, sauf si la source n'a plus de branche par défaut (dépôt vidé,
    `HEAD` sur une branche absente) : elle est alors supprimée aussi ;
  - échec : journalisé en Debug avec le dépôt, le nombre de révisions et le motif (sans chemin, nom de tag
    ni contenu), la synchro du dépôt réussit ; un échec d'accès à la base (lecture ou écriture du cache, lecture
    des tags) est journalisé de même en Debug ;
  - une date « future » (CL4) est jugée au jour du calcul, dans le fuseau de l'espace.
- **Synchro, réalisé en phase 8.4** (historique) :
  - seulement si `conventions.enabled` ; écrit dans la transaction de la synchro du dépôt, à sa date ;
  - `tag_observations` n'est mis à jour que si la lecture des tags de la synchro a réussi, n'est pas
    tronquée (moins de 1 000 tags : une liste de 1 000 tags exactement est tenue pour tronquée) et n'a ignoré
    aucun tag (nom vide, SHA invalide) ; sinon rien n'est ouvert ni fermé. Même nom avec un autre SHA :
    l'ancienne ligne est fermée (`rewritten`) et une nouvelle ouverte ; nom disparu : fermée (`deleted`) ;
  - `branch_rewrites` : au plus un événement par synchro, pour la branche par défaut suivie avant et après,
    sous le même nom, deux SHA complets différents ; ascendance lue par `git merge-base --is-ancestor
    --end-of-options` dans le clone mis à jour par la synchro (les deux modes) ; en mode API sans clone qui
    tranche, `ErrUnknownBase` sur l'ancienne tête compte comme réécriture ;
  - aucun nom de tag, de branche ni SHA dans les logs (Debug, nom du dépôt seulement) ; la synchro du dépôt
    n'échoue jamais pour ces calculs.
- **Données** (migrations à partir de `00016`, décrites d'abord dans [Modèle de données](04-modele-donnees.md)) :
  - `commits.parent_count`, écrit à la synchro depuis `Commit.Parents` ; l'existant est rattrapé par un
    `UPDATE` depuis `git rev-list --parents` (l'insertion ne modifie jamais un commit déjà en base) ;
  - `repository_conventions(repo_id, rev_sha, rev_kind, tag_name, rules_version, config_hash, results,
    computed_at)` : codes et compteurs, par tête et par tag ;
  - `tag_observations(repo_id, name, sha, first_seen_at, last_seen_at, ended_at, end_reason)` ;
  - `branch_rewrites(repo_id, branch, old_sha, new_sha, observed_at)` ;
  - `repositories.conventions_since` (NULL = date de l'espace) ;
  - réalisé, migration `00020` (phase 8.5) : `pull_requests.merge_commit_sha`, `from_fork`,
    `body_sections` et `repositories.allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge`,
    `conventions_prs_version` (NULL = inconnu, ou relecture complète pas encore faite) ;
  - réalisé, migration `00021` (phase 8.6) : `pull_requests.reviews_by_others`, `approvals_by_others`,
    `approvals_by_contributors` et `reviews_checked_at` (NULL = relectures jamais lues) : des compteurs
    seulement, jamais les logins des relecteurs (décision du mainteneur) ; `pr.min_reviews` et
    `pr.reviewer_must_contribute` sont appliqués à la lecture, depuis ces compteurs, alors que
    `excluded_accounts` et les auteurs de commits du dépôt le sont à la lecture des relectures : changer
    les comptes exclus ne vaut que pour les PR dont les relectures sont lues ensuite ;
  - `branches.protected` n'est pas ajouté tant que BR3 est reporté ;
  - soit 19 tables au lieu de 16.
  - CC, BR1, PR1, PR2, SV1, SV2 et SV6 sont calculés à la volée depuis la base : un changement de charte
    prend effet tout de suite.
- **Quota** :
  - phases 8.1 à 8.4 : aucune requête ;
  - 8.5 : aucune requête en régime courant : `merge_commit_sha` (gardé pour les seules PR fusionnées),
    `from_fork` et le corps (lu **en mémoire** seulement, pour calculer le masque `body_sections`, comme
    D2 : jamais stocké, journalisé ni renvoyé) viennent de `GET /pulls`, les méthodes de fusion de
    `GET /repos`, tous deux déjà demandés à chaque synchro. Ils sont remplis à l'insertion même conventions
    désactivées (ils ne coûtent rien). Avec `conventions.enabled`, un dépôt dont
    `repositories.conventions_prs_version` est absent ou inférieur à la version des règles de PR
    (`conventions.PRRulesVersion`, montée quand les synonymes de sections changent) reçoit **une**
    relecture complète de sa liste de PR (état `all`, au plus 10 pages, sous la réserve de quota existante)
    à la place de la lecture incrémentale qu'elle contient : les PR modifiées depuis la synchro précédente
    sont écrites en entier comme d'habitude, les autres ne reçoivent que `merge_commit_sha`, `from_fork` et
    `body_sections` (leurs `tool_ids` ne changent pas). Le marqueur n'est posé que si la liste a été lue
    sans erreur, sans PR inexploitable et sous le plafond de 1 000 PR ; une relecture interrompue (quota
    épuisé, échec) garde ce qui a été rempli et est retentée à la synchro suivante. Approximation : un
    dépôt de 1 000 PR ou plus ne reçoit jamais le marqueur et est relu (10 pages) à chaque synchro tant que
    les conventions sont actives. Mode clone : aucune PR, tout est « inconnu ». `from_fork` est inconnu
    quand la réponse ne donne pas le dépôt de base ; les méthodes de fusion ne sont renvoyées qu'à un compte
    qui a le droit push (sinon « inconnu », écrit NULL même sur une valeur connue auparavant) ;
  - 8.6, seulement avec `conventions.enabled` et en mode API, après l'écriture de la synchro : une requête
    (pagination comprise, 100 relectures par page) par PR fusionnée dont les relectures n'ont jamais été
    lues depuis sa fusion (`reviews_checked_at` NULL ou antérieur à `merged_at` : une PR lue ouverte est
    relue une fois après sa fusion), une seule fois, plus les PR ouvertes (hors brouillons) dont
    `updated_at` a changé depuis la dernière lecture (pour l'affichage ; PR4 ne juge que les fusionnées) ;
    jamais une PR issue d'un fork, d'un bot, ni ouverte avant la date de début des conventions du dépôt
    (`created_at`). Une PR ouverte est marquée lue à sa date de modification sur la forge (`updated_at`,
    jamais l'horloge locale) : toute fusion ultérieure la fait relire une fois. Fusionnées d'abord, la fusion la plus récente d'abord, puis
    les ouvertes ; au plus 100 requêtes par dépôt et par synchro (`sync.MaxReviewsPerRepo`), et la passe
    s'arrête dès que le quota restant passe sous la réserve (10 %) : le reste est lu aux synchros
    suivantes. Un quota épuisé reporte les dépôts suivants ; tout autre échec est journalisé en Debug
    (dépôt, numéro de PR, erreur ; jamais de login ni de contenu) et ne fait jamais échouer la synchro du
    dépôt. Estimation : 30 dépôts × 40 PR fusionnées = 1 200 requêtes au premier passage (24 % du quota
    horaire de 5 000), toutes sous le plafond de 100 par dépôt ; ensuite, en régime courant, une requête
    par PR nouvellement fusionnée et par PR ouverte modifiée depuis la dernière lecture (par exemple
    30 dépôts × 3 PR par semaine = 90 requêtes par semaine). Le cache ETag du transport est utilisé (un 304
    ne coûte pas de quota) mais il garde 1 024 entrées : un premier passage sur 1 200 PR chasse les
    premières, et il est vide à chaque processus CLI (partagé dans `watchoo serve`) ;
  - les releases sont déjà relues en entier à chaque synchro : coût inchangé.
- **Interaction avec le diagnostic** : la fusion en squash imposée par la charte produit de gros commits
  uniques sur la branche par défaut, qui alimentent A1 (commit massif) et A2 (dépôt rendu en une fois) et
  effacent la matière de H4. Quand `conventions.enabled` est actif, les commits de squash identifiés
  (« (#n) » avec le committer GitHub, phase 8.1 ; ou, phase 8.5, `merge_commit_sha` d'une PR fusionnée, à un
  parent, avec le suffixe « (#n) » de cette PR) sont exclus de A1 et A2, et le
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
| 8.1 | `internal/conventions` (parseur), `internal/semver`, réglages `conventions.*`, CC1 à CC4, `commits.parent_count` et son rattrapage, `watchoo conventions --commits`, section web et page contributeur, ajustements de A1, A2 et F4. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; code `scope` ajouté aux codes bloquants de CC2 ; CC1 par intervalle entre tags et tendance de CC3 depuis le dernier tag reportés à 8.2, avec la lecture des tags) | 0 quota, 1 migration | 1.7.0 |
| 8.2 | À la volée : BR1, PR1, PR2, SV1, SV2 (présence), SV6 ; première revue de jalon ; CC5 à CC7 ; CC1 par intervalle entre tags, tendance de CC3. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; réglages `branch.pattern`, `branch.exclude` et `tags.milestones` ; route `/conventions` ; approximations listées en [5.4](#54-tags-et-semver-sv) : forks inclus jusqu'à 8.5, comparaison de PR2 avec le seul commit non mesurée, mode API déduit ; pastille compacte du tableau de bord reportée) | 0 quota | 1.8.0 |
| 8.3 | Arbre du clone : `repository_conventions`, CL1 à CL9, RD1 à RD4, DOC1 à DOC8, TL1 à TL3 ; revue de jalon sur l'arbre des tags. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; migration `00017` ; réglages `changelog.categories_fr`, `readme.sections`, `readme.min_words`, `docs.dir`, `docs.media_dir`, `docs.archives_scope` ; écarts : DOC6 et DOC7 reportés à 8.7, CL10 à 8.4 ; `FileRulesVersion` distincte de `RulesVersion` ; `config_hash` couvre aussi les noms des tags de version (CL8) ; ordre des catégories (CL6) en avertissement ; écart de date entre tag et section (CL8) non mesuré ; TL1 sans date d'introduction ; DOC4 lit au plus 300 documents ; listes de chemins seulement sur la page d'un dépôt et dans `watchoo conventions owner/repo`, pas dans `/conventions` ; en mode API, tags des jalons lus dans la base, voir [section 9](#9-impact-technique)) | 0 quota, 1 migration | 1.9.0 |
| 8.4 | Historique : `tag_observations` (SV3, échéances de SV2), `branch_rewrites` (BR4), SV4, SV5, SV7, CL10. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; migrations `00018` et `00019` ; réglage `tags.prerelease_forms` ; SV3 dans la revue de jalon ; approximations listées en [5.4](#54-tags-et-semver-sv) : preuve d'une lecture des tags par les dates de `tag_observations`, « en retard » seulement sur une lecture faite après l'échéance, réécriture d'un tag annoté sur le même commit invisible, BR4 en mode API seul partiel ; SV4, SV5, SV7 et CL10 calculés à la lecture, non stockés) | 0 quota, 2 migrations | 1.10.0 |
| 8.5 | API sans requête de plus : `merge_commit_sha`, `from_fork`, `body_sections`, méthodes de fusion ; PR3 complet, PR5, PR6, BR2, BR5. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; migration `00020` ; réglages `pr.body_sections` et `pr.require_closes` ; PR3 dans la revue de jalon ; forks écartés de PR1, PR2, PR5, PR6, BR1 et BR2 ; approximations listées en [5.3](#53-pull-requests-pr) : PR3 filtré par la date du committer, `unmatched` tant que la liste des PR n'a pas été relue, PR6 « rebase » pour un squash au sujet réécrit, BR5 inconnu sans droit push, dépôt de 1 000 PR ou plus relu à chaque synchro ; comparaison de PR2 avec le seul commit non mesurée) | une relecture des listes de PR | 1.11.0 |
| 8.6 | Relectures : PR4, « fusionnées sans relecture », relectures reçues. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; migration `00021` ; réglages `pr.min_reviews` et `pr.reviewer_must_contribute`, appliqués à la lecture ; PR4 dans la revue de jalon ; aucun login de relecteur stocké : des compteurs seulement, `excluded_accounts` appliqué aux PR lues ensuite ; approximations listées en [5.3](#53-pull-requests-pr) et [7.1](#71-revue-de-jalon-rj) : PR fusionnée lue avant sa fusion « inconnue » jusqu'à la lecture suivante, relecteur sans compte jamais compté, `DISMISSED` daté du dépôt de la relecture et non de son retrait, relecteur contributeur reconnu par login ou par identifiant de compte, intervalle du jalon borné par les dates des commits visés par les tags) | 1 requête par PR fusionnée | 1.12.0 |
| 8.7 | Pistes facultatives : CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4, TL5. **Réalisée, non publiée** (`[Unreleased]` du CHANGELOG ; aucune migration, aucun réglage ; `FileRulesVersion` passe à 2 ; pistes hors grilles, hors revue de jalon et hors taux ; CL11, CL12, RD5, DOC6, DOC7 et TL4 en cache à la synchro, DOC9, DOC10 et TL5 à la lecture ; approximations listées en [5.8](#58-outillage-tl) : DOC9, DOC10 et TL5 seulement en vue détaillée, introduction de TL5 datée par fichier seulement, CL12 sous-estimé si la branche par défaut n'est pas suivie, grilles « à recalculer » jusqu'à la synchro suivante, aucun compteur de pistes dans `/conventions`) | 0 quota | 1.13.0 |

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
  date de CL8, seuils de CL12 (10 puces, 60 %, 20 %), de DOC9 (20 commits, 30 jours), de DOC10 (30 jours)
  et de TL5 (10 commits de chaque côté).
- **Phase 8.6** : une relecture met-elle à jour `updated_at` de la PR ? Toujours pas vérifié sur GitHub, et
  plus nécessaire pour les PR fusionnées : une lecture après la fusion suffit (les relectures déposées après
  `merged_at` ne comptent pas), et une PR lue quand elle était ouverte est relue une fois fusionnée. Seul
  l'affichage des PR ouvertes en dépend, relues quand leur `updated_at` change.
- **Si BR3 est repris** : `protected` reflète-t-il les rulesets ? Sur quel plan GitHub sont les dépôts
  étudiants (la protection d'un dépôt privé n'existe pas sur GitHub Free) ?

## 12. Références

- Conventional Commits 1.0.0 : <https://www.conventionalcommits.org/fr/v1.0.0/>
- Semantic Versioning 2.0.0 : <https://semver.org/lang/fr/>
- Keep a Changelog 1.1.0 : <https://keepachangelog.com/fr/1.1.0/>
- commitlint et `@commitlint/config-conventional` : <https://commitlint.js.org/>
- API GitHub, revues de pull request : <https://docs.github.com/en/rest/pulls/reviews>
- API GitHub, branches (`protected`) : <https://docs.github.com/en/rest/branches/branches>
