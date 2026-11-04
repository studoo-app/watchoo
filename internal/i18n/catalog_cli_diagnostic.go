// Textes du paquet internal/cli pour « watchoo diagnostic » (phase 7.2) : aide, rappel, tableau des
// pistes de réflexion et contexte D4. Préfixe des clés : « cli.diag. ». Les clés JSON ne passent pas
// par le catalogue (schéma stable) ; seul le rappel « notice » y est traduit.
package i18n

// cliDiagnosticTexts regroupe les textes de « watchoo diagnostic », enregistrés par init() ci-dessous.
var cliDiagnosticTexts = map[string]entry{
	"cli.diag.short": {
		"Shows the diagnosis leads of the tracked repositories (trainer only)",
		"Affiche les pistes de diagnostic des dépôts suivis (réservé au formateur)",
	},
	"cli.diag.long": {
		`Shows the reflection leads of the diagnosis, for the trainer who supervises the workspace:
commits and pull requests that declare the use of an AI tool, newest first, with the signals that
raised them and their raw values. A lead is never a verdict: it is a starting point for a
conversation with the learner ("explain this commit to me").

Level "declared": at least one of
  D1  AI co-author trailer in the commit message (Co-authored-by:)
  D2  AI signature in the commit message or the pull request description
  D3  bot author
  D5  pull request branch named by a coding agent (codex/, copilot/, claude/, cursor/)
D4 (agent configuration file in the repository) is shown as context only.

Level "to review": no declared signal, but at least one rhythm signal, each learner being compared
with their own history in the repository over 90 days (modified z-score above 3.5):
  R1  net lines of a day        R2  lines added by a commit (over 150)
  R3  addition ratio (over 0.95) R4  apparent speed (diagnostic.speed_signal, off by default)
  R5  files of a commit (8 or more)
  R6  regime break (14-day median of R1 over 3 times the previous 90 days)
Each rhythm signal needs at least 20 observations in the 90 days before: measured commits for R2, R3 and
R5, worked days for R1 and R6; below that, it does not trigger for the learner.

The diagnosis must be enabled in the workspace (diagnostic.enabled) and is measured at sync on the
local clones, for the commits synced after it was enabled.

Filters:
  --repo     one tracked repository (owner/repo or URL)
  --author   one author (exact login, name or e-mail; login only for pull requests)
  --since    a recent period (e.g. 30d, 7d)`,
		`Affiche les pistes de réflexion du diagnostic, pour le formateur qui supervise l'espace :
commits et pull requests qui déclarent l'usage d'un outil d'IA, du plus récent au plus ancien, avec
les signaux qui les ont levés et leurs valeurs brutes. Une piste n'est jamais un verdict : c'est un
point de départ pour un échange avec l'apprenant (« explique-moi ce commit »).

Niveau « déclaré » : au moins un signal parmi
  D1  trailer de co-auteur IA dans le message du commit (Co-authored-by:)
  D2  signature d'IA dans le message du commit ou la description de la pull request
  D3  auteur bot
  D5  branche de pull request nommée par un agent de codage (codex/, copilot/, claude/, cursor/)
D4 (fichier de configuration d'agent dans le dépôt) n'est affiché qu'en contexte.

Niveau « à regarder » : aucun signal déclaré, mais au moins un signal de rythme, chaque apprenant
étant comparé à son propre historique dans le dépôt sur 90 jours (score z modifié au-delà de 3,5) :
  R1  lignes nettes d'un jour      R2  lignes ajoutées par un commit (plus de 150)
  R3  ratio d'ajout (plus de 0,95) R4  vitesse apparente (diagnostic.speed_signal, désactivé par défaut)
  R5  fichiers d'un commit (8 ou plus)
  R6  rupture de régime (médiane R1 sur 14 jours plus de 3 fois celle des 90 jours précédents)
Chaque signal de rythme demande au moins 20 observations dans les 90 jours qui précèdent : commits
mesurés pour R2, R3 et R5, jours travaillés pour R1 et R6 ; en deçà, il ne se déclenche pas.

Le diagnostic doit être activé dans l'espace (diagnostic.enabled) ; il est mesuré à la synchro sur
les clones locaux, pour les commits synchronisés après son activation.

Filtres :
  --repo     un dépôt suivi (owner/repo ou URL)
  --author   un auteur (login, nom ou e-mail exact ; login seulement pour les pull requests)
  --since    une période récente (ex. 30d, 7d)`,
	},
	"cli.diag.notice": {
		"Reflection leads for the trainer, not verdicts: talk them through with the learner.",
		"Pistes de réflexion pour le formateur, pas des verdicts : à reprendre avec l'apprenant.",
	},
	"cli.diag.disabled": {
		"The diagnosis is disabled in this workspace (diagnostic.enabled: false in .watchoo/config.yaml).\nInform the learners before enabling it.",
		"Le diagnostic est désactivé dans cet espace (diagnostic.enabled: false dans .watchoo/config.yaml).\nInformez les apprenants avant de l'activer.",
	},
	"cli.diag.none":              {"No reflection lead.", "Aucune piste de réflexion."},
	"cli.diag.unknown_author":    {"unknown", "inconnu"},
	"cli.diag.fmt.date":          {"01/02/2006", "02/01/2006"},
	"cli.diag.level.declared":    {"declared", "déclaré"},
	"cli.diag.header_item":       {"Item", "Élément"},
	"cli.diag.header_item_ascii": {"Item", "Element"},
	"cli.diag.header_level":      {"Level", "Niveau"},
	"cli.diag.header_signals":    {"Signals", "Signaux"},
	"cli.diag.level.to_review":   {"to review", "à regarder"},
	"cli.diag.item_day":          {"day", "jour"},
	"cli.diag.hit_median_z":      {"(median %s, z %s)", "(médiane %s, z %s)"},
	"cli.diag.hit_median":        {"(median %s)", "(médiane %s)"},
	"cli.diag.history_short": {
		"%d of %d authors do not have enough history yet (fewer than %d measured commits in 90 days): rhythm signals do not trigger for them. See \"watchoo diagnostic backfill\".",
		"%d auteurs sur %d n'ont pas encore assez d'historique (moins de %d commits mesurés en 90 jours) : les signaux de rythme ne se déclenchent pas pour eux. Voir « watchoo diagnostic backfill ».",
	},
	"cli.diag.backfill_short": {
		"Measures the history already cloned for the diagnosis",
		"Mesure l'historique déjà cloné pour le diagnostic",
	},
	"cli.diag.backfill_long": {
		`Measures, on the local Git clones, the commits already stored that have no diagnosis signals yet
(commits synced before the diagnosis was enabled), within the retention period
(diagnostic.retention_days), merge commits excluded. Useful right after enabling the diagnosis: the
rhythm signals need at least 20 measured commits in 90 days per author. Commits already measured are
never measured again.

  --repo   one tracked repository (owner/repo or URL); all by default`,
		`Mesure, sur les clones Git locaux, les commits déjà en base qui n'ont pas encore de signaux de
diagnostic (commits synchronisés avant l'activation du diagnostic), dans la durée de conservation
(diagnostic.retention_days), hors commits de fusion. Utile juste après l'activation : les signaux de
rythme demandent au moins 20 commits mesurés en 90 jours par auteur. Un commit déjà mesuré ne l'est
jamais deux fois.

  --repo   un dépôt suivi (owner/repo ou URL) ; tous par défaut`,
	},
	"cli.diag.backfill_repo":   {"%s: %d commits measured (%d merges set aside)", "%s : %d commits mesurés (%d fusions écartées)"},
	"cli.diag.backfill_failed": {"%s: not measured: %s", "%s : non mesuré : %s"},
	"cli.diag.backfill_none":   {"No repository to measure.", "Aucun dépôt à mesurer."},
	"cli.diag.backfill_total":  {"%d commits measured in total.", "%d commits mesurés au total."},
	"cli.diag.backfill_errors": {"repositories not measured: %d", "dépôts non mesurés : %d"},
	"cli.diag.context_title":   {"Context (D4):", "Contexte (D4) :"},
	"cli.diag.context_line": {
		"%s: agent configured since %s",
		"%s : agent configuré depuis le %s",
	},
}

func init() {
	register(cliDiagnosticTexts)
}
