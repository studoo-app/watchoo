# 5. CLI

[← Sommaire](README.md)

## Options globales

| Option | Effet |
| --- | --- |
| `--json` | Sortie JSON pour rendre toutes les commandes scriptables |
| `-C <répertoire>` / `--workspace <répertoire>` | Travaille sur l'espace de ce répertoire au lieu de celui trouvé à partir du répertoire courant |
| `--verbose` | Logs détaillés (niveau debug) sur la sortie d'erreur. Sans elle, le niveau vient de la variable `WATCHOO_LOG_LEVEL` (`debug`, `info`, `warn` par défaut, `error`). Pas de lettre courte : `-v` serait confondu avec `--version` |

Toutes les commandes sauf `init`, `version` et `update` travaillent sur un **espace de supervision** : le répertoire
`.watchoo/` trouvé dans le répertoire courant ou ses parents (voir
[Espaces de supervision](10-espaces-de-supervision.md)).

**Langue des messages** : la CLI et le TUI parlent **anglais par défaut**. La variable `WATCHOO_LANG`
(`en` ou `fr`) choisit la langue ; sans elle, la locale du système décide (`LC_ALL`, `LC_MESSAGES`, puis
`LANG` : préfixe `fr` → français) ; à défaut, l'anglais. La langue de l'interface web se règle
séparément, par le réglage `lang` de l'espace ; `WATCHOO_LANG` étant aussi la variable de ce réglage,
la définir impose la langue partout (voir
[Interface web](06-interface-web.md#langue-de-linterface)). Les mêmes textes existent dans les deux
langues, servis par le paquet `internal/i18n` sans dépendance (voir
[Architecture](03-architecture.md#langues-et-traductions)).

**Accès aux dépôts** : avec la CLI GitHub `gh` installée et connectée (`gh auth login`), Watchoo passe
par l'API GitHub ; il obtient le token uniquement avec `gh auth token` et ne le stocke jamais. Sans `gh`
(absent ou non connecté), il suit les dépôts par **clones Git locaux** (`git` doit être installé), avec
la clé SSH ou les identifiants Git de la machine, sans pull requests, releases ni étoiles. Le choix est
refait à chaque synchro (voir [Architecture](03-architecture.md#authentification--gh-facultatif) et
[Dépôts Git locaux](03-architecture.md#dépôts-git-locaux-v2)).

## Commandes

| Commande | Effet | Version |
| --- | --- | --- |
| `watchoo init` | Fait du répertoire courant un espace de supervision : vérifie `gh` (sans `gh` : `git`), puis crée `.watchoo/` avec la config et la base SQLite. Refuse si l'espace existe déjà | MVP (sans `gh` : V2) |
| `watchoo version` | Affiche la version de Watchoo, le commit compilé et sa date, la version de Go et la plateforme (`watchoo --version` fait de même). Fonctionne hors d'un espace et sans `gh` | MVP |
| `watchoo repo add owner/repo [--branch main,dev,release/*] [--label "Alice Martin"] [--flemme 3] [--dead 10] [--conventions-since AAAA-MM-JJ]` | Ajoute un dépôt à surveiller (toutes ses branches par défaut), avec un nom affiché, des seuils d'activité et une date de début des conventions propres facultatifs | MVP (seuils : V2 ; date des conventions : 1.7.0) |
| `watchoo repo import fichier.csv [--dry-run]` | Ajoute d'un coup une liste de dépôts (ex. ceux d'une promotion), vérifie l'accès à chacun et affiche un bilan | MVP |
| `watchoo repo list` | Tableau : dépôt, dernier commit, auteur, état (Actif / Flemme / Dead) | MVP |
| `watchoo repo remove owner/repo [--keep-history] [--yes]` | Arrête le suivi et supprime l'historique du dépôt (et son clone Git local) après confirmation (`--keep-history` pour les garder) | MVP |
| `watchoo sync [owner/repo]` | Synchronise tout ou un seul dépôt | MVP |
| `watchoo log [--repo X] [--author Y] [--branch B] [--since 7d] [-n 20]` | Derniers commits filtrés | MVP |
| `watchoo contributors [--repo X] [--since 30d \| --weeks 12]` | Classement des contributeurs ; avec `--weeks`, leurs commits semaine par semaine | MVP (`--weeks` : V2) |
| `watchoo prs [--repo X] [--state open\|merged\|closed\|all] [-n 20]` | Pull requests des dépôts suivis, de la plus récemment modifiée à la plus ancienne | V2 |
| `watchoo releases [--repo X] [-n 20]` | Releases des dépôts suivis, puis leurs tags (`-n` ou `--limit`) | V2 |
| `watchoo languages [--repo X]` | Langages des dépôts suivis, avec leur pourcentage | 1.1.0 |
| `watchoo diagnostic [--repo X] [--author Y] [--since 30d]` | Pistes de réflexion du diagnostic, **réservées au formateur** : commits et pull requests qui déclarent l'usage d'un outil d'IA (niveau « déclaré » : signaux D1, D2, D3, D5), commits dont le rythme et la forme convergent (niveau « probable » : au moins deux signaux R et un signal F) et commits ou jours au rythme ou à la forme inhabituels pour l'apprenant (niveau « à regarder » : un signal R1 à R6, ou deux signaux F1 à F5 ; un signal F seul ne lève rien), signaux absolus A1 à A7 pour les projets rendus en peu de commits et traces de travail humain H1 à H4 qui abaissent « probable » (phase 7.7), avec leurs signaux, le contexte D4 des dépôts et les historiques encore insuffisants. Une piste n'est jamais un verdict. Diagnostic désactivé : la commande le dit, sans erreur (voir [Supervision et diagnostic](12-supervision-diagnostic.md)) | 7.2 (rythme : 7.3 ; forme : 7.5) |
| `watchoo diagnostic backfill [--repo X]` | Mesure sur les clones locaux l'historique déjà en base sans signaux (commits synchronisés avant l'activation du diagnostic), et complète la forme (F1 à F5) des commits mesurés avant la phase 7.5, dans la durée de conservation, hors fusions | 7.3 |
| `watchoo conventions [owner/repo] [--commits] [--failing] [--contributor X] [--milestone vX.Y.Z]` | Taux de commits conformes aux [conventions de projet](13-conventions.md) (CC1 à CC4), **réservés au formateur** : par dépôt, par contributeur et, avec `--commits`, par commit (SHA court, auteur et codes d'écart, jamais le message) ; depuis 1.8.0, noms de branche, titres de pull request (numéro et codes, jamais le titre), tags, jalons, releases en information, formulation, pieds et langue des commits, CC1 par intervalle entre tags, tendance de CC3 et, avec `--milestone`, la revue de jalon. Aucune note globale. Conventions désactivées : la commande le dit, sans erreur | 1.7.0 (`--milestone`, indicateurs de la phase 8.2 : 1.8.0 ; critères de fichiers : 1.9.0 ; historique et versions de la phase 8.4 : 1.10.0 ; fusions de la phase 8.5 : 1.11.0 ; relectures de la phase 8.6 : 1.12.0) |
| `watchoo status` | Résumé : dépôts à jour, inactifs, en erreur, source des synchros (API GitHub ou clones Git locaux), quota API restant, compte `gh` utilisé et ses droits (sans afficher le token) | MVP (source : V2) |
| `watchoo serve [--addr 127.0.0.1:8080] [--interval 30m]` | Lance l'interface web et la synchro périodique (30 min par défaut, 15 min minimum) | MVP |
| `watchoo tui [--interval 30m]` | Ouvre le mode interactif dans le terminal | MVP |
| `watchoo config export [-o fichier] [--force]` | Écrit la configuration partageable de l'espace (dépôts suivis et réglages communs) dans un fichier JSON, pour les autres personnes qui supervisent le même groupe | Après MVP |
| `watchoo update [--check] [--yes]` | Met Watchoo à jour depuis les releases GitHub après confirmation (archive vérifiée par SHA-256, binaire remplacé d'un bloc) ; `--check` vérifie seulement. Fonctionne hors d'un espace et sans `gh` (voir [Mise à jour](#mise-à-jour--watchoo-update)) | 0.6.0 |
| `watchoo config import fichier [--dry-run] [--yes]` | Fusionne un fichier de configuration partagé dans l'espace, après un aperçu : ajoute les dépôts absents, met à jour les autres, réécrit les réglages communs ; ne retire jamais rien | Après MVP |

### Détails

- **`repo add`** accepte `owner/repo` ou une URL GitHub complète. Sans `--branch`, **toutes les branches**
  du dépôt sont suivies, y compris celles créées plus tard. `--branch` restreint le suivi à une liste et à
  des motifs (`--branch main,dev,release/*`) ; une nouvelle branche qui correspond à un motif est suivie
  automatiquement. Voir [Architecture](03-architecture.md#branches-suivies).
  `--branch ""` revient à toutes les branches. `--flemme N` et `--dead N` fixent les **seuils
  d'activité propres** au dépôt, en jours sans commit, à la place de ceux de l'espace
  (`activity.flemme_after_days`, `activity.dead_after_days`) ; chacun est facultatif et indépendant,
  `0` revient au seuil de l'espace. Un seuil vaut au plus 3 650 jours et, quand les deux sont donnés,
  `--dead` doit être strictement supérieur à `--flemme` (sinon refus avant tout appel à GitHub).
  `--conventions-since AAAA-MM-JJ` (1.7.0) fixe la **date de début des [conventions](13-conventions.md)**
  propre au dépôt, à la place de `conventions.since` : une date du calendrier, interprétée à minuit dans
  le fuseau de l'espace ; `--conventions-since ""` revient à la date de l'espace ; une autre forme ou une
  date impossible est refusée avant tout appel à GitHub. Un
  dépôt déjà suivi n'est pas ajouté en double : son label, ses branches, ses seuils et sa date de début
  des conventions sont mis à jour s'ils sont donnés, et son suivi reprend s'il avait été arrêté. Le
  compte rendu cite les seuils propres (« seuils : flemme 3 j, dead 10 j ») et la date propre
  (« conventions depuis le 2026-09-01 ») ; `repo list` ajoute sous le tableau une ligne par dépôt qui a
  sa propre date. Le fichier d'import de `repo import` n'a pas de colonne pour cette date. L'accès est vérifié auprès de GitHub (une requête) ; `repo add`
  ne lance pas de synchro. Sans `gh` (V2), l'accès est vérifié par `git ls-remote`, sans rien cloner :
  un échec donne « accès refusé » ; le dépôt est enregistré sans identifiant GitHub (`provider_id`
  inconnu, résolu à la première synchro par l'API) et avec l'URL `https://github.com/owner/repo`.
- **`repo list`** affiche tous les dépôts de l'espace, suivi arrêté compris. La colonne DÉPÔT montre le
  label s'il existe, sinon `owner/repo` ; l'auteur est celui du dernier commit. États gris : `arrêté`
  (suivi arrêté), `erreur`, `jamais` (jamais synchronisé), `archivé`, `vide` (synchronisé sans commit).
- **`repo remove`** demande confirmation (`[o/N]`) quand l'entrée standard est un terminal ; sinon (script,
  tube) l'option **`--yes` / `-y` est obligatoire**. Une réponse autre que `o`, `oui`, `y` ou `yes`, ou un
  Ctrl+C pendant la question (pris en compte tout de suite), annule avec un code de retour non nul. Sans `--keep-history`, le dépôt, ses branches, commits et synchros sont
  supprimés de la base (les contributeurs restent), ainsi que son clone Git local
  (`repos/<name>` à la racine de l'espace, V2) s'il existe, et le clone d'un homonyme qui reprend ce nom
  est déplacé (voir [Dépôts Git locaux](03-architecture.md#dépôts-git-locaux-v2)) ; avec `--keep-history`, le suivi s'arrête (le dépôt
  n'est plus synchronisé par `watchoo sync`, il apparaît `arrêté`), l'historique en base **et** le clone
  sont gardés, et `repo add` le reprend.
- **`sync`** affiche un tableau par dépôt (statut `ok`, `erreur`, `ignoré` s'il est déjà en cours de
  synchronisation, `reporté` si le quota est bas, avec l'heure de reprise en heure locale), le bilan et le
  quota restant. `watchoo sync owner/repo` synchronise ce dépôt même si son suivi est arrêté. Code de
  retour non nul si un dépôt est en erreur. Un dépôt synchronisé dont les pull requests, releases ou
  tags n'ont pas pu être lus, ou dont le clone Git local est indisponible avec `gh` (statistiques de
  lignes alors lues par l'API, limitées), reste `ok` : l'avertissement est dans la colonne DÉTAIL (et
  `warnings` en JSON), et ne change pas le code de retour. Sans `gh` (V2), le bilan se termine par « Source : clones
  Git locaux (gh absent ou non connecté ; « gh auth login » pour passer par l'API GitHub) » et ne
  donne pas de quota ; un dépôt que `git` ne peut pas lire est `erreur` avec « accès refusé ».
- **`--since`** accepte une durée relative : un entier suivi de `d` (jours), `h` (heures) ou `m`
  (minutes), par exemple `7d`, `30d`, `12h`.
- **`log`** affiche, pour chaque commit, la date, le dépôt, l'auteur, le SHA court, ses lignes (colonne
  `+/-` : « +12/-3 », « +0/-0 » pour un commit réellement sans ligne, « - » si elles sont encore
  inconnues ; 0.5.0), les outils d'IA signés dessus (colonne IA, « - » si aucun ; 1.2.0, voir
  [Signatures d'IA](02-fonctionnalites.md#signatures-dia)) et la première ligne du message.
- **Signatures d'IA dans le TUI** (1.2.0) : une ligne dans le détail d'un contributeur, « 3/120 (Claude 2,
  Cursor 1) », seulement si une signature existe.
- **Langages dans le TUI** (1.2.0) : une ligne dans le détail d'un contributeur, « PHP 52,0 %, Twig 33,5 %,
  autres 14,5 % » (cinq langages, puis le reste regroupé), « - » si aucun commit n'est encore analysé.
- **Lignes `+/-` dans le TUI** (0.5.0) : par commit, « +12/-3 », « +0/-0 » ou « - » (inconnues) ; les
  totaux (contributeur, dépôt) additionnent les commits connus et ajoutent « (N non comptés) » quand une
  partie des commits n'en a pas encore, « - » si aucun n'en a.
- **`log -n`** (ou `--limit`) limite le nombre de commits affichés (20 par défaut, `0` = tous). Un commit
  présent sur plusieurs branches n'apparaît qu'une fois.
- **`log --author`** compare le login GitHub, le nom ou un e-mail de l'auteur, **à l'identique** (pas de
  sous-chaîne), sans tenir compte de la casse des lettres ASCII (« ÉLODIE » ne trouve pas « élodie »).
- **`log --branch`** montre les commits **vus sur cette branche** pendant les synchros : ceux qu'elle
  partage avec la branche de référence (souvent `main`) n'y sont rattachés que s'ils ont été vus en la
  comparant (voir [Architecture](03-architecture.md#synchronisation-incrémentale)).
- **Colonne IA de `contributors`** (1.2.0) : commits signés / commits analysés (« 3/120 »), « - » si aucun
  n'est encore analysé. Une ligne sous le tableau rappelle, dès qu'une signature est affichée, que son
  absence ne prouve rien.
- **`contributors`** classe les **auteurs** de commits par nombre de commits. Les lignes ajoutées et
  supprimées sont lues dans le clone Git local de chaque dépôt, pour tous les commits (premier import
  compris) ; elles additionnent les commits dont les statistiques sont connues (voir
  [Synchronisation incrémentale](03-architecture.md#synchronisation-incrémentale) : seul un clone
  indisponible les laisse inconnues, sauf pour les 100 commits par synchro que l'API donne alors).
  **`--weeks N`** (1 à 52) limite la période aux N dernières semaines (du lundi au dimanche, dans le
  fuseau d'affichage, la semaine en cours comprise) et donne, pour chaque contributeur de cette période,
  ses commits semaine par semaine : en texte, une colonne par semaine (datée du lundi, « 12/10 ») puis
  le total ; en JSON, la clé `weekly`. Avec `--repo`, seuls les commits du dépôt comptent, chacun une
  fois quelle que soit sa branche. `--weeks` et `--since` ne vont pas ensemble (refus).
- **`prs`** liste les pull requests enregistrées par les synchros (voir
  [Synchronisation incrémentale](03-architecture.md#synchronisation-incrémentale)), de la plus récemment
  modifiée à la plus ancienne : modification (date relative), dépôt, numéro, état (`ouverte`,
  `brouillon`, `mergée`, `fermée` ; fermée = sans merge), auteur (login), titre (première ligne,
  nettoyée comme un message de commit). `--state` filtre : `open` (ouvertes, brouillons compris),
  `merged`, `closed` (fermées sans merge) ou `all` (par défaut). `-n` (ou `--limit`) : 20 par défaut,
  `0` = toutes.
- **`releases`** affiche deux tableaux : les **releases** (publication, ou création pour un brouillon,
  en date relative ; dépôt, tag, titre, état `publiée`, `pré-release` ou `brouillon`, auteur), de la
  plus récente à la plus ancienne, puis les **tags** (dépôt, tag, SHA court du commit visé et date de ce
  commit s'il est en base, sinon `-`), du commit le plus récent au plus ancien. `-n` limite chaque
  tableau (20 par défaut, `0` = tout). Les brouillons ne sont visibles que si le compte `gh` a le droit
  d'écriture sur le dépôt (règle de GitHub).
- **`languages`** (1.1.0) affiche, par dépôt, ses langages du plus gros au plus petit : nom, part des octets
  (une décimale) et taille, sur la branche par défaut. Ils sont calculés à chaque synchro depuis le clone Git
  local (voir [Architecture](03-architecture.md#langages-dun-dépôt-110)) : un dépôt n'en a aucun avant sa
  prochaine synchro. Le TUI les donne dans le détail d'un dépôt.
- **Langages de `contributors`** (1.2.0) : colonne LANGAGE, le langage où le contributeur a ajouté le plus de
  lignes sur la période et le dépôt du classement, « - » si aucun de ses commits n'est encore analysé ; en
  JSON, tous ses langages. Règles : [Architecture](03-architecture.md#langages-dun-contributeur-120).
- **Part des commits** (1.1.0) : `contributors` donne la part de chaque contributeur (colonne PART ; `share`
  en JSON), par rapport aux commits du classement : avec `--repo`, ceux du dépôt ; avec `--since` ou
  `--weeks`, ceux de la période. Un commit sans auteur connu ne compte pas. Le tableau de `--weeks` n'a pas
  cette colonne, le JSON si. Le TUI donne cette part dans le détail d'un dépôt.
- **`conventions`** (1.7.0) évalue les messages de commit avec le parseur strict de la charte (réglages
  `conventions.commit.*`, voir [Conventions de projet](13-conventions.md#51-commits-cc)). **Aucune note** :
  un tableau par dépôt donne le taux `n/m` (commits sans écart bloquant / commits évalués) et le pourcentage
  (« — » sans commit évalué : inconnu, pas 0 %), la vue informative de la seule branche par défaut, le
  nombre de commits exclus, la date de début effective et la source du périmètre ; puis, par dépôt, la date
  de début et son origine (espace ou propre au dépôt), les exclusions par motif (fusions, bots, commit
  initial de GitHub, comptes de `conventions.excluded_accounts`), les commits antérieurs à la date de début
  (comptés à part) et les écarts par code avec leur libellé et leur gravité (bloquant ou avertissement) ;
  enfin un tableau des contributeurs (taux et codes reçus, triés par nom, jamais classés). Le périmètre est
  **toutes les branches suivies**, chaque commit une fois, lu dans le clone Git local existant (`git
  rev-list` des `origin/<branche>` suivies, sans `fetch`) ; sans clone lisible, il se replie sur les
  commits rattachés aux branches suivies en base (source « base », marquée d'un astérisque et de la
  mention « peut inclure des commits réécrits »). Un commit dont le nombre de parents est encore inconnu
  est reconnu comme fusion à son message par défaut (une ligne le rappelle). `owner/repo` limite à un dépôt
  suivi ; `--contributor` à un auteur (login, nom ou e-mail exact) ; `--commits` ajoute la liste des commits
  évalués (date, dépôt, SHA court, auteur, « conforme » ou « non conforme » et codes) et `--failing` la
  limite aux commits non conformes (il implique `--commits`). **Le message de commit n'est jamais
  affiché**, ni en texte ni en JSON. Le code de sortie est `0` quels que soient les taux (pas d'option
  `--strict`). Conventions désactivées (`conventions.enabled: false`, défaut) : un message le dit ; en
  JSON, `enabled: false` et des listes vides.
  Depuis 1.8.0 (phase 8.2, voir [Conventions de projet](13-conventions.md#54-tags-et-semver-sv)),
  le détail de chaque dépôt ajoute : les **branches** (BR1 : décompte « n/m conformes, k à revoir » et
  source, `clone` ou « base, branches suivies seulement », puis les noms par code avec leur libellé et leur
  gravité, une branche de tête suivie des numéros de ses pull requests) ; les **pull requests** (PR1 et
  PR2 : titres conventionnels des ouvertes et des fusionnées à part, titres par défaut, PR écartées, puis
  chaque pull request non conforme sous la forme `#numéro auteur (branche de tête) : codes` ; **jamais le
  titre** ; « inconnu » en clone seul ou pour un dépôt jamais vu par l'API) ; les **tags** (SV1, noms par
  code), les **jalons** de `conventions.tags.milestones` (SV2 : présent, nom non conforme, absent) et les
  **releases** (SV6, information seule) ; la **formulation** des sujets (avertissements CC5 : passé, forme
  en « -ing », première personne), les **pieds** et références d'issue (CC6), la **langue** des sujets
  (CC7 : `fr`, `en`, inconnue, hors `conventions.commit.language`, « mélange ») ; enfin **CC1 entre tags**
  (chaque intervalle entre tags finals conformes, `début→v0.1.0`, `v0.1.0→v1.0.0`…, puis depuis le
  dernier ; lu dans le clone, « inconnu » sans clone). Le tableau des contributeurs ajoute les deux taux de
  la **tendance de CC3** (jusqu'au et depuis le dernier tag final de chaque dépôt, « — » si non mesurée),
  les titres de pull request conformes et les branches conformes (celles de leurs pull requests).
  `--milestone vX.Y.Z` (1.8.0) affiche, à la place de la vue d'ensemble, la **revue de jalon** d'un jalon
  de `conventions.tags.milestones` : par dépôt, « k/n critères conformes, u inconnus », puis CC1 sur
  l'intervalle du jalon, BR1, SV1 et SV2, chacun conforme, non conforme ou inconnu avec sa raison, jamais
  de note globale ; un dépôt sans date de début est « non évaluable (date de début manquante) ». La revue
  porte sur les dépôts : `--contributor` ne la restreint pas. Un jalon non configuré est une erreur
  d'usage (code de sortie `1`) qui liste les jalons configurés.
  Depuis 1.9.0 (phase 8.3, voir [Conventions de projet](13-conventions.md#55-changelog-cl)), le détail de
  chaque dépôt ajoute les **critères de fichiers**, mesurés par la synchro sur l'arbre de la tête de la
  branche par défaut du clone local et gardés en cache : la révision mesurée (SHA court, date du calcul),
  puis une grille par famille, **CHANGELOG** (CL1 à CL8), **README** (RD1 à RD4), **documentation** (DOC1 à
  DOC5, DOC8) et **outillage** (TL1 à TL3), en « k/n critères conformes, u inconnus », suivie de chaque
  critère non conforme, inconnu (avec sa raison) ou porteur d'un avertissement, avec ses codes et leur
  libellé. Rien de calculé encore : « inconnus (pas encore calculé, lancez watchoo sync) » ; réglages,
  règles ou révision changés depuis : « à recalculer » ; sans clone local : « pas de clone local ». Avec un
  dépôt (`watchoo conventions owner/repo`, vue détaillée), les **listes de chemins** sont recalculées depuis
  le clone, jamais stockées : documents non référencés par l'INDEX (DOC3), liens morts `source → cible`
  (DOC4), dossiers et fichiers mal nommés (DOC5), archives et documents bureautiques (DOC8), au plus 20
  chemins par liste en texte. La revue de jalon ajoute CL9, CL8, RD1, RD2 (section installation seulement ;
  les sections trouvées sont données en information), RD3, DOC1 et DOC3, lus sur l'arbre du tag du jalon ;
  tag absent : « inconnu » avec la raison de SV1. Indicateurs de dépôt, jamais attribués à une personne :
  `--contributor` les donne « inconnus (rapport restreint à des contributeurs) ».
  Depuis 1.10.0 (phase 8.4, voir [Conventions de projet](13-conventions.md#54-tags-et-semver-sv)), le
  détail de chaque dépôt se termine par son **historique**, observé par les synchros quand les conventions
  sont activées et attribué à personne : les **réécritures de la branche par défaut** (BR4 : nombre, date de
  la dernière, puis chaque événement en `branche le date : ancien → nouveau`, SHA courts ; sinon « aucune
  réécriture de la branche par défaut observée », jamais « jamais réécrite »), l'**historique des tags**
  (SV3 : « n sans réécriture observée depuis le <première observation>, r réécrit(s), d supprimé(s), p supprimé(s) puis reposé(s) sur le même commit », puis
  chaque tag qui a changé), les **échéances** des jalons (SV2 : à l'heure, en retard, ou indéterminé « posé
  entre le X et le Y »), les **pré-versions** (avertissements SV4 : forme libre, posée après la version
  finale, précédence contraire à l'historique), la **précédence** des versions finales (SV7 : version
  supérieure posée sur un ancêtre d'une version inférieure ; incrément sauté en information), les **sauts de
  version** (SV5 : cohérent, sous-versionné avec le saut attendu, indéterminé, inconnu) et la **fraîcheur du
  CHANGELOG** (CL10, information : commits `feat` et `fix` depuis le dernier commit qui a touché le
  CHANGELOG, grisée quand le CHANGELOG du dernier jalon est à jour). SV5 et CL10 sont lus dans le clone local
  (« inconnu (pas de clone local) » sans clone) ; sans clone, SV4 juge la pose d'une pré-version par la
  première observation des tags et SV7 ne garde que l'incrément sauté (information). Avec `--contributor`, cet historique n'est pas
  affiché. La revue de jalon ajoute SV3 après SV2 (12 critères) et juge l'échéance dans SV2.
  Depuis 1.11.0 (phase 8.5, voir [Conventions de projet](13-conventions.md#53-pull-requests-pr)), le détail
  de chaque dépôt ajoute les **fusions** : les commits arrivés sur la branche par défaut hors pull request
  (PR3 : « n sur m (par PR p, indéterminés u) », lus sur le premier parent de la branche par défaut du clone
  local depuis la date de début, puis chaque commit hors PR sous la forme `SHA court auteur (auteur) :
  raison`, attribué à l'auteur du commit, jamais à qui l'a poussé ; clone seul : lu sur les messages de
  GitHub), les **descriptions** des pull requests (PR5 : conformes sur celles lues, puis ce qui manque à
  chacune), les **fusions** (PR6 : en squash au message conforme, par méthode), les **branches réutilisées**
  (BR2) et les **méthodes de fusion autorisées** (BR5 : conforme si seul le squash est autorisé ; « inconnu
  (méthodes de fusion non renvoyées …) » sans droit push). Les pull requests issues d'un fork sont écartées
  (« issues d'un fork »). Le tableau des contributeurs ajoute « CORPS DE PR » et « HORS PR » (commits de la
  branche par défaut dont il est l'auteur arrivés hors PR, sur ceux classés). La revue de jalon ajoute PR3
  après BR1 (13 critères) : aucun commit hors pull request sur l'intervalle du jalon. Ni description, ni
  titre, ni message de commit n'est jamais affiché.
  Depuis 1.12.0 (phase 8.6, mode API), les fusions donnent aussi les **relectures** (PR4) : « pull requests
  fusionnées relues par un autre compte : r/k (u pas encore lues) », r relues sur les k pull requests
  fusionnées dont les relectures ont été lues après la fusion, puis « fusionnées sans relecture (n) : #4
  (auteur), … » (numéro et auteur, jamais le titre). Le tableau des contributeurs ajoute « PR RELUES »
  (« 1/2, 3 relecture(s) reçue(s) » : ses pull requests fusionnées relues, et les relectures reçues sur ses
  pull requests ; « 3 relecture(s) reçue(s) » seul tant qu'aucune de ses pull requests fusionnées n'a ses
  relectures lues, « - » sans aucune pull request lue ; les relectures données ne sont pas comptées, faute
  de garder les relecteurs). La revue de
  jalon ajoute PR4 après PR3 (14 critères) : toutes les pull requests fusionnées dans l'intervalle du jalon
  relues (« r/m pull requests fusionnées relues (u pas encore lues) (#n, …) ») ; inconnu en clone seul
  (`no_api`), sans pull request fusionnée (`no_merged_pr`), tant que des relectures ne sont pas lues
  (`reviews_unchecked`) ou si la date du commit d'un tag est absente de la base (`no_tag_date`). Aucun login
  de relecteur n'est jamais affiché.
- **`status`** : compte `gh` et ses droits, **source** des synchros (V2 : « Source : API GitHub (gh
  connecté) » ou « Source : clones Git locaux », avec le rappel de `gh auth login`), quota restant
  (lecture gratuite ; « sans objet » avec les clones Git locaux), dépôts par état, dépôts en erreur,
  dernière synchro réussie, branches suivies. Sous 10 % de quota restant, le garde-fou est signalé
  (« Quota bas : synchros reportées jusqu'à HH:MM », `quota_low` en JSON), avec la même règle que le
  moteur. Un `gh` absent ou non connecté est signalé sans faire échouer la commande. La source est celle
  que prendrait une synchro lancée maintenant (`gh auth token`, sans contacter GitHub). Le token n'est
  jamais affiché.
- **Sorties** : un tableau texte aligné, sans bordure, en-têtes en majuscules ; dates relatives
  (« il y a 2 h ») en heure locale (réglage `timezone`, sinon fuseau du système). Couleurs (en-têtes en
  gras, états gris en gris) seulement sur un terminal, sans `NO_COLOR` ni `TERM=dumb`. Sur un terminal
  sans Unicode, symboles et en-têtes ASCII (`! 42 j`, `DEPOT`) ; une sortie redirigée reste en UTF-8.
  Les textes venus des dépôts ou de la base (messages de commit, noms Git, labels, messages d'erreur)
  sont **nettoyés** avant d'être écrits, en texte comme dans le TUI : séquences d'échappement (ESC, CSI,
  OSC) retirées, autres caractères de contrôle et de format (sauts de ligne, marques de sens d'écriture)
  remplacés par une espace. Un étudiant ne peut donc ni changer le titre de la fenêtre ni effacer
  l'écran de l'enseignant. Le JSON garde le texte d'origine, échappé par l'encodeur.
- **Labels** : `repo add --label` applique la même règle que l'import (UTF-8 valide, aucun caractère de
  contrôle, 200 caractères au plus) ; un label invalide est refusé avant tout appel à GitHub.
- **`--json`** : chaque commande qui produit des données écrit un objet JSON stable (clés en anglais,
  `snake_case`, dates UTC RFC 3339 ou `null`) ; ses clés sont listées dans [Sortie JSON](#sortie-json)
  et vérifiées par les tests. Les erreurs vont sur la sortie d'erreur.
- **Hors espace** : toutes les commandes sauf `init`, `version`, `update` et `help` s'arrêtent avec « aucun espace
  Watchoo ici, lancez « watchoo init » (ou indiquez un espace avec -C <répertoire>) ».
- **`serve`** écoute sur `web.addr` (`127.0.0.1:8080` par défaut ; `--addr` le remplace) et affiche
  l'URL à ouvrir (carte d'en-tête sur un terminal). Hors boucle locale (`127.0.0.0/8`, `::1`,
  `localhost`), il refuse de démarrer sans authentification : `web.auth.username` avec le mot de passe
  dans la variable `WATCHOO_WEB_PASSWORD` (12 caractères au moins, jamais dans un fichier), ou
  `web.trusted_proxy: true` derrière un reverse proxy authentifiant, avec ses noms publics dans
  `web.allowed_hosts` (avertissement au démarrage ; sans `web.allowed_hosts`, refus). Port
  déjà pris : message clair. `--interval` comme pour `tui` (`0` = synchro manuelle seule) ; au démarrage,
  une synchro n'est lancée que si la dernière date de plus d'un intervalle. `--json` est refusé. Ctrl+C
  (ou SIGTERM) arrête proprement : plus de nouvelle requête, synchro en cours annulée et attendue, base
  fermée ensuite. Voir [Interface web](06-interface-web.md).
- `watchoo help [commande]` et l'option `-h` / `--help` affichent l'aide dans la langue des messages
  (anglais par défaut, français). Il n'y a pas de
  commande `completion` (celle que `cobra` ajoute d'office est désactivée : son aide n'existe qu'en anglais).
- **`--version`**, option de `watchoo` seul (`watchoo init --version` est refusée), a le même effet que
  `watchoo version`, `--json` compris.

## Codes de sortie

| Code | Sens |
| --- | --- |
| `0` | Succès |
| `1` | Tout échec : option ou argument invalide, espace introuvable, erreur de la commande, confirmation annulée, Ctrl+C, et échecs partiels (`sync` avec un dépôt en erreur, `repo import` et `config import` avec un dépôt inaccessible ou en erreur) |

Le message est sur la sortie d'erreur. Une distinction plus fine entre les échecs, si elle vient un
jour, utilisera de nouveaux codes supérieurs à `1` et sera un changement compatible.

## Variables d'environnement

Chaque réglage de `config.yaml` peut être remplacé par une variable `WATCHOO_<SECTION>_<CLÉ>` (voir
[Espaces de supervision](10-espaces-de-supervision.md)) ; les listes s'écrivent séparées par des virgules
(`"dependabot/*, renovate/*"`).

| Variable | Rôle |
| --- | --- |
| `WATCHOO_NAME` | Réglage `name` |
| `WATCHOO_SYNC_AUTO`, `WATCHOO_SYNC_INTERVAL`, `WATCHOO_SYNC_PARALLELISM` | Réglages `sync.*` |
| `WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS`, `WATCHOO_ACTIVITY_DEAD_AFTER_DAYS` | Réglages `activity.*` |
| `WATCHOO_BRANCHES_EXCLUDE` | Réglage `branches.exclude` (liste) |
| `WATCHOO_WEB_ADDR`, `WATCHOO_WEB_AUTH_USERNAME`, `WATCHOO_WEB_TRUSTED_PROXY`, `WATCHOO_WEB_ALLOWED_HOSTS` | Réglages `web.*` (`allowed_hosts` : liste) |
| `WATCHOO_TIMEZONE` | Réglage `timezone` |
| `WATCHOO_LANG` | Langue de la CLI et du TUI, et réglage `lang` de l'interface web (`en` ou `fr`) |
| `WATCHOO_GITHUB_USER`, `WATCHOO_GITHUB_GH_PATH` | Réglages `github.*` |
| `WATCHOO_GIT_PROTOCOL` | Réglage `git.protocol` |
| `WATCHOO_DIAGNOSTIC_ENABLED`, `WATCHOO_DIAGNOSTIC_EXCLUDE`, `WATCHOO_DIAGNOSTIC_RETENTION_DAYS`, `WATCHOO_DIAGNOSTIC_SPEED_SIGNAL` | Réglages `diagnostic.*` (`exclude` : liste ; voir [Supervision et diagnostic](12-supervision-diagnostic.md)) |
| `WATCHOO_CONVENTIONS_ENABLED`, `WATCHOO_CONVENTIONS_SINCE`, `WATCHOO_CONVENTIONS_COMMIT_TYPES`, `WATCHOO_CONVENTIONS_COMMIT_SCOPES`, `WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_SOFT`, `WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_HARD`, `WATCHOO_CONVENTIONS_COMMIT_FORBIDDEN`, `WATCHOO_CONVENTIONS_COMMIT_LANGUAGE`, `WATCHOO_CONVENTIONS_BRANCH_PATTERN`, `WATCHOO_CONVENTIONS_BRANCH_EXCLUDE`, `WATCHOO_CONVENTIONS_PR_BODY_SECTIONS`, `WATCHOO_CONVENTIONS_PR_REQUIRE_CLOSES`, `WATCHOO_CONVENTIONS_PR_MIN_REVIEWS`, `WATCHOO_CONVENTIONS_PR_REVIEWER_MUST_CONTRIBUTE`, `WATCHOO_CONVENTIONS_TAGS_MILESTONES`, `WATCHOO_CONVENTIONS_TAGS_PRERELEASE_FORMS`, `WATCHOO_CONVENTIONS_CHANGELOG_CATEGORIES_FR`, `WATCHOO_CONVENTIONS_README_SECTIONS`, `WATCHOO_CONVENTIONS_README_MIN_WORDS`, `WATCHOO_CONVENTIONS_DOCS_DIR`, `WATCHOO_CONVENTIONS_DOCS_MEDIA_DIR`, `WATCHOO_CONVENTIONS_DOCS_ARCHIVES_SCOPE`, `WATCHOO_CONVENTIONS_EXCLUDED_ACCOUNTS` | Réglages `conventions.*` (`types`, `scopes`, `forbidden`, `branch.exclude`, `tags.milestones`, `tags.prerelease_forms`, `pr.body_sections`, `readme.sections`, `excluded_accounts` : listes ; `since` : `AAAA-MM-JJ` ; `branch.pattern` : expression RE2, jamais découpée ; `tags.milestones` : `vX.Y.Z` ou `vX.Y.Z@AAAA-MM-JJ` ; `readme.sections` : clés parmi `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`, `license` ; `readme.min_words` : au moins 1 ; `docs.archives_scope` : `repository` ou `documentation` ; `tags.prerelease_forms` : au moins une forme, mots en minuscules ; réglages des branches et des tags : 1.8.0 ; réglages du CHANGELOG, du README et de la documentation : 1.9.0 ; `pr.body_sections` : clés parmi `context`, `changes`, `testing`, `screenshots` ; `tags.prerelease_forms` : 1.10.0 ; `pr.body_sections` et `pr.require_closes` : 1.11.0 ; `pr.min_reviews` : au moins 1 ; `pr.min_reviews` et `pr.reviewer_must_contribute` : 1.12.0 ; voir [Conventions de projet](13-conventions.md#8-configuration)) |
| `WATCHOO_DIR` | Espace à utiliser (comme `-C`) |
| `WATCHOO_WEB_PASSWORD` | Mot de passe de l'interface web (jamais dans un fichier) |
| `WATCHOO_LOG_LEVEL` | Niveau de log (`debug`, `info`, `warn` par défaut, `error`) |
| `WATCHOO_NO_UPDATE_CHECK` | Coupe la vérification de nouvelle version |
| `WATCHOO_VERSION`, `WATCHOO_INSTALL_DIR` | Script d'installation seulement (`install.sh`) |

Les autres variables `WATCHOO_*` (`WATCHOO_DOCKER`, posée par l'image Docker ; `WATCHOO_RELEASES_URL`,
pour les tests du script d'installation) sont internes et hors de la surface publique.

## Sortie JSON

Avec `--json`, chaque commande qui produit des données écrit **un seul objet JSON** (indenté, suivi d'une
fin de ligne) sur la sortie standard, et rien d'autre : ni carte d'en-tête, ni astuce. Avertissements,
questions de confirmation et erreurs vont sur la sortie d'erreur ; en cas d'erreur, la sortie standard
reste vide (sauf `repo import` et `config import`, qui écrivent leur bilan puis rendent un code non nul
s'il reste des dépôts inaccessibles).

Règles communes, qui forment un **contrat stable** pour les scripts (une clé n'est ni renommée ni
retirée sans nouvelle version majeure depuis la 1.0.0, voir [Skills](11-skills.md)) :

- clés en anglais, en `snake_case` ; toutes les clés sont toujours présentes (sauf dans le document de
  `config export`, dont les clés facultatives suivent [Partage de la configuration](#partage-de-la-configuration--watchoo-config)) ;
- dates en UTC, RFC 3339 à la seconde (`"2026-10-27T10:00:00Z"`), `null` si inconnues ;
- listes vides écrites `[]`, jamais `null` ; un texte absent vaut `""` ;
- textes venus des dépôts (messages, noms, labels) gardés tels quels, échappés par l'encodeur ;
- jamais de token. `tui` et `serve` refusent `--json`.

Ce que couvre le contrat (1.0.0) :

- **changements compatibles** (MINOR) : ajouter une clé à un objet, ajouter une valeur à une liste de
  valeurs (`state`, `status`, `outcome`, `action`…) ; un script ignore les clés qu'il ne connaît pas et
  accepte une valeur inconnue ;
- **textes dans la langue choisie** (`WATCHOO_LANG`) : `state_label`, `message`, `error`, `reason`,
  `rate_limit_error` et les messages des `warnings` ; un script teste les codes et les états, pas ces
  textes ;
- **identifiants** : `id` est l'identifiant local dans la base de l'espace (sauf `releases[].id`,
  l'identifiant sur la forge) ; il ne vaut pas pour un autre espace, ni après un `repo remove` sans
  `--keep-history` suivi d'un `repo add` ;
- **`flemme`** (état, seuils, option `--flemme`, colonne de l'import, réglage) est le nom machine
  stable de l'état « Flemme » (« Slacking » en anglais), quelle que soit la langue des messages ;
- **hors contrat** : la sortie texte (colonnes, formulations), les logs (`WATCHOO_LOG_LEVEL`) et les
  variables internes.

Clés de premier niveau (la liste est vérifiée par un test, `internal/cli/json_schema_test.go`) :

| Commande | Clés de premier niveau | Notes |
| --- | --- | --- |
| `watchoo init` | `name`, `root`, `dir`, `config_file`, `database`, `parent_workspace`, `github`, `source`, `warnings` | `github` : `host`, `login`, `token_source`, `scopes` (vides sans `gh`) ; `source` : `api` ou `git` (clones Git locaux, sans `gh`, V2) ; `parent_workspace` vide sans espace parent |
| `watchoo version` | `version`, `commit`, `date`, `go`, `os`, `arch` | Voir ci-dessous |
| `watchoo repo add` | `outcome`, `repository` | `outcome` : `added`, `already` ou `updated` ; `repository` : objet dépôt |
| `watchoo repo import` | `file`, `format`, `dry_run`, `summary`, `entries` | `format` : `csv` ou `list` ; `summary` : `added`, `already_tracked`, `updated`, `inaccessible`, `errors` ; chaque entrée : `line`, `repo`, `outcome`, `repository` (`null` si inaccessible ou en erreur), `error` |
| `watchoo repo list` | `repositories` | Chaque élément : objet dépôt plus `activity`, `commits`, `tracked_branches`, `last_commit` (objet commit ou `null`), `last_sync` (`null` si aucune) |
| `watchoo repo remove` | `action`, `repository`, `deleted_commits` | `action` : `deleted`, `stopped` ou `already_stopped` |
| `watchoo sync` | `started_at`, `finished_at`, `summary`, `repositories`, `rate_limit`, `source` | `source` : `api`, `git` (clones Git locaux, V2 ; `rate_limit` est alors `null`) ou `""` si la synchro n'a pas commencé ; `summary` : `repositories`, `success`, `error`, `locked`, `deferred`, `new_commits` ; chaque dépôt : `id`, `full_name`, `display_name`, `status` (`success`, `error`, `locked` : déjà en cours de synchro, `ignoré` en texte ; `deferred` : quota bas ; `success` est `ok` en texte), `new_commits`, `tracked_branches`, `stats_fetched`, `duration_ms`, `error`, `resume_at`, `warnings` (messages en français : pull requests, releases ou tags non synchronisés, ou clone Git local indisponible avec `gh`, alors que le dépôt a réussi ; `[]` sinon) ; `stats_fetched` : statistiques de commits calculées dans le clone, ou demandées à l'API en repli |
| `watchoo log` | `commits` | Du plus récent au plus ancien ; objets commit |
| `watchoo contributors` | `contributors` | Chaque élément : `id`, `name`, `login`, `display_name`, `avatar_url`, `commits`, `additions`, `deletions`, `repositories`, `share` (1.1.0 : part des commits du classement, en pourcentage avec une décimale), `last_commit_at`, `ai_signed_commits`, `ai_scanned_commits`, `ai_tools` (1.2.0 : commits avec une signature, commits analysés, chaque outil avec `tool` et `commits` ; `0` n'est pas la preuve d'un travail sans IA), `languages_scanned_commits`, `languages` (1.2.0 : commits dont les langages sont lus ; chaque langage avec `name`, `lines` (lignes ajoutées) et `percent` (part de ses lignes, une décimale), de celui qui en a le plus au moins ; `[]` si aucun), `weekly` (avec `--weeks N` : N objets `week` (lundi, `AAAA-MM-JJ`, fuseau d'affichage) et `count`, du plus ancien au plus récent ; `[]` sans `--weeks`) |
| `watchoo prs` | `pull_requests` | De la plus récemment modifiée à la plus ancienne ; objets pull request |
| `watchoo releases` | `releases`, `tags` | Objets release (du plus récent au plus ancien) et tag ; `-n` limite chaque liste |
| `watchoo languages` | `languages` | D'un dépôt au suivant, plus gros langage d'abord. Chaque élément : `repository`, `name`, `bytes`, `percent` (part du dépôt, une décimale) |
| `watchoo status` | `workspace`, `github`, `source`, `rate_limit`, `rate_limit_error`, `quota_low`, `repositories`, `errors`, `last_synced_at`, `tracked_branches` | `workspace` : `name`, `root` ; `github` : `ok`, `host`, `login`, `token_source`, `scopes`, `error` ; `source` (V2) : `kind` (`api`, `git`, ou `""` si `gh` est en panne) et `reason` (pourquoi les clones Git locaux, ou l'erreur de `gh`) ; `repositories` : `total`, `active`, `flemme`, `dead`, `error`, `never_synced`, `archived`, `empty`, `stopped` ; chaque erreur : `repo`, `message` |
| `watchoo diagnostic` | `enabled`, `leads`, `contexts`, `histories`, `notice`, `filters` | `enabled` : réglage `diagnostic.enabled` (faux : listes vides) ; chaque piste : `kind` (`commit`, `pull_request` ou `day`), `level` (`declared`, `probable` ou `to_review`), `repository`, `sha`, `short_sha` (vides sauf pour un commit), `number` (0 sauf pour une PR), `title` (titre de la PR, vide sinon), `url`, `author` (`name`, `login`, `display_name`), `date` (écriture du commit, ouverture de la PR, ou début du jour), `signals` (chaque signal : `id` parmi `D1`, `D2`, `D3`, `D5`, `R1` à `R6`, `F1` à `F5`, `A1` à `A7`, `H1` à `H4` (contre-indicateurs, phase 7.7) ; `value` : outil, compte bot, branche ou mesure ; `raw`, `median`, `z` : mesure, médiane personnelle (F5 : part habituelle des commits avec tests) et score z modifié des signaux de rythme et de forme, `null` s'il n'y en a pas) ; chaque contexte : `repository`, `agent_config_since` ; chaque historique : `repository`, `author`, `commits`, `worked_days`, `sufficient` ; `notice` : rappel traduit ; `filters` : `repository`, `author`, `since` |
| `watchoo diagnostic backfill` | `repositories`, `measured` | Chaque dépôt : `repository`, `measured` (commits mesurés), `merges` (fusions écartées), `missing` (absents du clone), `error` (vide si réussi) ; code `1` si un dépôt a échoué |
| `watchoo conventions` | `enabled`, `rules_version`, `repositories`, `contributors`, `commits`, `notice`, `filters`, `milestone` | 1.7.0 (`milestone` et les clés de la phase 8.2 : 1.8.0 ; `changelog`, `readme`, `docs`, `tooling` et `milestone.repositories[].files` : 1.9.0 ; les clés de la phase 8.4 : 1.10.0 ; les clés de la phase 8.5 : 1.11.0 ; les clés de la phase 8.6 : 1.12.0). `enabled` : réglage `conventions.enabled` (faux : listes vides) ; `rules_version` : version des règles du parseur ; chaque dépôt : `repository`, `source` (`clone` ou `database`, repli qui peut inclure des commits réécrits), `since` (date de début effective `"AAAA-MM-JJ"`, `null` = depuis le début), `since_origin` (`workspace`, `repository` ou `null`), `commits` (`evaluated`, `conform`, `rate` (au millième, `null` sans commit évalué), `excluded` (`merge`, `bot`, `initial_commit`, `excluded_account`), `issues` (chacun `code`, `severity` : `blocking` ou `warning`, `count`, `shas`)), `before_since` (commits antérieurs à la date de début), `parents_unknown` (commits au nombre de parents inconnu), `default_branch` (`name`, `evaluated`, `conform`, `rate`, vue informative) ; chaque contributeur : `author` (`name`, `login`, `display_name`), `commits` (mêmes clés que celles d'un dépôt) ; `commits` (`[]` sans `--commits` ni `--failing`) : chacun `repository`, `sha`, `short_sha`, `author`, `date`, `conform`, `codes`, jamais le message ; `notice` : rappel traduit ; `filters` : `repository`, `contributor`, `commits`, `failing`, `milestone` (`""` sans `--milestone`). Depuis 1.8.0, chaque dépôt a aussi `traits` (`evaluated`, `wording` (`past_tense`, `progressive`, `first_person`), `footer`, `issue_reference`), `language` (`expected`, `fr`, `en`, `unknown`, `other`, `mixed`), `branches` (`source`, `checked`, `conform`, `review`, `items` : chacun `name`, `codes` (BR1 ; bloquants `unknown_type`, `uppercase`, `underscore`, `space`, `accent`, `no_slash`, `pattern` ; avertissement `single_segment`), `conform`, `review`, `in_source` (faux : connue seulement comme branche de tête d'une pull request), `pull_requests` (numéros), `authors`), `pull_requests` (`known`, `reason` (`no_api` ou `null`), `open` et `merged` (`evaluated`, `conform`, `default`), `skipped` (`closed`, `draft`, `bot`, `before_since`), `items` : chacun `number`, `category` (`open` ou `merged`), `author`, `head_branch`, `codes` (codes CC2 de l'en-tête), `default_codes` (PR2 : `forbidden`, `github_default`, `head_branch`), `conform`, jamais le titre), `tags` (`checked`, `conform`, `items` (`name`, `codes` (SV1 ; bloquants `no_v`, `upper_v`, `missing_component`, `leading_zero`, `empty_prerelease`, `invalid` ; avertissement `build_metadata`), `conform`, `final` (conforme, sans pré-version ni métadonnées de build : seul un tag final borne un intervalle)), `milestones` (`name`, `due`, `status` : `present`, `nonconforming_name` ou `absent`, `tag`, `others`), `releases` (`known`, `reason` (`no_api` ou `null`), `notes` : chacune `code` (SV6 : `milestone_without_release`, `release_without_tag`, `prerelease_mismatch`), `tag`)), `intervals` (`known`, `reason` (`no_clone`, `clone_unreadable`, `filtered`, `no_final_tag` ou `null`), `last_tag`, `items` : chacun `from` (`null` = depuis le début), `to`, `known`, `commits`, `before_since`, `missing` ; `since_last_tag`) ; chaque contributeur `traits`, `trend` (`known`, `repositories`, `unmeasured`, `before`, `since`, taux comme `commits`), `pull_requests` (`open`, `merged`), `branches` (`checked`, `conform`, `review`) ; `milestone` : `null` sans `--milestone`, sinon `name`, `due`, `criteria` (`CC1`, `BR1`, `SV1`, `SV2`, puis depuis 1.9.0 `CL9`, `CL8`, `RD1`, `RD2` (section installation seulement), `RD3`, `DOC1`, `DOC3`) et `repositories` : chacun `repository`, `evaluable`, `reason` (`no_since` ou `null`), `since`, `status`, `tag`, `tag_codes`, `interval` (`from`, `to`, `due`, `commits`, `before_since`, `missing` ; `null` si non mesuré), `branches`, `files` (1.9.0, `null` si non évaluable : `state` (`computed` ou `unknown`), `reason` (`no_tag`, `not_due`, `tag_absent`, `not_computed`, `stale`, `no_clone` ou `null`), `sha`, `computed_at`, `readme_expected` et `readme_found`, clés des sections du README, en information), `criteria` (chacun `id`, `status` : `conform`, `nonconform` ou `unknown`, `reason` d'un critère inconnu : `no_tag`, `not_due`, `tag_absent`, `no_clone`, `clone_unreadable`, `no_commits`, `partial`, `no_branch`, et pour les critères de fichiers `not_computed`, `stale`, `no_file`, `empty_repository`, `tree_truncated`, `blob_too_large`, `blob_unreadable`, `no_docs_dir`, `unread_documents`, `null` sinon ; voir [Conventions de projet](13-conventions.md#71-revue-de-jalon-rj)), `conform`, `unknown`, `total`. Depuis 1.9.0 (phase 8.3), chaque dépôt a aussi `changelog` (CL1 à CL8), `readme` (RD1 à RD4), `docs` (DOC1 à DOC5, DOC8) et `tooling` (TL1 à TL3), lus dans le cache de la synchro : `state` (`computed` ou `unknown`), `reason` (`not_computed`, `stale`, `no_clone`, `filtered` ou `null`), `sha` et `computed_at` (révision mesurée, date du calcul ; `null` si inconnus), `conform`, `unknown`, `total`, `criteria` (chacun `id`, `status`, `reason` (raison de la famille, ou `empty_repository`, `tree_truncated`, `no_file`, `blob_too_large`, `blob_unreadable`, `no_docs_dir`, `unread_documents`, `null` sinon), `codes` (bloquants : `absent`, `misnamed`, `elsewhere`, `no_h1`, `h1_title`, `no_keepachangelog`, `no_semver`, `unreleased_absent`, `unreleased_misplaced`, `unreleased_duplicate`, `unreleased_no_brackets`, `no_brackets`, `date_fr`, `no_date`, `invalid_date`, `future_date`, `invalid_version`, `version_order`, `date_order`, `unknown_category`, `duplicate_category`, `bullet_outside_category`, `empty_version`, `empty_category`, `empty_bullet`, `tag_without_section`, `section_without_tag`, `version_missing`, `missing_section`, `index_link_absent`, `index_link_dead`, `index_link_folder`, `changelog_link_absent`, `changelog_link_dead`, `too_short`, `no_table`, `missing_column`, `no_rows`, `unreferenced`, `duplicate_row`, `dead_link`, `case_mismatch`, `dir_name`, `file_name`, `archive`, `not_found` ; avertissements : `french_label`, `v_prefix`, `category_order`, `office_document`, `husky_no_prepare`, `translation_only`), `counts` (compteurs seulement : `versions`, `sections`, `tags`, `expected`, `found`, `expected_mask`, `found_mask`, `license_file`, `translations`, `words`, `min_words`, `tables`, `rows`, `columns`, `documents`, `links`, `sources`, `unread`, `dirs`, `files`, `configs`, `extends_conventional`, `tools_mask`, `templates`, et un compteur par code), jamais un chemin, un contenu de fichier ni une cellule de l'INDEX ; codes et compteurs par critère, ordre des bits des masques : voir [Conventions de projet](13-conventions.md#58-outillage-tl)) ; `docs` ajoute `lists` (`computed`, `reason` (`not_computed`, `stale`, `no_clone`, `clone_unreadable`, `tree_truncated` ou `null`), `unreferenced`, `duplicate_rows`, `dead_links` (chacun `source`, `target`, `resolved`, `case_mismatch`), `bad_names`, `archives`, `office_documents`) : remplies seulement dans la vue détaillée d'un dépôt (`watchoo conventions owner/repo`), listes vides sinon. Depuis 1.10.0 (phase 8.4), les éléments de `tags.milestones` ont aussi `deadline` (`null` sans échéance jugée, sinon `status` : `on_time`, `late` ou `undetermined`, `due_end` (fin de l'échéance, exclue), `first_seen_at` (première observation du tag, `null` s'il n'a jamais été observé), `commit_at` (date du commit visé à la première observation du tag, `null` si inconnue ou si le tag a été réécrit depuis), `set_after` (borne basse de la pose : la plus tardive entre `commit_at` et la dernière lecture complète des tags qui ne l'a pas vu, avant sa première observation ; `null` si inconnue)) ; chaque dépôt a aussi `rewrites` (BR4 : `known` (toujours `true`), `reason` (toujours `null`, gardée pour la symétrie), `count`, `last`, `events` : chacun `branch`, `old_sha`, `new_sha`, `observed_at`, du plus récent au plus ancien, jamais d'auteur), `tag_history` (SV3 : `known`, `reason` (`not_observed` ou `null`), `since` (première observation), `counts` (`stable`, `restored`, `rewritten`, `deleted`), `items` : chacun `name`, `status`, `sha` (`null` si supprimé), `first_seen_at`, `rewrites`, `deletions`), `prereleases` (SV4, avertissements : `known`, `reason` (`filtered` ou `null`), `checked`, `conform`, `notes` : chacune `code` (`free_form`, `after_final`, `order`), `tag`, `other`), `bumps` (SV5 : `known`, `reason` (`no_clone`, `filtered` ou `null`), `counts` (`consistent`, `under`, `undetermined`, `unknown`), `items` : chacun `from`, `to`, `status`, `reason` (`no_base`, `clone_unreadable`, `partial`, `low_conform` ou `null`), `actual` et `expected` (`patch`, `minor`, `major` ou `null`), `evaluated`, `conform`, `breaking`, `features`, `missing`), `precedence` (SV7 : `known`, `reason` (`no_clone`, `filtered` ou `null`), `pairs`, `unknown`, `conform`, `notes` : chacune `code` (`precedence_ancestry`, `increment_skip` en information), `tag`, `other`) et `changelog_freshness` (CL10, information : `known`, `reason` (`no_clone`, `no_file`, `clone_unreadable`, `filtered` ou `null`), `since` (SHA du dernier commit qui a touché le CHANGELOG), `feat`, `fix`, `missing`, `grayed`) ; `milestone.criteria` reçoit `SV3` après `SV2`, et chaque dépôt de la revue reçoit `deadline` (mêmes clés) et `tag_history` (`status`, `first_seen_at`, `rewrites`, `deletions` ; `null` si le tag est absent ou jamais observé), avec les raisons `deadline_undetermined` et `not_observed`. Depuis 1.11.0 (phase 8.5), `pull_requests.skipped` a `fork`, `pull_requests.open` et `pull_requests.merged` de chaque dépôt et de chaque contributeur ont `body_known`, `body_conform`, `merge_known`, `squash`, `merge_conform`, `pull_requests` a `methods` (`squash`, `merge`, `rebase`, `unknown` : pull requests fusionnées par méthode), chaque pull request de `items` a `body` (PR5 : `known`, `found` et `missing` (clés `context`, `changes`, `testing`, `screenshots`), `closes`, `conform` ; jamais la description) et `merge` (PR6 : `method` (`squash`, `merge`, `rebase` ou `unknown`), `message_conform`, `conform` ; `null` pour une pull request ouverte) ; `branches` a `reused` (BR2 : `known`, `reason` (`no_api` ou `null`), `items` : chacune `name`, `pull_requests`) ; chaque dépôt a `arrivals` (PR3 : `known`, `reason` (`no_clone`, `clone_unreadable` ou `null`), `mode` (`api` ou `clone`), `complete` (liste des pull requests relue en entier), `by_pr`, `outside_pr`, `undetermined`, `reasons` (`merge_commit_sha`, `rebase_run`, `github_merge`, `squash_suffix`, `human_committer`, `local_merge`, `web_edit`, `not_in_db`, `committer_unknown`, `unmatched`, `clone_only`), `outside` : chacun `sha`, `short_sha`, `reason`, `author` (auteur du commit)) et `merge_methods` (BR5 : `known`, `reason` (`no_api`, `no_push_access` ou `null`), `squash`, `merge`, `rebase` (`null` si inconnus), `conform`) ; chaque contributeur a `arrivals` (`by_pr`, `outside_pr`, `undetermined`) ; `milestone.criteria` reçoit `PR3` après `BR1`, et chaque dépôt de la revue reçoit `arrivals` (mêmes clés ; `null` si l'intervalle n'est pas évaluable), avec la raison `undetermined`. Depuis 1.12.0 (phase 8.6), `pull_requests.open` et `pull_requests.merged` de chaque dépôt et de chaque contributeur ont `review_known` (pull requests dont les relectures sont lues ; fusionnées : après la fusion), `reviewed` (fusionnées : PR4 conforme ; ouvertes : approuvées à la dernière lecture, en information) et `reviews_received` (relectures déposées par d'autres comptes, tous états sauf `PENDING`), chaque pull request de `items` a `review` (PR4 : `known`, `reviews`, `approvals` (relecteurs approuvant retenus : contributeurs seulement avec `conventions.pr.reviewer_must_contribute`), `reviewed` ; des compteurs seulement, jamais un relecteur), `pull_requests` a `merged_without_review` (chacune `number`, `author`, numéro décroissant, jamais le titre) ; `milestone.criteria` reçoit `PR4` après `PR3`, et chaque dépôt de la revue reçoit `reviews` (`merged`, `known`, `reviewed`, `unreviewed` (numéros) ; `null` si non évaluable), avec les raisons `reviews_unchecked`, `no_merged_pr` et `no_tag_date` |
| `watchoo update` | `current`, `latest`, `update_available`, `release_url` | Seulement avec `--check` (voir [Mise à jour](#mise-à-jour--watchoo-update)) ; `watchoo update --json` sans `--check` est refusé |
| `watchoo config export` | `format`, `version`, `exported_at`, `watchoo_version`, `workspace`, `settings`, `repositories` | Le document de partage lui-même (voir [Partage de la configuration](#partage-de-la-configuration--watchoo-config)), avec ou sans `--json` ; avec `-o`, il va dans le fichier et la sortie standard ne reçoit qu'une ligne de confirmation en texte (rien avec `--json`) |
| `watchoo config import` | `file`, `dry_run`, `applied`, `source`, `settings`, `repositories`, `summary`, `warnings` | `source` : `workspace_name`, `watchoo_version`, `exported_at` (tels qu'écrits dans le fichier) ; `settings` : `changes` (chacun `key`, `before`, `after`, `env_var`), `rewrite`, `written`, `backup_file` ; chaque dépôt : `index`, `repo`, `action` (`add`, `update`, `unchanged`, `inaccessible`, `error`), `reenable`, `before` (`label`, `branches`, `flemme_after_days`, `dead_after_days` (`null` = seuil de l'espace), `conventions_since` (1.7.0, `"AAAA-MM-JJ"`, `null` = date de l'espace), `enabled` ; `null` pour un ajout), `after`, `outcome` (comme `repo import`, `""` tant que rien n'est appliqué), `error` ; `summary` : `added`, `already_tracked`, `updated`, `reenabled` (parmi `updated`), `inaccessible`, `errors` (prévus avec `--dry-run`, obtenus sinon) ; chaque avertissement : `code` (`env_override`, `comments_lost`), `key`, `message` |

Objets partagés :

- **dépôt** : `id`, `full_name`, `owner`, `name`, `label`, `display_name`, `url`, `default_branch`,
  `branch_patterns` (`[]` = toutes les branches), `flemme_after_days` et `dead_after_days` (seuils
  propres au dépôt, `null` = seuil de l'espace ; les seuils appliqués sont dans l'objet activité),
  `conventions_since` (1.7.0 : date de début des conventions propre au dépôt, `"AAAA-MM-JJ"`, `null` =
  date de l'espace), `enabled`, `archived`, `stars`, `open_issues`, `last_synced_at`, `last_commit_at` ;
- **commit** : `repository` (`id`, `full_name`, `display_name`), `sha`, `short_sha`, `message`, `url`,
  `author` (`name`, `login`, `display_name`), `authored_at`, `committed_at`, `additions`, `deletions`,
  `stats_known` (0.5.0 : `true` si `additions` et `deletions` ont été calculées, même à 0 ; `false` si
  elles sont encore inconnues, `additions` et `deletions` valant alors 0), `ai_signatures` (1.2.0 : `tool` et
  `kind` de chaque signature, `[]` si aucune) et `ai_scanned` (`false` : pas encore analysé, `ai_signatures`
  ne dit encore rien) ;
- **activité** : `state` (`active`, `flemme`, `dead`, `stopped`, `error`, `never_synced`, `archived`,
  `empty`), `state_label`, `gray`, `days_since_last_commit` (`null` sans commit), `flemme_after_days`,
  `dead_after_days` ;
- **synchro** (`last_sync`) : `status` (`running`, `success`, `error`), `started_at`, `finished_at`,
  `new_commits`, `error` ;
- **quota** (`rate_limit`, `null` si inconnu) : `limit`, `remaining`, `used`, `reset`.
- **pull request** : `repository` (`id`, `full_name`, `display_name`), `number`, `title`, `state`
  (`open`, `merged` ou `closed` : fermée sans merge), `draft`, `author` (`login`, `""` si inconnu),
  `url`, `base_branch`, `head_branch`, `created_at`, `updated_at`, `closed_at`, `merged_at` (`null` si
  sans objet) ;
- **release** : `repository`, `id` (identifiant sur la forge), `tag_name`, `name`, `draft`,
  `prerelease`, `author` (`login`), `url`, `created_at`, `published_at` (`null` pour un brouillon) ;
- **tag** : `repository`, `name`, `sha`, `short_sha`, `commit_url` et `committed_at` (lien et date du
  commit visé, `""` et `null` s'il n'est pas en base).

### Version

`watchoo version --json` (ou `watchoo --version --json`) :

```json
{
  "version": "v0.1.0",
  "commit": "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
  "date": "2026-10-27T10:00:00Z",
  "go": "go1.27.3",
  "os": "linux",
  "arch": "amd64"
}
```

- `version`, `commit` (SHA complet) et `date` (date **du commit** compilé, pour des compilations
  reproductibles) sont injectés à la compilation par `-ldflags` sur `internal/cli.Version`,
  `internal/cli.Commit` et `internal/cli.Date` (Makefile, GoReleaser et Dockerfile, mêmes options ; voir
  [Distribution](03-architecture.md#distribution)).
- Sans ces options, ils viennent des informations de compilation de Go quand elles existent :
  `go install github.com/studoo-app/watchoo/cmd/watchoo@v0.1.0` donne la version du module ; un
  `go build` dans un dépôt Git donne le commit, sa date et la version du module (`v0.1.0` sur un tag,
  une pseudo-version `v0.0.0-…` ailleurs, `dev` si l'arbre a des modifications non commitées). Sinon
  `version` vaut `dev`, `commit` `""` et `date` `null`.
- En texte : `watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, linux/amd64)` ; commit et date sont
  omis s'ils sont inconnus. La carte d'en-tête, le TUI, l'interface web et le `User-Agent` des appels
  GitHub montrent la même version.

## Carte d'en-tête

Sur un terminal, l'aide générale, `watchoo version` et le compte rendu de `watchoo init` commencent par une
**carte d'en-tête** : un cadre aux coins arrondis avec, à gauche, le hibou en petit format
(`ascii-mini.txt`, voir [Logo et icônes](06-interface-web.md#versions-texte-cli-et-tui)) et, à sa droite,
« Watchoo », la version et un sous-titre ; dessous, parfois, une astuce. Des lignes d'état précédées
d'un `●` peuvent suivre la carte. Exemple, `watchoo` lancé dans un espace :

```
╭───────────────────────────────────────────────────────────────────╮
│  █▄    ▄█  Watchoo v0.1.0                                         │
│  █ ●██● █  Qui a travaillé, sur quoi, et quand ?                  │
│  ▀██▙▟██▀                                                         │
│                                                                   │
│  Astuce : « watchoo repo add owner/repo » puis « watchoo sync ».  │
╰───────────────────────────────────────────────────────────────────╯

● Espace : licence-projet (~/supervision/licence-projet)

Watchoo supervise des dépôts GitHub, surtout privés, et répond à trois questions :
…
```

| Sortie | Sous-titre et astuce de la carte | Après la carte |
| --- | --- | --- |
| Aide générale : `watchoo`, `watchoo help`, `watchoo --help` / `-h` | « Qui a travaillé, sur quoi, et quand ? » ; astuce « `watchoo init` fait de ce répertoire un espace de supervision » si aucun espace n'est trouvé, « `watchoo repo add owner/repo` puis `watchoo sync` » dans un espace, « `watchoo help <commande>` détaille une commande » si l'espace est illisible | Une ligne vide, la ligne d'état de l'espace, une ligne vide, puis l'aide habituelle |
| `watchoo version`, `watchoo --version` | Version de Go et plateforme (`go1.27.3 · darwin/arm64`) ; pas d'astuce | Rien |
| `watchoo init` réussi | « Espace de supervision « nom » créé » ; astuce « `watchoo repo add owner/repo` ajoute un premier dépôt à suivre » | Une ligne vide, puis le compte rendu en lignes d'état : espace, réglages, base, compte `gh` et ses droits (sans `gh` : « Source : clones Git locaux ») |
| `watchoo status` | « État de l'espace « nom » » ; astuce « `watchoo sync` met à jour les dépôts suivis » | Une ligne vide, puis le résumé habituel |

- **Scripts** : la carte n'est affichée que si la sortie standard est un terminal, et jamais avec `--json`.
  Une sortie redirigée (fichier, tube) reste identique : aide sans carte, `watchoo version` sur une seule
  ligne (`watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, darwin/arm64)`), compte rendu habituel de `init`. La sortie d'erreur
  (erreurs, avertissements) n'a jamais de carte, l'aide des sous-commandes (`watchoo help init`) non plus.
- **Ligne d'état de l'espace** (aide générale) : `● Espace : <nom> (<racine>)`, avec `~` pour le
  répertoire personnel, ou `● Aucun espace de supervision ici`. L'espace est cherché comme le feraient les
  autres commandes (répertoire courant et ses parents, `-C`, `WATCHOO_DIR`), localement, sans lancer
  `gh`. La recherche ne fait jamais échouer l'aide : une erreur (réglages invalides, répertoire de `-C`
  inexistant…) est résumée sur la ligne d'état, `● Espace : <message>`.
- **Largeur** : celle du contenu, sans dépasser 80 colonnes ni la largeur du terminal (variable `COLUMNS`
  si c'est un entier positif, sinon celle que donne le système). Un texte trop long est coupé sur les
  espaces. Sur un terminal de moins de 40 colonnes, pas de carte : la sortie est celle d'une sortie
  redirigée. Une lettre accentuée occupe une colonne, qu'elle soit précomposée (`é`) ou décomposée (`e`
  suivi d'un accent combinant, forme fréquente dans les noms de répertoires créés sous macOS, donc dans
  le nom par défaut d'un espace).
- **Unicode** : si la locale est en UTF-8 (première variable non vide parmi `LC_ALL`, `LC_CTYPE` et
  `LANG`) ou dans Windows Terminal (`WT_SESSION`). Sinon, la carte et les lignes d'état sont en ASCII pur :
  bordures `+`, `-` et `|`, hibou `ascii-mini-pur.txt`, guillemets droits au lieu de « », accents
  retirés, puce `*`.
- **Couleurs** : bordure, version, sous-titre et lignes d'état en gris, « Watchoo » en gras, pupilles du
  hibou en vert, commande citée dans l'astuce en cyan. Pas de couleur si `NO_COLOR` est définie et non
  vide, si `TERM=dumb`, ni sous Windows dans une console qui n'accepte pas les séquences ANSI.
- **Version** : un numéro reçoit le préfixe `v` (`0.1.0` devient `v0.1.0`) ; `v0.1.0` ou `dev` (binaire
  compilé sans version) sont affichés tels quels.

## Mise à jour : `watchoo update`

Depuis la 0.6.0, Watchoo signale ses nouvelles versions et se met à jour lui-même, à partir des releases
publiées sur GitHub (pages publiques, sans token ni API GitHub : voir
[Architecture](03-architecture.md#mise-à-jour-de-watchoo-060)). Ni espace ni `gh` ne sont nécessaires.

### Avis de nouvelle version

À **chaque lancement** d'une commande, Watchoo vérifie en arrière-plan si une version plus récente que la
sienne est publiée. À la fin de la commande, il écrit sur la **sortie d'erreur** :

```
Nouvelle version de Watchoo disponible : v0.5.0 → v0.6.0. Mettez-la à jour avec « watchoo update ».
```

- La vérification part dès le début de la commande et dispose de **1,5 s** au total : une commande n'est
  jamais retardée de plus, et l'avis n'est pas écrit si la réponse n'est pas arrivée à temps. Une
  commande plus longue que ce délai (`sync`, par exemple) n'attend pas : la réponse est déjà là, et l'avis
  est toujours écrit. Sans réseau, ou en cas d'erreur, rien n'est affiché (log de niveau debug avec
  `--verbose`).
- `watchoo` seul (la carte d'en-tête et l'aide générale) a aussi son avis, écrit après l'aide, comme
  `watchoo repo` ou `watchoo config` seuls ; pas `watchoo --help` ni `watchoo help …`.
- Pas de vérification avec `--json`, quand la sortie d'erreur n'est pas un terminal (scripts, CI, tube),
  pour une version qui n'est pas une version de release (`dev`, pseudo-version de `go install` ou
  `go build`, pré-version), pour `watchoo update` lui-même, `watchoo serve` et `watchoo tui` (qui ont la
  leur) et l'aide.
- **`WATCHOO_NO_UPDATE_CHECK`** : posée à une valeur non vide autre que `0` ou `false`, elle coupe toute
  vérification (commandes, `serve` et `tui`). L'image Docker la pose (`WATCHOO_NO_UPDATE_CHECK=1`) : elle se met
  à jour en changeant de tag.
- Une pré-version n'est jamais proposée ; une version installée plus récente que la dernière publiée ne
  donne aucun avis.
- **`watchoo serve`** vérifie au démarrage puis toutes les 30 min tant qu'il tourne ; une nouvelle version
  est annoncée une fois sur la sortie d'erreur (même phrase) et l'interface web montre un bandeau (voir
  [Interface web](06-interface-web.md#bandeau-de-nouvelle-version-060)).
- **`watchoo tui`** vérifie une fois, en arrière-plan, à l'ouverture (10 s au plus, sans retarder
  l'affichage) ; si une version plus récente est publiée, le mode interactif l'annonce sur la ligne sous
  son en-tête, tant qu'il reste ouvert :
  « Nouvelle version de Watchoo disponible : v0.6.2 → v0.7.0. Quittez puis lancez « watchoo update ». »
  (dans l'image Docker : « Changez le tag de l'image Docker pour la mettre à jour. ») ; sur un terminal
  trop étroit, une forme courte : « Watchoo v0.7.0 disponible : quittez puis lancez « watchoo update ». ».
  Échec ou délai dépassé : rien. Pas d'avis de plus sur la sortie d'erreur en quittant. Mêmes exceptions
  que les commandes : `WATCHOO_NO_UPDATE_CHECK`, version qui n'est pas une version de release.

### `watchoo update --check`

Vérifie seulement, sans rien installer : version installée, dernière version publiée et, s'il y en a une
plus récente, le lien vers sa release. Code de retour 0 qu'une mise à jour existe ou non ; non nul si la
dernière version n'a pas pu être lue (réseau). Avec une version de développement, la comparaison est
impossible : la dernière version est affichée, `update_available` vaut `false`.

```
$ watchoo update --check
Version installée : v0.5.0
Dernière version : v0.6.0
Nouvelle version disponible : https://github.com/studoo-app/watchoo/releases/tag/v0.6.0
Mettez-la à jour avec « watchoo update ».
```

`watchoo update --check --json` :

```json
{
  "current": "v0.5.0",
  "latest": "v0.6.0",
  "update_available": true,
  "release_url": "https://github.com/studoo-app/watchoo/releases/tag/v0.6.0"
}
```

### `watchoo update`

1. Refuse tout de suite, avec un code non nul et un message qui dit quoi faire :
   - dans l'**image Docker** (variable `WATCHOO_DOCKER` ou fichier `/.dockerenv`) : changer le tag de
     l'image, ou `docker compose pull && docker compose up -d` ;
   - pour une **version de développement** (`dev`, pseudo-version, pré-version : binaire compilé depuis
     les sources) : `go install github.com/studoo-app/watchoo/cmd/watchoo@latest`, ou
     `git pull && make build` dans le clone ;
   - si le **répertoire du binaire** n'est pas accessible en écriture : droits du répertoire, `sudo`, ou
     réinstallation par `install.sh` avec `WATCHOO_INSTALL_DIR` (voir le README).
2. Lit la dernière version. Déjà à jour : « Watchoo v0.6.0 est à jour (dernière version publiée : v0.6.0). », code 0.
3. Affiche `v0.5.0 → v0.6.0`, le lien vers la release et vers le `CHANGELOG.md` de cette version, puis
   demande confirmation (`[o/N]`) sur la sortie d'erreur. Comme pour `repo remove`, `--yes` / `-y` saute
   la question, et elle est **obligatoire** quand l'entrée standard n'est pas un terminal ; une autre
   réponse que `o`, `oui`, `y` ou `yes` annule avec un code non nul, sans rien modifier.
4. Télécharge l'archive de la release pour le système et l'architecture
   (`watchoo_<version>_<os>_<arch>.tar.gz`, `.zip` sous Windows) et le fichier de sommes, **refuse** une
   archive dont la somme SHA-256 ne correspond pas, extrait le binaire et remplace celui en cours d'un
   bloc (fichier temporaire dans le même répertoire, puis renommage ; sous Windows, l'ancien est d'abord
   renommé en `watchoo.exe.old`).
5. Rappelle qu'un `watchoo serve` déjà lancé garde l'ancienne version jusqu'à son redémarrage, et que la
   base de chaque espace est mise à niveau (migrations) à la prochaine commande.

`--json` n'est accepté qu'avec `--check`.

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
| `flemme` | non | Seuil Flemme propre au dépôt, en jours sans commit, comme `--flemme` (`0` = seuil de l'espace) |
| `dead` | non | Seuil Dead propre au dépôt, comme `--dead` ; plus grand que `flemme` sur la même ligne |

Un simple fichier texte est aussi accepté : un dépôt par ligne, sans en-tête, lignes vides et lignes
commençant par `#` ignorées.

### Comportement

1. Lit tout le fichier et le **valide** avant d'ajouter quoi que ce soit : une ligne mal formée arrête
   l'import en indiquant le numéro de ligne. Un dépôt présent deux fois dans le fichier (`owner/repo` ou
   URL, sans tenir compte de la casse) est une ligne en erreur : rien n'est importé, le message donne la
   ligne du doublon et celle de la première occurrence.
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

- Une cellule `label`, `branches`, `flemme` ou `dead` **vide** veut dire « non fourni » : la valeur
  existante est gardée (un import ne peut donc pas remettre un dépôt à « toutes les branches » ; utiliser
  `repo add owner/repo --branch ""`). Pour revenir au seuil de l'espace, écrire `0` dans `flemme` ou
  `dead`. Un seuil illisible, négatif, au-delà de 3 650 jours, ou un `dead` inférieur ou égal au
  `flemme` de la même ligne est une ligne en erreur.
- `-` à la place du fichier lit la liste sur l'entrée standard.
- Le bilan détaille ensuite chaque dépôt inaccessible ou en erreur, avec son numéro de ligne.
- `--dry-run` : fait la validation et les vérifications d'accès, affiche le bilan (« à ajouter » au lieu
  d'« ajoutés »), mais n'enregistre rien.
- `--json` : bilan au format JSON.
- Code de retour non nul s'il reste des dépôts inaccessibles ou en erreur, pour les scripts.
- L'import ne lance pas de synchro : on enchaîne avec `watchoo sync`.

## Partage de la configuration : `watchoo config`

Plusieurs personnes supervisent souvent le même groupe (enseignants d'une même promotion), chacune avec
son propre espace sur sa machine. `watchoo config export` écrit la configuration **partageable** de
l'espace dans un fichier JSON ; `watchoo config import` la fusionne dans un autre espace. On partage
ainsi la liste des dépôts, leurs noms affichés et leurs branches, et les réglages communs, sans jamais
partager de données d'étudiants ni de réglage propre à une machine.

### Contenu du fichier

```json
{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-27T10:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {
    "name": "licence-projet"
  },
  "settings": {
    "sync": {
      "auto": true,
      "interval": "30m",
      "parallelism": 4
    },
    "activity": {
      "flemme_after_days": 5,
      "dead_after_days": 15
    },
    "branches": {
      "exclude": []
    },
    "timezone": "",
    "lang": "en",
    "conventions": {
      "since": "2026-09-01",
      "commit": {
        "types": ["feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"],
        "scopes": [],
        "max_subject_soft": 50,
        "max_subject_hard": 72,
        "forbidden": ["update", "maj", "wip", "test", "modif", "fix", "dépôt 1", "."],
        "language": "any"
      },
      "branch": {
        "pattern": "",
        "exclude": ["gh-pages", "dependabot/*", "renovate/*"]
      },
      "pr": {
        "body_sections": ["context", "changes", "testing", "screenshots"],
        "require_closes": true,
        "min_reviews": 1,
        "reviewer_must_contribute": false
      },
      "tags": {
        "milestones": ["v1.0.0", "v2.0.0", "v3.0.0"],
        "prerelease_forms": ["rc", "beta", "alpha"]
      },
      "changelog": {
        "categories_fr": true
      },
      "readme": {
        "sections": ["presentation", "group", "stack", "install", "run", "env", "tree", "license"],
        "min_words": 150
      },
      "docs": {
        "dir": "documentation",
        "media_dir": "Media",
        "archives_scope": "repository"
      }
    }
  },
  "repositories": [
    {
      "provider": "github",
      "repo": "alice/projet-licence",
      "label": "Alice Martin",
      "branches": ["main", "release/*"]
    }
  ]
}
```

| Clé | Contenu |
| --- | --- |
| `format` | Toujours `watchoo-config` (obligatoire) |
| `version` | Version du format, `1` (obligatoire). Une version plus grande est refusée : « fichier créé par une version plus récente de Watchoo » |
| `exported_at`, `watchoo_version` | Date d'export (UTC, RFC 3339) et version de Watchoo qui l'a fait ; informatifs, facultatifs |
| `workspace.name` | Nom de l'espace exporté ; informatif : l'import ne renomme jamais l'espace |
| `settings` | Réglages communs, valeurs **en vigueur** dans l'espace exporté (fichiers et variables `WATCHOO_*` compris) : `sync.auto`, `sync.interval` (durée Go entre guillemets : `"30m"`, `"1h30m"`, `"0"`), `sync.parallelism`, `activity.flemme_after_days`, `activity.dead_after_days`, `branches.exclude`, `timezone`, `lang` (`"en"` ou `"fr"` ; clé facultative de la version 1 : un fichier sans elle reste valide), `conventions` (charte des [conventions de projet](13-conventions.md#8-configuration) : `since`, `"AAAA-MM-JJ"` ou `""`, et `commit` : `types`, `scopes`, `max_subject_soft`, `max_subject_hard`, `forbidden`, `language` ; clé facultative de la version 1, ajoutée en 1.7.0 : un fichier sans elle reste valide, mais un fichier qui la contient est refusé par les versions antérieures, qui n'acceptent aucune clé inconnue ; et, depuis 1.8.0, `branch` : `pattern` (expression RE2, `""` = règle intégrée), `exclude`, et `tags` : `milestones` (`"vX.Y.Z"` ou `"vX.Y.Z@AAAA-MM-JJ"`), clés facultatives refusées par les versions antérieures à 1.8.0 ; et, depuis 1.9.0, `changelog` : `categories_fr` (booléen), `readme` : `sections` (clés parmi `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`, `license` ; `[]` = aucune section attendue), `min_words` (au moins 1), et `docs` : `dir` (dossier relatif à la racine du dépôt, sans `.` ni `..`), `media_dir` (un seul nom de dossier), `archives_scope` (`"repository"` ou `"documentation"`), clés facultatives refusées par les versions antérieures à 1.9.0 ; et, depuis 1.10.0, `tags` : `prerelease_forms` (formes de pré-version conformes, au moins une, mots en minuscules), clé facultative refusée par les versions antérieures à 1.10.0 ; et, depuis 1.11.0, `pr` : `body_sections` (clés parmi `context`, `changes`, `testing`, `screenshots` ; `[]` = aucune section attendue), `require_closes` (booléen), clés facultatives refusées par les versions antérieures à 1.11.0 ; et, depuis 1.12.0, `pr` : `min_reviews` (au moins 1), `reviewer_must_contribute` (booléen), clés facultatives refusées par les versions antérieures à 1.12.0) |
| `repositories` | Dépôts dont le suivi est actif (les dépôts arrêtés ne sont pas exportés), triés par `owner/repo` : `provider` (`github`), `repo` (`owner/repo`), `label` (nom affiché, `""` sans nom), `branches` (motifs suivis, `[]` = toutes les branches), et, **seulement pour un dépôt qui en a**, `flemme_after_days` et `dead_after_days` (seuils propres, en jours) et `conventions_since` (1.7.0 : date de début des conventions propre, `"AAAA-MM-JJ"`). Ces clés sont facultatives dans la version 1 du format : un fichier sans seuil ni date propres reste lisible par les versions de Watchoo qui ne les connaissent pas ; un fichier qui contient `conventions_since` est refusé par les versions antérieures à 1.7.0 |

**Jamais exportés** : les réglages propres à la machine ou à la personne (`name` du fichier, `github.*` :
compte `gh` et chemin de `gh`, qu'un fichier venu d'ailleurs ne doit jamais pouvoir changer ; `git.*` :
protocole des clones Git locaux, qui dépend des identifiants de la machine ;
`web.*` : adresse d'écoute, authentification, reverse proxy, noms acceptés ; `diagnostic.*` ;
`conventions.enabled`, choix propre à l'espace, et `conventions.excluded_accounts`, qui contient des logins), le token, les commits, les
contributeurs, les e-mails, les dates de synchro et tout chemin local. Le fichier ne contient donc que
des noms de dépôts, des noms affichés et des réglages ; il peut quand même révéler la liste des
étudiants suivis : le partager seulement avec les personnes concernées.

### `watchoo config export`

- Sans `-o`, écrit le document sur la sortie standard (`watchoo config export > licence.json`), sans
  carte d'en-tête ni astuce, avec ou sans `--json`.
- `-o fichier` (`--output`) crée le fichier en `0600` et affiche une ligne de confirmation (dépôts et
  réglages exportés) ; il **refuse d'écraser** un fichier existant sans `--force`. Avec `--json`, rien
  n'est écrit sur la sortie standard : le fichier est le résultat.
- Ne contacte ni `gh` ni GitHub : tout vient de la base et des réglages de l'espace.

### `watchoo config import fichier`

Le fichier vient d'une autre personne : c'est une **donnée non fiable**, entièrement validée avant de
modifier quoi que ce soit.

1. **Validation** (rien n'est modifié en cas d'erreur, le message dit pourquoi) : JSON valide, un seul
   objet, 1 Mio au plus ; `format` présent et égal à `watchoo-config` ; `version` égale à 1 ; aucune clé
   inconnue, à aucun niveau (une clé `github` ou `web` est donc refusée) ; 1 000 dépôts au plus ; chaque
   dépôt : `provider` absent ou `github`, `repo` de la forme `owner/repo` (ou URL GitHub), présent une
   seule fois (sans tenir compte de la casse), `label` valide (comme `repo add --label`), motifs de
   branches valides (comme `--branch`, sans virgule), seuils propres valides (comme `--flemme` et
   `--dead`), date de début des conventions valide (comme `--conventions-since` : `"AAAA-MM-JJ"` ou
   `""`) ; réglages valides une fois fusionnés avec ceux de
   l'espace (mêmes règles que `config.yaml`, ex. `dead_after_days` plus grand que `flemme_after_days`),
   y compris une fois les variables `WATCHOO_*` définies réappliquées : l'aperçu refuse donc tout ce que
   l'écriture refuserait (ex. `WATCHOO_ACTIVITY_DEAD_AFTER_DAYS=10` et un import de
   `flemme_after_days: 12`).
   `settings` et `repositories` sont facultatifs, chaque réglage aussi : un fichier peut ne contenir que
   des dépôts, ou qu'un fuseau.
2. **Aperçu**, toujours affiché avant d'appliquer :
   - dépôts **à ajouter**, **à mettre à jour** (nom affiché, branches, seuils propres et date de début
     des conventions, avant → après, `espace` pour le seuil ou la date de l'espace ; dépôt arrêté **à reprendre**), **inchangés** ;
   - réglages modifiés (clé, valeur actuelle → valeur importée) ; un réglage identique n'est pas listé.
     Un réglage masqué par une variable `WATCHOO_*` définie est comparé à sa valeur **dans les
     fichiers** (fichier utilisateur puis `config.yaml`), pas à celle de la variable : c'est elle que
     l'import réécrit, et réimporter le même fichier ne change plus rien ;
   - avertissements : chaque variable `WATCHOO_*` qui masque un réglage importé, qu'il change ou non (la
     valeur est écrite dans `config.yaml`, ou y est déjà, mais la variable l'emporte tant qu'elle est
     définie) ; `config.yaml` réécrit, donc
     **ses commentaires personnels perdus** (copie gardée dans `config.yaml.bak`).
3. **Confirmation** : sur un terminal, `Appliquer ces changements ? [o/N]` (mêmes réponses que
   `repo remove`) ; sinon (script, tube, fichier lu sur l'entrée standard avec `-`) l'option
   **`--yes` / `-y` est obligatoire** : sans elle, la commande s'arrête avant d'afficher quoi que ce
   soit (l'aperçu s'obtient alors avec `--dry-run`). Sans confirmation, rien n'est modifié. Si l'aperçu
   ne montre aucun changement, l'espace l'indique (« Rien à changer ») sans poser de question ni exiger
   `--yes` en sortie texte (en `--json` sans `--dry-run`, `--yes` reste obligatoire).
4. **Application**, dans cet ordre :
   - **réglages** : si au moins un réglage change, `.watchoo/config.yaml` est **réécrit** à partir du
     modèle commenté de `watchoo init`, réglages importés décommentés. Les clés propres à la machine
     déjà présentes dans le fichier (`name`, `github.*`, `git.*`, `web.*`, `diagnostic.*`,
     `conventions.enabled`, `conventions.excluded_accounts`) et les réglages communs que le fichier
     importé ne donne pas sont **gardés tels quels**. L'ancien fichier est copié dans
     `.watchoo/config.yaml.bak` (`0600`, remplacé à chaque import). Le nouveau fichier est écrit à côté
     (`0600`), relu et validé, et comparé aux valeurs attendues avant de remplacer l'ancien (écriture
     atomique) : en cas d'écart, rien n'est changé. Si aucun réglage ne change, `config.yaml` n'est pas
     touché ;
   - **dépôts** : comme `repo import`. L'accès à chaque dépôt est vérifié auprès de GitHub ; un dépôt
     absent est ajouté ; un dépôt déjà suivi (cherché par son identifiant GitHub puis par
     `owner/repo`, donc renommage compris) reçoit le `label` et les `branches` du fichier, et son suivi
     **reprend s'il était arrêté** (comme `repo add`). Un `label` vide garde le nom affiché existant ;
     `branches` absent (ou `null`) garde les branches existantes, `[]` revient à toutes les branches ;
     `flemme_after_days` ou `dead_after_days` absent (ou `null`) garde le seuil existant, `0` revient
     au seuil de l'espace ; `conventions_since` absent (ou `null`) garde la date existante, `""` revient
     à la date de l'espace.
     Un dépôt inaccessible est signalé sans bloquer les autres. **Aucun dépôt n'est jamais retiré** et
     aucune synchro n'est lancée.
5. **Bilan** : réglages écrits (et copie `.bak`), puis dépôts ajoutés, déjà suivis, mis à jour,
   inaccessibles, en erreur (ligne « dont repris » pour les dépôts arrêtés qui reprennent), avec le
   détail de chaque dépôt en échec (`dépôt n° N  owner/repo : message`). Code de retour non nul s'il reste
   des dépôts inaccessibles ou en erreur (relancer l'import une fois l'accès donné : il est sans risque).

```
IMPORT  licence.json  (espace « licence-projet », exporté le 27/10/2026 avec Watchoo v0.1.0)
Réglages
  sync.interval              30m → 1h
  activity.dead_after_days   15 → 21
Dépôts
  à ajouter         18
  à mettre à jour    1   bob/projet-licence (branches : toutes → main, dev)
  à reprendre        1   carol/projet-licence
  inchangés          3
! config.yaml sera réécrit : ses commentaires personnels seront perdus (copie dans config.yaml.bak)
Appliquer ces changements ? [o/N]
```

- **`--dry-run`** : validation et aperçu seulement, avec en plus la vérification d'accès de chaque dépôt
  (comme `repo import --dry-run`) ; rien n'est écrit, ni la base ni `config.yaml` (octet pour octet).
  L'aperçu sans `--dry-run` ne vérifie pas l'accès (cela se fait à l'application) : un dépôt renommé sur
  GitHub peut y apparaître « à ajouter » et se révéler « déjà suivi ».
- **`--json`** : aperçu (avec `--dry-run`) ou bilan au format JSON (voir [Sortie JSON](#sortie-json)) ;
  sans `--dry-run`, `--yes` reste obligatoire et l'aperçu n'est pas écrit à part.
- `-` à la place du fichier lit le document sur l'entrée standard (`--yes` obligatoire alors, l'entrée
  standard n'étant plus disponible pour la question).
- Les textes venus du fichier (nom de l'espace, labels, messages) sont nettoyés avant d'être affichés,
  comme ceux des dépôts.
- Les réglages réécrits valent pour les commandes lancées ensuite ; un `watchoo serve` ou `watchoo tui`
  déjà ouvert les prend en compte à son prochain lancement.

## Mode interactif (TUI)

`watchoo tui` ouvre un mode interactif dans le terminal (Bubble Tea). On y parcourt les dépôts, les commits
et les contributeurs au clavier, avec filtre et vue détail, et on peut y lancer une synchro ou ajouter un
dépôt à suivre. Voir
[Architecture](03-architecture.md#terminal--mode-interactif-tui).

- Seule la commande `tui` est interactive. Toutes les autres affichent un tableau texte ou du JSON
  (`--json`), puis rendent la main.
- `watchoo tui` lancé hors d'un terminal s'arrête avec un message clair.
- Tant qu'il est ouvert, `watchoo tui` synchronise automatiquement (30 min par défaut, désactivable). Une
  touche lance une synchro manuelle de tous les dépôts, une autre celle du dépôt sélectionné.
- Les couleurs sont désactivées hors terminal et quand `NO_COLOR` est définie.
- `--interval` remplace `sync.interval` le temps de la session : `0` pour la synchro manuelle seule, sinon
  15 min au minimum ; sans l'option, la synchro automatique suit `sync.auto` et `sync.interval`.
- Pendant le TUI, les logs ne sont pas écrits (ils casseraient l'affichage). `watchoo tui --json` est
  refusé, comme un argument en trop ; la commande s'arrête aussi si l'entrée ou la sortie standard n'est
  pas un terminal.
- **Vues** : Dépôts (dépôt, dernier commit, auteur, branches, état), Commits (quand, dépôt, auteur, SHA
  court, `+/-` du commit, première ligne du message ; les 200 derniers) et Contributeurs (nom, login,
  commits, `+/-` totaux, dépôts, dernier commit). Le **détail d'un dépôt** montre l'URL, l'état et ses
  seuils, le dernier commit, les branches, le nombre de commits et leurs lignes `+/-`, la dernière synchro (ou son erreur), étoiles et issues, les pull
  requests ouvertes et la dernière release publiée (V2), les 15
  derniers commits (avec leurs `+/-`) et les 5 premiers contributeurs, avec leurs lignes `+/-` dans le
  dépôt et leurs commits sur les 12 dernières semaines
  (colonne « 12 SEM. ») et une courbe d'un caractère par semaine, la plus récente à droite (« PAR
  SEMAINE » : `▁` à `█`, ou `.` `-` `=` `#` sans Unicode ; `▁` ou `.` = aucun commit).
- **Détail d'un contributeur** (0.5.0, `entrée` dans la vue Contributeurs ; `échap` revient à la
  liste) : nom, login (le TUI n'affiche pas d'adresse, ni celle du profil), nombre d'identités Git
  rattachées (jamais les adresses) ; totaux (commits, lignes `+/-` ou `-` si inconnues, dépôts, premier
  et dernier commit) ; activité sur 12 semaines (total et courbe d'un caractère par semaine, comme
  ci-dessus) ; ses dépôts (commits, `+/-`, courbe par semaine, dernier commit, état du dépôt) ; ses 15
  derniers commits (quand, dépôt, SHA court, `+/-`, première ligne du message). Les lignes ajoutées et
  supprimées sont lues dans le clone Git local de chaque dépôt ; `-` signale des statistiques encore
  inconnues (clone indisponible), `+0/-0` un commit réellement sans ligne (rappel en bas du détail). Le détail défile avec `↑` `↓`. **Choix** : la table des principaux contributeurs du
  détail d'un dépôt n'est pas sélectionnable (le détail défile, il n'a pas de curseur) ; on passe par la
  vue Contributeurs (filtre `/`).
- **En-tête** : hibou mini (pupilles vertes), version et espace, onglets, ligne de synchro (en cours avec
  la progression, dernière synchro, prochaine synchro automatique à HH:MM, ou « Synchro manuelle (s) »).
  Quand une nouvelle version de Watchoo est publiée, la ligne sous l'en-tête l'annonce (voir
  [Avis de nouvelle version](#avis-de-nouvelle-version)).
- **Ajout d'un dépôt** (`a`, vue Dépôts) : une ligne de saisie « Dépôt à suivre (owner/nom ou URL
  GitHub) : » s'ouvre en bas de l'écran ; `entrée` valide, `échap` annule. La saisie accepte les mêmes
  références que `watchoo repo add` (`owner/repo` ou URL GitHub, mêmes règles et mêmes messages) ; une
  référence invalide est signalée sans rien appeler et la saisie reste ouverte pour la corriger (une
  saisie contenant `@`, qui peut porter des identifiants, est effacée). L'ajout passe par le même cœur
  que `repo add` (accès vérifié auprès de GitHub, ou sans `gh` par lecture du dépôt distant) et suit
  les règles de `repo add` sans option : toutes les branches, sans label ni seuils propres (label,
  branches et seuils se règlent ensuite avec `repo add`, l'import ou la page Réglages du web). Il tourne
  en arrière-plan, un seul à la fois. Bilan en bas de l'écran : « owner/nom ajouté » (ou « déjà suivi »,
  « suivi repris » pour un dépôt arrêté, « inaccessible » avec la raison, comme `repo add`) ; la liste
  est relue, le dépôt sélectionné et sa synchro lancée, comme `S`. Si une synchro est déjà en cours,
  l'ajout se fait quand même et la synchro du nouveau dépôt démarre à la fin de celle en cours (une
  synchro de tous les dépôts lancée entre-temps la remplace). Un dépôt déjà suivi n'est ni modifié ni
  resynchronisé.
- **Synchro** : une seule à la fois (une demande de plus est ignorée et signalée) ; à la fin, une ligne de
  bilan (nouveaux commits, erreurs, dépôts ignorés ou reportés avec l'heure de reprise locale,
  avertissements sur les pull requests, releases, tags ou un clone Git local indisponible), puis les
  vues sont relues. Une synchro automatique qui tombe pendant une synchro est sautée, la suivante reste
  planifiée. Quitter pendant une synchro l'annule : `watchoo tui` attend alors que le moteur ait clos
  ses synchros (« synchro annulée », quelques secondes au plus) avant de fermer la base, pour qu'aucun
  dépôt ne reste verrouillé.

| Touche | Effet |
| --- | --- |
| `tab` / `maj+tab`, `1` `2` `3` | Vue suivante / précédente, vue Dépôts, Commits, Contributeurs |
| `↑` `↓` ou `k` `j`, `pg.préc` `pg.suiv`, `g` `G` | Déplacer la sélection, par page, début et fin de la liste |
| `entrée` | Détail du dépôt sélectionné (vue Dépôts ou Commits) ou du contributeur sélectionné (vue Contributeurs, 0.5.0) |
| `échap` | Retour, ou effacer le filtre |
| `/` | Filtrer la vue (sous-chaîne, sans tenir compte de la casse) ; `entrée` garde le filtre, `ctrl+u` le vide |
| `s` / `S` | Synchroniser tous les dépôts / le dépôt sélectionné |
| `a` | Ajouter un dépôt à suivre (vue Dépôts) ; `entrée` valide, `échap` annule |
| `r` | Relire la base |
| `?` | Aide des touches |
| `q`, `ctrl+c` | Quitter (pendant la saisie d'un filtre ou d'un dépôt, seul `ctrl+c` quitte) |

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
watchoo contributors --repo retardataire/projet-licence --weeks 6
watchoo serve
```

Partager la configuration avec une collègue, qui l'importe dans son propre espace :

```bash
watchoo config export -o licence.json          # chez soi
watchoo config import licence.json --dry-run    # chez la collègue : aperçu
watchoo config import licence.json              # puis application, après confirmation
watchoo sync
```

Depuis n'importe où, consulter un autre espace : `watchoo -C ~/supervision/master-projet status`.
