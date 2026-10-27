package selfupdate

// Versions SemVer 2.0.0 (https://semver.org/lang/fr/) : analyse et ordre de précédence.

import (
	"strconv"
	"strings"
)

// Version est une version SemVer analysée : « v1.2.3 », « 1.2.3-rc.1 », « v1.2.3+build ».
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string // identifiants de pré-version (« rc », « 1 ») ; vide pour une version finale
	Build               string   // métadonnées de build (après « + ») ; ignorées pour l'ordre
}

// ParseVersion analyse s, avec ou sans préfixe « v ». Elle renvoie false si s n'est pas une version
// SemVer valide (« dev », « 1.2 », « 01.2.3 »…).
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	var v Version
	if core, build, ok := strings.Cut(s, "+"); ok {
		if !validIdentifiers(build, false) {
			return Version{}, false
		}
		s, v.Build = core, build
	}
	core, pre, hasPre := strings.Cut(s, "-")
	if hasPre {
		if !validIdentifiers(pre, true) {
			return Version{}, false
		}
		v.Pre = strings.Split(pre, ".")
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	nums := [3]*uint64{&v.Major, &v.Minor, &v.Patch}
	for i, p := range parts {
		n, ok := numericIdentifier(p)
		if !ok {
			return Version{}, false
		}
		*nums[i] = n
	}
	return v, true
}

// numericIdentifier analyse un identifiant numérique SemVer : chiffres seulement, sans zéro en tête.
func numericIdentifier(s string) (uint64, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

// validIdentifiers vérifie une suite d'identifiants séparés par des points : non vides, en
// [0-9A-Za-z-] ; pour une pré-version (pre), un identifiant numérique n'a pas de zéro en tête.
func validIdentifiers(s string, pre bool) bool {
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return false
		}
		numeric := true
		for _, r := range id {
			switch {
			case r >= '0' && r <= '9':
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
				numeric = false
			default:
				return false
			}
		}
		if pre && numeric && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}

// IsFinal indique une version finale : ni pré-version ni métadonnées de build.
func (v Version) IsFinal() bool {
	return len(v.Pre) == 0 && v.Build == ""
}

// String renvoie la version avec son préfixe « v » (« v1.2.3-rc.1+build »).
func (v Version) String() string {
	s := "v" + strconv.FormatUint(v.Major, 10) + "." + strconv.FormatUint(v.Minor, 10) + "." +
		strconv.FormatUint(v.Patch, 10)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare renvoie -1, 0 ou 1 selon que v précède, égale ou suit o dans l'ordre SemVer : MAJOR, MINOR
// et PATCH numériquement, puis une pré-version avant la version finale, puis les identifiants de
// pré-version un à un (numériques entre eux, numérique avant alphanumérique, texte en ASCII, liste plus
// courte d'abord). Les métadonnées de build sont ignorées.
func (v Version) Compare(o Version) int {
	for _, p := range [][2]uint64{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if p[0] != p[1] {
			if p[0] < p[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(v.Pre) == 0 && len(o.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		return 1
	case len(o.Pre) == 0:
		return -1
	}
	for i := 0; i < len(v.Pre) && i < len(o.Pre); i++ {
		if c := compareIdentifier(v.Pre[i], o.Pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(v.Pre) < len(o.Pre):
		return -1
	case len(v.Pre) > len(o.Pre):
		return 1
	}
	return 0
}

// compareIdentifier compare deux identifiants de pré-version.
func compareIdentifier(a, b string) int {
	na, aNum := numericIdentifier(a)
	nb, bNum := numericIdentifier(b)
	switch {
	case aNum && bNum:
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		}
		return 0
	case aNum:
		return -1
	case bNum:
		return 1
	}
	return strings.Compare(a, b)
}

// IsRelease indique si s est la version d'une release publiée : version SemVer finale (« v0.5.0 »).
// « dev », une pseudo-version de Go (« v0.0.0-20261027100000-9412b2f170d8 »), une pré-version, un
// instantané (« v0.5.1-SNAPSHOT-abc1234 ») ou une version avec métadonnées de build ne le sont pas :
// aucun avis ni mise à jour automatique pour elles.
func IsRelease(s string) bool {
	v, ok := ParseVersion(s)
	return ok && v.IsFinal()
}
