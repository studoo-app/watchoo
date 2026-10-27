#!/bin/sh
# Installation de Watchoo depuis les releases GitHub (Linux et macOS).
#
#   curl -fsSL https://raw.githubusercontent.com/studoo-app/watchoo/main/install.sh | sh
#
# Le script télécharge l'archive de la dernière version pour ce système, vérifie sa somme SHA-256
# contre le fichier de sommes de la release, puis installe le binaire, sans sudo, dans
# ~/.local/bin. Il ne touche à rien d'autre (ni token, ni configuration, ni PATH).
#
# Réglages, par variables d'environnement (à placer devant « sh ») :
#   WATCHOO_VERSION      version à installer (ex. v0.1.0 ou 0.1.0) ; par défaut, la dernière
#   WATCHOO_INSTALL_DIR  répertoire d'installation ; par défaut ~/.local/bin
#
# Exemple : curl -fsSL …/install.sh | WATCHOO_INSTALL_DIR=/usr/local/bin sudo -E sh
#
# Tout le script est dans des fonctions appelées à la dernière ligne : un téléchargement
# interrompu n'exécute donc rien.

set -eu

REPO_URL="https://github.com/studoo-app/watchoo"
# Racine des releases ; WATCHOO_RELEASES_URL ne sert qu'aux tests (faux serveur local).
RELEASES_URL="${WATCHOO_RELEASES_URL:-$REPO_URL/releases}"
# curl n'accepte que HTTPS, redirections comprises (HTTP seulement pour le faux serveur des tests).
case "$RELEASES_URL" in
http://127.0.0.1:*) PROTO='=http' ;;
*) PROTO='=https' ;;
esac

say() { printf '%s\n' "$*"; }
fail() {
	printf 'erreur : %s\n' "$*" >&2
	exit 1
}

# fetch URL FICHIER : télécharge URL dans FICHIER, avec curl ou wget.
fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --proto "$PROTO" --proto-redir "$PROTO" -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "ni curl ni wget n'est installé"
	fi
}

# latest_tag : tag de la dernière release, lu dans la redirection de …/releases/latest
# (vers …/releases/tag/vX.Y.Z), sans passer par l'API GitHub et son quota.
latest_tag() {
	if command -v curl >/dev/null 2>&1; then
		url=$(curl -fsSL --proto "$PROTO" --proto-redir "$PROTO" -o /dev/null -w '%{url_effective}' "$RELEASES_URL/latest") || return 1
	elif command -v wget >/dev/null 2>&1; then
		url=$(wget -q -O /dev/null --server-response "$RELEASES_URL/latest" 2>&1 |
			sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1 | tr -d '\r')
	else
		fail "ni curl ni wget n'est installé"
	fi
	tag=${url##*/}
	case "$tag" in
	v[0-9]*) printf '%s\n' "$tag" ;;
	*) return 1 ;;
	esac
}

detect_os() {
	case "$(uname -s)" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	MINGW* | MSYS* | CYGWIN*)
		fail "sous Windows, téléchargez l'archive .zip depuis $REPO_URL/releases"
		;;
	*) fail "système non pris en charge : $(uname -s)" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) fail "architecture non prise en charge : $(uname -m)" ;;
	esac
}

# sha256 FICHIER : empreinte SHA-256 de FICHIER.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "ni sha256sum ni shasum n'est installé : impossible de vérifier l'archive"
	fi
}

main() {
	os=$(detect_os)
	arch=$(detect_arch)

	tag="${WATCHOO_VERSION:-}"
	asked="$tag"
	if [ -z "$tag" ]; then
		tag=$(latest_tag) || fail "impossible de trouver la dernière version sur $RELEASES_URL"
	fi
	case "$tag" in
	v*) ;;
	*) tag="v$tag" ;;
	esac
	case "$tag" in
	v[0-9]*.[0-9]*.[0-9]*) ;;
	*) fail "version invalide : $asked (attendu : vX.Y.Z)" ;;
	esac
	version=${tag#v}

	if [ -z "${WATCHOO_INSTALL_DIR:-}" ] && [ -z "${HOME:-}" ]; then
		fail "HOME n'est pas défini : indiquez WATCHOO_INSTALL_DIR"
	fi
	dir="${WATCHOO_INSTALL_DIR:-$HOME/.local/bin}"
	archive="watchoo_${version}_${os}_${arch}.tar.gz"
	sums="watchoo_${version}_checksums.txt"

	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t watchoo)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	say "Téléchargement de Watchoo $tag ($os/$arch)…"
	fetch "$RELEASES_URL/download/$tag/$archive" "$tmp/$archive" ||
		fail "téléchargement de $archive impossible (version $tag inexistante ?)"
	fetch "$RELEASES_URL/download/$tag/$sums" "$tmp/$sums" ||
		fail "téléchargement de $sums impossible"

	want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/$sums")
	[ -n "$want" ] || fail "$archive absent de $sums"
	got=$(sha256 "$tmp/$archive")
	[ "$got" = "$want" ] || fail "somme SHA-256 de $archive incorrecte : archive corrompue ou modifiée"
	say "Somme SHA-256 vérifiée."

	tar -xzf "$tmp/$archive" -C "$tmp" watchoo || fail "extraction de $archive impossible"

	mkdir -p "$dir" 2>/dev/null || fail "impossible de créer $dir (choisissez un autre WATCHOO_INSTALL_DIR)"
	[ -w "$dir" ] || fail "$dir n'est pas accessible en écriture (choisissez un autre WATCHOO_INSTALL_DIR, ou utilisez sudo)"
	# Copie puis renommage : un binaire en cours d'exécution n'est jamais écrasé à moitié.
	cp "$tmp/watchoo" "$dir/.watchoo.tmp.$$"
	chmod 0755 "$dir/.watchoo.tmp.$$"
	mv -f "$dir/.watchoo.tmp.$$" "$dir/watchoo"

	say "Watchoo $tag installé dans $dir/watchoo."
	case ":${PATH:-}:" in
	*":$dir:"*) ;;
	*) say "Attention : $dir n'est pas dans votre PATH. Ajoutez-le, par ex. : export PATH=\"$dir:\$PATH\"" ;;
	esac
	if ! command -v gh >/dev/null 2>&1; then
		say "Attention : Watchoo a besoin de la CLI GitHub « gh » (https://cli.github.com/), puis de « gh auth login »."
	fi
	say "Pour commencer : mkdir mon-groupe && cd mon-groupe && watchoo init"
}

main "$@"
