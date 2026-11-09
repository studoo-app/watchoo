package conventions

// Texte du détail d'une piste (phase 8.7), commun à la CLI et au web : les compteurs utiles et les dates
// d'une piste, traduits par la fonction du catalogue de la couche d'affichage (i18n.T ou Translator.T),
// sans dépendre du paquet i18n. Ni chemin, ni valeur d'en-tête, ni sujet de commit : la piste n'en porte
// aucun.

import (
	"strconv"
	"strings"
	"time"
)

// Translate traduit une clé du catalogue avec ses arguments.
type Translate func(key string, args ...any) string

// LeadDetail renvoie le détail traduit d'une piste : compteurs utiles puis dates (formatées par date) ;
// vide pour une piste inconnue ou sans compteur. Les clés sont « conv.lead_detail.* », « conv.lead_header.* »
// et « conv.lead_date.* ».
func LeadDetail(t Translate, date func(time.Time) string, l LeadCheck) string {
	if l.Status == CriterionUnknown || len(l.Counts) == 0 {
		return ""
	}
	c := l.Counts
	present := func(key string, ok bool) string {
		if ok {
			return t(key + "_present")
		}
		return t(key + "_absent")
	}
	var parts []string
	switch l.ID {
	case CriterionCL11:
		parts = append(parts, t("conv.lead_detail.CL11", c[CountLinked], c[CountTitles]))
	case CriterionCL12:
		parts = append(parts, t("conv.lead_detail.CL12", c[CountBullets], c[CountSubjectPercent], c[CountSubjects],
			c[CountShaMergePercent]))
	case CriterionRD5:
		parts = append(parts, t("conv.lead_detail.RD5", c[CountCommandBlocks]))
		if c[CountEnvAnnounced] > 0 {
			parts = append(parts, present("conv.lead_detail.env_example", c[CountEnvExample] > 0))
		}
	case CriterionDOC6:
		parts = append(parts, t("conv.lead_detail.DOC6", c[CountComplete], c[CountDocuments]))
		fields := make([]string, 0, len(AllHeaderFields))
		for _, f := range AllHeaderFields {
			fields = append(fields, t("conv.lead_header."+f)+" "+strconv.Itoa(c[HeaderCountKey(f)]))
		}
		parts = append(parts, t("conv.lead_detail.DOC6_fields", strings.Join(fields, ", ")))
	case CriterionDOC7:
		parts = append(parts, t("conv.lead_detail.DOC7", c[CountImages], c[CountBadges], c[CountMermaid]),
			present("conv.lead_detail.media", c[CountMediaDir] > 0))
	case CriterionDOC9:
		parts = append(parts, t("conv.lead_detail.DOC9", c[CountCommitsSince], c[CountDaysBehind]))
	case CriterionDOC10:
		parts = append(parts, t("conv.lead_detail.DOC10", c[CountRows], c[CountCompared], c[CountUnresolved]))
		if n := c[CountOverLimit]; n > 0 {
			parts = append(parts, t("conv.lead_detail.DOC10_over", n, DOC10MaxDocuments))
		}
	case CriterionTL4:
		if checks := CIChecks(c[CountCIMask]); len(checks) > 0 {
			parts = append(parts, t("conv.lead_detail.TL4", c[CountWorkflows], strings.Join(checks, ", ")))
		} else {
			parts = append(parts, t("conv.lead_detail.TL4_none", c[CountWorkflows]))
		}
	case CriterionTL5:
		parts = append(parts, t("conv.lead_detail.TL5", c[CountBeforeConform], c[CountBefore], c[CountBeforePercent],
			c[CountAfterConform], c[CountAfter], c[CountAfterPercent]),
			t("conv.lead_detail.TL5_sources", c[CountSources], c[CountUndatedSources]))
	}
	for _, k := range AllLeadDates {
		if at, ok := l.Dates[k]; ok && !at.IsZero() {
			parts = append(parts, t("conv.lead_date."+k, date(at)))
		}
	}
	return strings.Join(parts, t("conv.lead_sep"))
}
