// Textes du paquet internal/repoimport : références de dépôts, motifs de branches, labels, seuils
// d'activité et fichiers d'import. Préfixe des clés : « repoimport. ».
package i18n

// repoimportTexts regroupe les textes de internal/repoimport, enregistrés dans le catalogue par le
// init() ci-dessous.
var repoimportTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : » et « ; »,
	// l'anglais n'en met pas.
	"repoimport.err.sep":       {": ", " : "},
	"repoimport.err.semicolon": {"; ", " ; "},

	// ParseRef : la forme attendue est rappelée dans chaque message.
	"repoimport.ref.expected": {
		"expected owner/repo or https://github.com/owner/repo",
		"attendu owner/repo ou https://github.com/owner/repo",
	},
	"repoimport.ref.empty": {"empty repository: %s", "dépôt vide : %s"},
	"repoimport.ref.bad_url": {
		"repository: unreadable URL (value not shown), %s",
		"dépôt : URL illisible (valeur non reproduite), %s",
	},
	"repoimport.ref.credentials": {
		"repository: credentials not allowed in the URL (value not shown), %s",
		"dépôt : identifiants interdits dans l'URL (valeur non reproduite), %s",
	},
	"repoimport.ref.scheme": {
		"repository %s: scheme %q not supported, %s",
		"dépôt %s : schéma %q non pris en charge, %s",
	},
	"repoimport.ref.host": {
		"repository %s: only the github.com host is supported, %s",
		"dépôt %s : seul l'hôte github.com est pris en charge, %s",
	},
	"repoimport.ref.query": {
		"repository %s: query parameters and fragment not allowed in the URL, %s",
		"dépôt %s : paramètres et fragment interdits dans l'URL, %s",
	},
	"repoimport.ref.invalid": {"invalid repository %s: %s", "dépôt %s invalide : %s"},
	"repoimport.ref.wrapped": {"repository %s (%s)", "dépôt %s (%s)"},

	// ParseBranchPatterns.
	"repoimport.branch.pattern": {"branch pattern %s", "motif de branche %s"},
	"repoimport.branch.syntax": {
		"invalid branch pattern %s (pattern syntax: *, ?, [a-z])",
		"motif de branche %s invalide (syntaxe de motif : *, ?, [a-z])",
	},

	// Erreurs de lignes du fichier d'import.
	"repoimport.line.prefix":       {"line %d: %s", "ligne %d : %s"},
	"repoimport.parse.errors_one":  {"1 line with errors:", "1 ligne en erreur :"},
	"repoimport.parse.errors_many": {"%d lines with errors:", "%d lignes en erreur :"},
	"repoimport.err.no_entry":      {"no repository in the file", "aucun dépôt dans le fichier"},
	"repoimport.read.too_long": {
		"reading the import file: line longer than %d bytes",
		"lecture du fichier d'import : ligne de plus de %d octets",
	},
	"repoimport.read.err": {"reading the import file", "lecture du fichier d'import"},
	"repoimport.header.unknown": {
		"unknown column %s in the header (possible columns: repo, label, branches, flemme, dead)",
		"colonne %s inconnue dans l'en-tête (colonnes possibles : repo, label, branches, flemme, dead)",
	},
	"repoimport.header.duplicate": {
		"duplicate column %s in the header",
		"colonne %s en double dans l'en-tête",
	},
	"repoimport.line.duplicate": {
		"duplicate repository %s (already on line %d)",
		"dépôt %s en double (déjà présent ligne %d)",
	},
	"repoimport.line.one_repo": {
		"one repository per line expected, got %s (for a CSV, start with the header repo,label,branches)",
		"un seul dépôt par ligne attendu, reçu %s (pour un CSV, commencer par l'en-tête repo,label,branches)",
	},
	"repoimport.line.bad_csv": {"malformed CSV (check the quotes)", "CSV mal formé (guillemets à vérifier)"},
	"repoimport.line.cells": {
		"%d cell(s) instead of %d (as in the header)",
		"%d cellule(s) au lieu de %d (comme l'en-tête)",
	},
	"repoimport.line.empty_repo": {"empty repo column", "colonne repo vide"},

	// Labels.
	"repoimport.label.utf8":    {"label: invalid UTF-8 text", "label : texte UTF-8 invalide"},
	"repoimport.label.control": {"label: control character not allowed", "label : caractère de contrôle interdit"},
	"repoimport.label.too_long": {
		"label too long (%d characters at most)",
		"label trop long (%d caractères au plus)",
	},

	// Seuils d'activité.
	"repoimport.threshold.parse": {
		"%s: whole number of days expected, from 1 to %d (0 = global value)",
		"%s : nombre entier de jours attendu, de 1 à %d (0 = valeur globale)",
	},
	"repoimport.threshold.range": {
		"%s threshold: %d days is invalid (from 1 to %d, or 0 for the global value)",
		"seuil %s : %d jours invalide (de 1 à %d, ou 0 pour la valeur globale)",
	},
	"repoimport.threshold.order": {
		"dead threshold (%d d): must be strictly greater than the flemme threshold (%d d)",
		"seuil dead (%d j) : doit être strictement supérieur au seuil flemme (%d j)",
	},
}

func init() { register(repoimportTexts) }
