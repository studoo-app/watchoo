# 2. Fonctionnalités

[← Sommaire](README.md)

Le MVP se limite à **suivre, synchroniser et afficher** ; tout le reste vient ensuite.

## MVP

| Fonctionnalité | Détail |
| --- | --- |
| Ajouter / retirer un dépôt suivi | Saisie `owner/repo` ou URL GitHub ; toutes les branches suivies par défaut, restreignables par liste ou motif. Ajout par `watchoo repo add`, l'import, la page Réglages du web ou la touche `a` du TUI (vue Dépôts : toutes les branches, synchro lancée tout de suite) |
| Import groupé de dépôts | Fichier CSV (dépôt, nom affiché, branches) : toute une promotion d'un coup, accès vérifié, bilan, relançable sans doublon |
| Synchronisation des commits | Récupération **incrémentale** depuis le dernier SHA connu |
| Liste des derniers commits | Message, auteur, date, SHA court, lien GitHub ; filtres par dépôt, auteur, période |
| Vue contributeurs | Nombre de commits par personne et par dépôt, date de dernière activité |
| Statut d'un dépôt | Dernier commit, état (Actif / Flemme / Dead), branche par défaut, étoiles, issues ouvertes |
| Synchro automatique | Tâche périodique configurable : 30 min par défaut, 15 min minimum (dans `serve` et `tui`), désactivable |
| Synchro manuelle | À tout moment, tous les dépôts ou un seul : `watchoo sync`, bouton dans le web, touche dans le TUI |
| Graphiques d'activité (web) | Commits par jour sur 90 jours pour l'espace et pour chaque dépôt ; commits par semaine sur 12 semaines des 8 contributeurs les plus actifs |

## V2

