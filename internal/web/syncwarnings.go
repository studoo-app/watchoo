package web

// Avertissements de synchro (1.3.0, documentation/fr/06-interface-web.md) : pastille des cartes du
// tableau de bord, alerte de la page d'un dépôt et menu de l'indicateur de synchro. Les avertissements
// viennent du dernier sync_run de chaque dépôt (model.SyncRun.Warnings) ou du dernier rapport en
// mémoire (indicateur) ; leur code (model.SyncWarningKind) est traduit dans la langue de la page, leur
// message reste celui enregistré lors de la synchro.

import (
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
)

// warnLabel renvoie le libellé court d'un avertissement (« Local Git clone unavailable » / « Clone Git
// local indisponible ») ; un code inconnu donne un libellé générique.
func warnLabel(tr *i18n.Translator, kind model.SyncWarningKind) string {
	switch kind {
	case model.WarnStatsNotComputed, model.WarnCloneUnavailable, model.WarnLanguagesNotComputed,
		model.WarnListsNotSynced, model.WarnAINotScanned, model.WarnDiagnosticNotRecorded:
		return tr.T("web.warn.label." + string(kind))
	default:
		return tr.T("web.warn.label.other")
	}
}

// warnEffect renvoie la conséquence d'un avertissement, affichée sur la carte du dépôt (« Line
// statistics read through the API (limited) ») ; un code inconnu donne le libellé générique.
func warnEffect(tr *i18n.Translator, kind model.SyncWarningKind) string {
	switch kind {
	case model.WarnStatsNotComputed, model.WarnCloneUnavailable, model.WarnLanguagesNotComputed,
		model.WarnListsNotSynced, model.WarnAINotScanned, model.WarnDiagnosticNotRecorded:
		return tr.T("web.warn.effect." + string(kind))
	default:
		return tr.T("web.warn.label.other")
	}
}

// warnCloneHint dit si l'un des avertissements vient du clone Git local (accès refusé, git absent…) :
// la page du dépôt rappelle alors de vérifier la clé SSH ou les identifiants Git.
func warnCloneHint(warnings []model.SyncWarning) bool {
	for _, w := range warnings {
		switch w.Kind {
		case model.WarnStatsNotComputed, model.WarnCloneUnavailable, model.WarnLanguagesNotComputed:
			return true
		}
	}
	return false
}

// warnTitle renvoie les libellés courts des avertissements, un par ligne (infobulle de la pastille).
func warnTitle(tr *i18n.Translator, warnings []model.SyncWarning) string {
	labels := make([]string, len(warnings))
	for i, w := range warnings {
		labels[i] = warnLabel(tr, w.Kind)
	}
	return strings.Join(labels, "\n")
}

// warnSummary résume les avertissements d'un dépôt pour le menu de l'indicateur de synchro : le
// libellé court s'il n'y en a qu'un, sinon leur nombre (« 2 warnings »).
func warnSummary(tr *i18n.Translator, warnings []model.SyncWarning) string {
	if len(warnings) == 1 {
		return warnLabel(tr, warnings[0].Kind)
	}
	return pluralText(tr, "web.n.warning", len(warnings))
}
