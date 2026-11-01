// Textes du paquet internal/selfupdate : vérification de la dernière version de Watchoo,
// téléchargement vérifié et remplacement du binaire. Préfixe des clés : « selfupdate. ».
package i18n

// selfupdateTexts regroupe les textes de internal/selfupdate, enregistrés dans le catalogue par le
// init() ci-dessous.
var selfupdateTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : », l'anglais non.
	"selfupdate.err.sep": {": ", " : "},

	// Erreurs sentinelles (install.go), testées avec errors.Is.
	"selfupdate.err.docker":       {"watchoo is running in a Docker container", "watchoo tourne dans un conteneur Docker"},
	"selfupdate.err.dev_build":    {"development version", "version de développement"},
	"selfupdate.err.not_writable": {"directory of the binary not writable", "répertoire du binaire non accessible en écriture"},
	"selfupdate.err.checksum": {
		"wrong SHA-256 checksum of the archive: archive corrupted or modified",
		"somme SHA-256 de l'archive incorrecte : archive corrompue ou modifiée",
	},

	// Téléchargement et installation (install.go).
	"selfupdate.exe.not_found": {"path of the running binary not found", "chemin du binaire en cours introuvable"},
	"selfupdate.version.invalid": {
		"invalid version: %q (expected: vX.Y.Z)",
		"version invalide : %q (attendu : vX.Y.Z)",
	},
	"selfupdate.download": {"downloading %s", "téléchargement de %s"},
	"selfupdate.download.http": {
		"downloading %s: HTTP response %d (version or platform without an archive?)",
		"téléchargement de %s : réponse HTTP %d (version ou plateforme sans archive ?)",
	},
	"selfupdate.download.too_big": {
		"downloading %s: file too big (more than %d MB)",
		"téléchargement de %s : fichier trop gros (plus de %d Mo)",
	},
	"selfupdate.checksum.unreadable": {
		"checksum of %s unreadable in the checksums file",
		"somme de %s illisible dans le fichier de sommes",
	},
	"selfupdate.checksum.missing": {
		"%s missing from the checksums file of the release",
		"%s absent du fichier de sommes de la release",
	},
	"selfupdate.extract":         {"extracting %s", "extraction de %s"},
	"selfupdate.extract.missing": {"extracting %s: binary %s missing from the archive", "extraction de %s : binaire %s absent de l'archive"},
	"selfupdate.extract.empty":   {"extracting %s: empty binary %s", "extraction de %s : binaire %s vide"},
	"selfupdate.binary.too_big":  {"binary too big (more than %d MB)", "binaire trop gros (plus de %d Mo)"},
	"selfupdate.replace.write":   {"writing the new binary", "écriture du nouveau binaire"},
	"selfupdate.replace.perms":   {"permissions of the new binary", "droits du nouveau binaire"},
	"selfupdate.replace.rename":  {"replacing %s", "remplacement de %s"},
	"selfupdate.replace.remove":  {"removing %s", "suppression de %s"},
	"selfupdate.replace.aside":   {"moving %s aside", "mise de côté de %s"},

	// Lecture de la dernière version (selfupdate.go).
	"selfupdate.base.refused": {
		"releases address refused (HTTPS required): %s",
		"adresse des releases refusée (HTTPS obligatoire) : %s",
	},
	"selfupdate.redirect.too_many": {"too many redirects", "trop de redirections"},
	"selfupdate.redirect.refused": {
		"redirect refused (HTTPS required): %s",
		"redirection refusée (HTTPS obligatoire) : %s",
	},
	"selfupdate.request": {"request to %s", "requête vers %s"},
	"selfupdate.latest.none": {
		"no published release found (no redirect to a version)",
		"aucune release publiée trouvée (pas de redirection vers une version)",
	},
	"selfupdate.latest.http": {
		"latest version unreadable: HTTP response %d",
		"dernière version illisible : réponse HTTP %d",
	},
	"selfupdate.latest.no_version": {
		"latest version unreadable: redirect without a version",
		"dernière version illisible : redirection sans version",
	},
	"selfupdate.latest.bad_tag": {
		"latest version unreadable: unexpected tag %q (expected: vX.Y.Z)",
		"dernière version illisible : tag %q inattendu (attendu : vX.Y.Z)",
	},
}

func init() { register(selfupdateTexts) }