| Fonctionnalité | Détail |
| --- | --- |
| Activité par contributeur dans un dépôt | Graphique des commits de chaque contributeur à l'intérieur d'un même dépôt. **Publié en v0.4.0** : sur la page d'un dépôt (web), commits par semaine sur 12 semaines des 8 contributeurs les plus actifs du dépôt, comme la page des contributeurs ; total et courbe par semaine des principaux contributeurs dans le détail d'un dépôt du TUI ; `watchoo contributors [--repo X] --weeks N` (commits semaine par semaine, clé `weekly` en JSON). Toutes les branches suivies comptent, chaque commit une fois |
| Seuils d'activité par dépôt | Régler les seuils Flemme / Dead d'un dépôt (option de `repo add`, colonne de l'import, réglage web). **Publié en v0.4.0** : `repo add --flemme N --dead N`, colonnes `flemme` et `dead` de l'import, partage de la configuration, formulaire de la page d'un dépôt |
| Pull requests et releases | PR ouvertes / mergées, tags, releases. **Publié en v0.4.0** : lues à chaque synchro (PR de façon incrémentale, depuis la dernière modification connue ; releases et tags en entier, gratuits quand rien n'a changé), `watchoo prs [--repo X] [--state open\|merged\|closed\|all]` et `watchoo releases [--repo X]` (texte et `--json`), cartes « Pull requests » et « Releases et tags » sur la page d'un dépôt (web), PR ouvertes sur le tableau de bord, PR ouvertes et dernière release dans le détail du TUI. Un échec sur ces listes n'empêche pas la synchro des commits : il devient un avertissement |
| Dépôts Git locaux | Suivre un dépôt depuis un clone local (`git clone`, puis `git fetch` périodique) : commits, branches et tags sans appel à l'API GitHub. Utilisé quand `gh` n'est pas installé ou connecté. **Publié en v0.4.0** : choix refait à chaque synchro selon `gh` seulement ; clones avec copie de travail dans `repos/<name>` à la racine de l'espace (homonymes : `repos/<owner>-<name>` ; modifications locales des fichiers suivis écrasées à chaque synchro) (SSH par défaut, `git.protocol: https` possible), avec les identifiants Git de la machine, sans invite ; `watchoo init` sans `gh` prévient au lieu de s'arrêter ; source affichée par `watchoo status`, le bilan de `watchoo sync` et la page Réglages ; étoiles, issues, pull requests et releases masquées pour un dépôt jamais lu par l'API ; `repo remove` supprime le clone (`--keep-history` le garde) ; image Docker avec `git` |

## 0.5.0

Version suivante de la V2, décidée par le mainteneur : elle complète la vue contributeurs.

| Fonctionnalité | Détail |
| --- | --- |
| Page par contributeur | Une page par contributeur : ses dépôts, ses commits et son activité dans le temps, sur le web et dans le TUI. **Publié en v0.5.0** : page web `/contributors/{id}` (en-tête avec nom, login, avatar et nombre d'identités Git connues, sans aucune adresse ; totaux de commits, lignes `+/-`, dépôts, premier et dernier commit ; commits par jour sur 90 jours ; commits par semaine et par dépôt sur 12 semaines ; ses dépôts avec commits, lignes, dernier commit et état du dépôt ; ses commits paginés ; ses pull requests quand son compte de forge est connu), liens vers cette page depuis toutes les listes où un contributeur apparaît ; détail d'un contributeur dans le TUI (`entrée` dans la vue Contributeurs). Pas d'état Actif / Flemme / Dead pour une personne : ces états restent des règles de dépôt ; la page donne l'ancienneté du dernier commit et l'état de chacun de ses dépôts |
| Lien vers le profil de forge | Partout où l'interface web montre un contributeur dont le compte de forge est connu, un lien secondaire « GitHub » ouvre son profil (adresse fournie par la forge, enregistrée dans `contributors.profile_url` ; pour un contributeur connu avant la 0.5.0, complétée à partir de son login à la synchro suivante). Aucun lien pour un contributeur connu seulement par son nom et son e-mail Git (clones Git locaux) |

## 0.6.0

Version suivante, décidée par le mainteneur : Watchoo signale lui-même ses nouvelles versions et sait se
mettre à jour.

| Fonctionnalité | Détail |
| --- | --- |
| Avis de nouvelle version | À **chaque lancement** d'une commande, Watchoo vérifie, sans bloquer, si une version plus récente que la sienne est publiée, et l'annonce sur la sortie d'erreur à la fin de la commande (« Nouvelle version de Watchoo disponible : v0.5.0 → v0.6.0. Mettez-la à jour avec « watchoo update ». »). Aucun avis avec `--json`, hors terminal (scripts, CI), pour une version de développement, ni si la variable `WATCHOO_NO_UPDATE_CHECK` est posée (image Docker). `watchoo serve` vérifie au démarrage puis toutes les 30 min, et l'interface web montre alors un bandeau discret. **Publié en v0.6.0** |
| Mise à jour en une commande | `watchoo update` affiche la version installée et la dernière, demande confirmation, télécharge l'archive de la release pour le système, vérifie sa somme SHA-256 et remplace le binaire d'un bloc ; `watchoo update --check` ne fait que vérifier. Refus expliqué dans l'image Docker (changer le tag), pour une version compilée depuis les sources (`go install …@latest` ou `git pull && make build`) et si le répertoire du binaire n'est pas accessible en écriture. Seules les pages publiques des releases de Watchoo sont lues : ni token, ni API GitHub, ni quota. **Publié en v0.6.0** |

## 0.7.0

| Fonctionnalité | Détail |
| --- | --- |
| Interface bilingue | CLI, TUI, interface web et script d'installation en **anglais par défaut**, disponibles aussi en **français** (`WATCHOO_LANG`, la locale du système, ou le réglage `lang` de l'espace pour le web). Les clés `--json` et les valeurs d'état ne changent pas. **Publié en v0.7.0** (voir [Langues et traductions](03-architecture.md#langues-et-traductions)) |

## 1.0.0

Aucune fonctionnalité nouvelle : la surface publique (commandes, options, codes de sortie, clés
`--json`, réglages et variables `WATCHOO_*`, schéma de la base, routes web) est relue et déclarée
**stable** ; désormais, un changement incompatible demande une version majeure. **Publié en v1.0.0**.

## 1.1.0

| Fonctionnalité | Détail |
| --- | --- |
| Langages d'un dépôt | Langages de la branche par défaut avec leur pourcentage (part des octets), calculés à chaque synchro depuis le clone Git local, avec ou sans `gh` et sans quota : `watchoo languages`, carte « Langages » de la page d'un dépôt, détail d'un dépôt dans le TUI. Les chiffres sont proches de ceux de GitHub, sans être identiques (voir [Architecture](03-architecture.md#langages-dun-dépôt-110)). **Publié en v1.1.0** |
| Part des contributeurs | Part des commits de chaque contributeur en pourcentage, dans `watchoo contributors` (clé `share` en JSON), sur la page d'un dépôt et dans le détail d'un dépôt du TUI. **Publié en v1.1.0** |

## 1.2.0

| Fonctionnalité | Détail |
| --- | --- |
| Langages d'un contributeur | Lignes ajoutées par langage dans les commits dont il est l'auteur, avec leur pourcentage, lues dans le clone Git local : carte « Langages » de la page d'un contributeur (et langage principal de chacun de ses dépôts), `watchoo contributors` (colonne LANGAGE, clé `languages` en JSON), détail d'un contributeur du TUI. Voir [Architecture](03-architecture.md#langages-dun-contributeur-120). **Publié en v1.2.0** |
| Signatures d'IA | Outils déclarés dans les commits (co-auteur, pied de message, compte d'agent), par commit et par contributeur, dans `log`, `contributors`, le TUI et l'interface web. Voir [Signatures d'IA](#signatures-dia). **Publié en v1.2.0** |

## 1.8.0

| Fonctionnalité | Détail |
| --- | --- |
| Noms des contributeurs | Les commits ne donnent qu'un pseudo (login, nom Git) : le superviseur donne à un contributeur le nom de son choix dans les réglages web (`/settings/contributors`, ou le lien « Renommer » de sa page), affiché partout à la place du pseudo (web, CLI, TUI, `--json`), le login restant visible. Gardé dans la base de l'espace seulement : jamais écrit par la synchro, jamais exporté. Voir [Modèle de données](04-modele-donnees.md#contributors--personnes-identifiées) |
| Pages de réglages | Les réglages web sont répartis sur quatre pages avec un menu : dépôts, contributeurs, synchronisation (intervalle, synchro, quota), partage (export, import). Voir [Interface web](06-interface-web.md#5-réglages) |

## V3

| Fonctionnalité | Détail |
| --- | --- |
| Suivi d'une organisation entière | Importer tous les dépôts d'un compte ou d'une organisation |
| Alertes | Webhook Discord / Slack / e-mail, ou notification desktop |
| Webhooks GitHub entrants | Mise à jour en temps réel au lieu du polling |
| Autres forges | GitLab, Gitea |
| Export | CSV / JSON pour rapports |

## Règles fonctionnelles

- **États d'activité** : calculés à partir de l'âge du dernier commit, en jours entiers.

  | État | Âge du dernier commit (défaut) | Web / TUI | CLI (`repo list`) |
  | --- | --- | --- | --- |
  | **Actif** | 0 à 5 jours | vert | `-` |
  | **Flemme** | 6 à 15 jours | orange | `~ 9 j` |
  | **Dead** | plus de 15 jours | rouge | `⚠ 42 j` |

  - « Dernier commit » = le plus récent **sur toutes les branches suivies** du dépôt, pas seulement la
    branche par défaut.
  - Les deux seuils (5 et 15 jours) se règlent **globalement** dans la config (utilisateur ou espace),
    et peuvent être remplacés **pour un dépôt** (une bibliothèque stable qui bouge rarement, un projet
    rendu en fin de semestre…) : `watchoo repo add owner/repo --flemme N --dead N`, colonnes `flemme` et
    `dead` du fichier d'import, partage de la configuration, ou formulaire « Seuils d'activité » de la
    page d'un dépôt dans l'interface web. Chaque seuil est indépendant : un dépôt peut ne fixer que son
    seuil Dead et garder le seuil Flemme de l'espace. `0` (ou un champ vide dans le web) revient au
    seuil de l'espace.
  - Un seuil propre vaut de 1 à 3 650 jours ; quand les deux sont donnés ensemble, Dead doit être
    strictement supérieur à Flemme (sinon refus, avant tout appel à GitHub). Si le mélange d'un seuil
    propre et d'un seuil de l'espace donne Dead inférieur ou égal à Flemme, Dead est ramené à Flemme + 1
    jour pour le calcul de l'état.
- **États hors activité** : affichés en **gris**, ils sont prioritaires sur les états d'activité.
  - **Jamais synchronisé** : le dépôt vient d'être ajouté.
  - **En erreur** : la dernière synchro a échoué (dépôt supprimé, accès perdu…).
  - **Archivé** : le dépôt est archivé sur GitHub, il ne bougera plus ; il n'est pas signalé en Dead.
  - **Arrêté** : le suivi est arrêté (`repo remove --keep-history`) ; l'historique reste consultable.
  - **Vide** : le dépôt a été synchronisé mais n'a aucun commit (dépôt d'étudiant tout juste créé).
  - Priorité entre états gris : Arrêté, En erreur, Jamais synchronisé, Archivé, Vide.
- **Jours** : comptés en jours calendaires dans le fuseau d'affichage (réglage `timezone`, sinon celui du
  système) : un commit d'hier à 23 h a 1 jour, quelle que soit l'heure.
- **Incrémental** : un commit déjà présent en base n'est jamais re-téléchargé.
- **Identité** : un contributeur peut utiliser plusieurs e-mails ; ils sont rattachés à son login GitHub
  quand celui-ci est connu. Depuis la 1.8.0, le superviseur peut lui donner un nom, affiché à la place du
  pseudo des commits.
- **Lecture seule** : Watchoo n'écrit jamais sur GitHub.

## Signatures d'IA

Depuis la 1.2.0, Watchoo montre quels outils d'IA sont **déclarés** dans les commits. Il répond à « un
outil est-il signé sur ce commit ? », jamais à « ce code a-t-il été écrit par une IA ? ».

- **Ce que dit une signature** : un outil est déclaré sur le commit. Il a pu n'écrire que le message, la
  signature s'ajoute ou se retire à la main, et un squash peut la rattacher à la personne qui a fusionné.
  **Son absence ne prouve rien** : la plupart des outils permettent de la désactiver. L'interface ne dit
  donc jamais « humain », ne donne ni score ni pourcentage, et un commit sans signature n'affiche rien.
- **Trois types de signature** (`kind`) : `coauthor_trailer` (ligne `Co-authored-by` du message dont
  l'adresse appartient à un outil), `footer` (ligne entière écrite par un outil), `agent_account` (l'auteur
  du commit est le compte ou l'adresse d'un agent).
- **Éviter les faux positifs** : adresses et comptes sont comparés exactement, **jamais les noms Git**
  (une personne peut s'appeler Claude) ; le sujet et les lignes en retrait ne sont pas lus (citations,
  exemples) ; un texte libre qui mentionne un outil ne compte pas ; l'automatisation sans IA (Dependabot,
  Renovate) n'est pas au catalogue.
- **Catalogue** : uniquement des signatures observées sur de vrais commits publics. Un outil sans
  signature vérifiée est laissé de côté plutôt que deviné.

| Outil (`tool`) | Signatures reconnues |
| --- | --- |
| `aider` | Co-auteur `aider@aider.chat` ou `noreply@aider.chat` |
| `claude` | Co-auteur `noreply@anthropic.com` ; pied de message `Generated with [Claude Code]` |
| `codex` | Co-auteur `noreply@openai.com` ou `codex@openai.com` |
| `copilot` | Co-auteur `copilot@github.com` ; co-auteur ou auteur : le compte `Copilot` ou `copilot-swe-agent[bot]` |
| `cursor` | Co-auteur ou auteur `cursoragent@cursor.com` |

- **Non lus** : le contenu du code, le style des messages, le rythme des commits. Ces indices ont des
  explications innocentes (push groupé, rebase, étudiant qui vient d'apprendre Conventional Commits) et
  aucune fiabilité mesurée ; cette fonction ne les lit pas. Le [diagnostic](12-supervision-diagnostic.md)
  (phase 7), désactivé par défaut et réservé au formateur, en fait des pistes de réflexion, jamais des
  verdicts, chaque apprenant comparé à son propre historique.
- **Objectif** : être en mesure de savoir quels outils sont déclarés, par qui et où, pas sanctionner. Pour
  en savoir plus, demander aux contributeurs de laisser activée l'attribution de leurs outils : Watchoo
  ne voit que ce que les commits déclarent.

## Conventions de projet

Phases 8.1 à 8.7 (non publiées) : Watchoo mesure la conformité des messages de commit à une charte
Conventional Commits, puis celle des noms de branche, des titres de pull request et des tags, puis celle du
CHANGELOG, du README, de la documentation et de l'outillage, puis l'historique des tags, les fusions et les
relectures, et enfin des pistes à relire ; le détail est
dans [Conventions de projet](13-conventions.md).

- **Désactivé par défaut** (`conventions.enabled`) et **réservé au formateur** : ce sont des constats
  objectifs, séparés du [diagnostic](12-supervision-diagnostic.md), **sans note ni score global**.
- **CC1** : taux de commits conformes (commits sans écart bloquant / commits évalués), expliqué en `n/m`
  par dépôt ; **CC2** : écarts par motif, bloquants ou avertissements ; **CC3** : taux par contributeur ;
  **CC4** : exclusions comptées à part (fusions, bots, commit racine « Initial commit » créé par GitHub,
  comptes de `conventions.excluded_accounts`).
- **Périmètre** : toutes les branches suivies, depuis la date `conventions.since` (surchargeable par dépôt).
- **Phase 8.2**, calculée à la lecture (sans migration ni requête de plus) :
  - **BR1** : noms des branches du distant et des branches de tête des PR (`<type>/<mots-avec-tirets>` ou
    `conventions.branch.pattern`, hors `conventions.branch.exclude` et branche par défaut) ;
  - **PR1** et **PR2** : titres des PR ouvertes et fusionnées (même parseur, en-tête seulement), titres par
    défaut ; mode API seulement, « inconnu » en clone seul ;
  - **SV1** : tags `vX.Y.Z` conformes à SemVer 2.0.0 ; **SV2** : présence des jalons de
    `conventions.tags.milestones` ; **SV6** : releases, en information seulement ;
  - **CC5** (avertissements de formulation), **CC6** (footers, issues citées) et **CC7** (langue des sujets),
    en information ;
  - **CC1 par intervalle** entre tags finals et **tendance de CC3** (avant et depuis le dernier tag final) ;
  - **revue de jalon** : critères (CC1 sur l'intervalle, BR1, SV1, SV2) conformes, non conformes ou inconnus
    (`conform`, `nonconform`, `unknown` en `--json`), « k/n critères conformes, u inconnus », sans note ;
    exige une date de début.
- **Phase 8.3**, mesurée par la synchro sur l'arbre du clone local, par ses objets Git (sans requête à l'API
  ni fetch de plus), et mise en cache (table `repository_conventions`, migration `00017`) :
  - **fichiers** : CHANGELOG (CL1 à CL9), README (RD1 à RD4), documentation (DOC1 à DOC5, DOC8) et
    outillage (TL1 à TL3), pour la tête de la branche par défaut et le tag de chaque jalon ; une grille par
    famille (« k/n critères conformes, u inconnus »), listes de chemins dans le détail d'un dépôt seulement,
    recalculées depuis le clone et jamais stockées ;
  - **revue de jalon** : s'ajoutent CL9, CL8, RD1, RD2 (section installation), RD3, DOC1 et DOC3, sur
    l'arbre du tag du jalon.
- **Phase 8.4**, historique observé par la synchro quand les conventions sont activées (tables
  `tag_observations` et `branch_rewrites`, migrations `00018` et `00019`), versions lues dans le clone :
  - **BR4** : réécritures observées de la branche par défaut (événements datés, ancien et nouveau SHA),
    jamais attribuées ;
  - **SV3** : tags réécrits ou supprimés, « aucune réécriture observée depuis le … », jamais « jamais
    réécrit » ; **SV2** : échéance d'un jalon comparée à la première observation de son tag (à l'heure, en
    retard, indéterminé) ;
  - **SV4** (formes de pré-version de `conventions.tags.prerelease_forms`, avertissements), **SV5** (saut
    de version cohérent avec les commits déclarés, sous-versionnement seulement), **SV7** (précédence face à
    l'historique) et **CL10** (`feat` et `fix` depuis la dernière modification du CHANGELOG, information) ;
  - **revue de jalon** : SV3 s'ajoute après SV2, qui juge aussi l'échéance.
- **Phase 8.5**, fusions lues sans requête de plus (migration `00020` : SHA de fusion, origine fork et
  sections de la description d'une PR, la description elle-même n'étant lue qu'en mémoire et jamais
  stockée ; méthodes de fusion autorisées, connues seulement avec le droit push) :
  - **PR3** : commits arrivés sur la branche par défaut hors PR (premier parent de la branche par défaut du
    clone local), rattachés à l'auteur du commit, jamais à qui l'a poussé ; par PR, hors PR ou
    indéterminés ;
  - **PR5** (sections attendues de la description, `conventions.pr.body_sections`, et mot-clé de fermeture
    `Closes #n`, `conventions.pr.require_closes`), **PR6** (fusion squash au message conforme), **BR2**
    (branche de tête fusionnée plus d'une fois) et **BR5** (squash seule méthode de fusion autorisée,
    information) ; les PR issues d'un fork sont écartées de PR1, PR2, PR5, PR6, BR1 et BR2 ;
  - **revue de jalon** : le critère **PR3 = 0** s'ajoute après BR1, sur l'intervalle du jalon.
- **Phase 8.6**, relectures (migration `00021` : trois compteurs et la date de lecture par pull request, jamais le login d'un
  relecteur ; une requête par pull request fusionnée, mode API seulement, au plus 100 par dépôt et par
  synchro) :
  - **PR4** : pull request fusionnée relue par un autre compte (au moins `conventions.pr.min_reviews`
    relecteurs distincts, ni l'auteur, ni un bot, ni un compte exclu, dont la dernière relecture décisive
    avant la fusion est une approbation ; contributeurs seulement avec `conventions.pr.reviewer_must_contribute`),
    pull requests fusionnées sans relecture (numéro et auteur) et, par contributeur, relectures reçues sur
    ses pull requests (les relectures données ne sont pas comptées) ;
  - **revue de jalon** : le critère **PR4** s'ajoute après PR3, sur les pull requests fusionnées dans
    l'intervalle du jalon.
- **Phase 8.7**, pistes (sans migration, sans réglage, sans requête API) : elles invitent à relire et ne
  jugent jamais, ne comptent dans aucune grille, aucun taux ni dans la revue de jalon ; affichées dans un
  bloc « Pistes » séparé :
  - sur l'arbre de la tête, en cache de la synchro : **CL11** (liens de comparaison), **CL12** (recopie
    probable de `git log`, pourcentages seulement), **RD5** (bloc de commandes de l'installation,
    `.env.example`), **DOC6** (en-têtes de document, compteurs par champ, jamais les valeurs), **DOC7**
    (images, blocs `mermaid`, dossier des médias), **TL4** (CI qui vérifie les conventions) ;
  - sur l'historique du clone, dans le détail d'un dépôt seulement : **DOC9** (fraîcheur de la
    documentation par rapport au code), **DOC10** (dates déclarées dans l'INDEX) et **TL5** (CC1 avant et
    après l'outillage, « n/a » sous 10 commits d'un côté).
- **Restitution** : commande `watchoo conventions` (`--milestone vX.Y.Z` pour la revue de jalon), page
  `/conventions`, section de la page d'un dépôt et page d'un contributeur ; jamais le message de commit ni le
  titre d'une PR, seulement le SHA court, le numéro de PR, l'auteur, les noms de branche et de tag et les
  codes. Les indicateurs de dépôt (branches sans PR, tags, jalons, fichiers) ne sont attribués à personne.
- La charte (`since`, `commit.*`, `branch.*`, `tags.milestones`, depuis la phase 8.3 `changelog.*`,
  `readme.*`, `docs.*`, depuis la phase 8.4 `tags.prerelease_forms`, depuis la phase 8.5 `pr.body_sections`
  et `pr.require_closes`, et depuis la phase 8.6 `pr.min_reviews` et `pr.reviewer_must_contribute`) se partage par `watchoo config export` /
  `import`.
