package selfupdate

import "github.com/studoo-app/watchoo/internal/semver"

// IsRelease indique si s est la version d'une release publiée : version SemVer finale (« v0.5.0 »).
// « dev », une pseudo-version de Go (« v0.0.0-20261027100000-9412b2f170d8 »), une pré-version, un
// instantané (« v0.5.1-SNAPSHOT-abc1234 ») ou une version avec métadonnées de build ne le sont pas :
// aucun avis ni mise à jour automatique pour elles.
func IsRelease(s string) bool {
	v, ok := semver.ParseVersion(s)
	return ok && v.IsFinal()
}
