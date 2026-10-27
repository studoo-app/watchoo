# Tâches de développement de Watchoo. « make help » liste les cibles.
#
# La CI (.github/workflows/ci.yml) lance les mêmes cibles : un « make check » vert en local
# annonce une CI verte.

GO       ?= go
SQLC     ?= sqlc
TAILWIND ?= tailwindcss
PKG     := github.com/studoo-app/watchoo
BIN     := bin/watchoo
# Version injectée dans le binaire (watchoo version) : tag Git le plus proche, sinon commit court,
# suffixé de « -dirty » si l'arbre de travail a des modifications ; « dev » hors d'un dépôt Git.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Un nom de tag Git peut contenir « " », « $ », « ; » ou « ` » (et un tag arrive tout seul avec un
# « git fetch ») : VERSION n'est donc jamais recopiée dans le texte d'une recette, que le shell
# interpréterait. Elle passe par l'environnement (WATCHOO_VERSION), et la recette n'en garde que les
# caractères A-Z a-z 0-9 . _ + - (« dev » s'il ne reste rien).
export WATCHOO_VERSION := $(VERSION)
# Commit et date du commit (ISO 8601 UTC, format fixe 2006-01-02T15:04:05Z), injectés comme la version
# et filtrés de la même façon (alphanumériques, « : » et « - » pour la date). Vides hors d'un dépôt Git.
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
DATE   ?= $(shell TZ=UTC0 git log -1 --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ 2>/dev/null)
export WATCHOO_COMMIT := $(COMMIT)
export WATCHOO_DATE := $(DATE)

# Options -ldflags communes à « make build », à l'image Docker (Dockerfile, arguments VERSION, COMMIT
# et DATE) et à GoReleaser (.goreleaser.yaml) : à garder identiques dans les trois. Les variables
# shell $$version, $$commit et $$date sont posées par la recette (valeurs filtrées). Un -X vers une
# variable absente du binaire est ignoré par l'éditeur de liens.
LDFLAGS = -s -w -X $(PKG)/internal/cli.Version=$${version:-dev} \
	-X $(PKG)/internal/cli.Commit=$$commit -X $(PKG)/internal/cli.Date=$$date
# Pose $$version, $$commit et $$date à partir de l'environnement, sans jamais recopier les valeurs
# dans le texte de la recette.
SET_BUILD_VARS = version="$$(printf '%s' "$$WATCHOO_VERSION" | LC_ALL=C tr -cd '[:alnum:]._+-')"; \
	commit="$$(printf '%s' "$$WATCHOO_COMMIT" | LC_ALL=C tr -cd '[:alnum:]')"; \
	date="$$(printf '%s' "$$WATCHOO_DATE" | LC_ALL=C tr -cd '[:alnum:]:-')"

GORELEASER ?= goreleaser
DOCKER     ?= docker
# Image construite par « make docker » (locale, jamais poussée par le Makefile).
IMAGE      ?= watchoo:dev

# Feuille de style de l'interface web : source Tailwind + DaisyUI (vendored) et CSS généré, commité.
CSS_IN  := internal/web/tailwind/input.css
CSS_OUT := internal/web/static/app.css
# Logos de l'interface web : copiés de assets/logo dans internal/web/static (go:embed ne remonte pas
# hors du package).
WEB_LOGOS := watchoo-icone.svg watchoo-logo-final-nom.svg watchoo-logo-final-nom-dark.svg favicon.ico \
	apple-touch-icon.png icon-192.png icon-512.png

.DEFAULT_GOAL := help
.PHONY: help build test vet fmt-check sqlc sqlc-check css css-check css-check-if-available logo check \
	release-check release-snapshot docker clean

help: ## Liste les cibles disponibles
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-17s %s\n", $$1, $$2}'

build: ## Compile bin/watchoo (sans CGO, version injectée)
	@$(SET_BUILD_VARS); set -x; \
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/watchoo

test: ## Lance tous les tests avec le détecteur de concurrence
	$(GO) test -race ./...

vet: ## Lance go vet (y compris sur le faux gh des tests, ignoré par ./...)
	$(GO) vet ./...
	$(GO) vet ./internal/auth/testdata/fakegh

fmt-check: ## Échoue si un fichier Go n'est pas formaté par gofmt
	@out="$$(gofmt -l .)"; \
	if [ -n "$$out" ]; then \
		echo "fichiers à formater (gofmt -w) :" >&2; echo "$$out" >&2; exit 1; \
	fi

sqlc: ## Régénère internal/store/db à partir des requêtes et des migrations
	$(SQLC) generate

# Régénère dans un répertoire vidé (un fichier généré orphelin est donc signalé aussi), puis compare
# avec l'instantané pris avant ; en cas d'échec de sqlc, l'instantané est remis en place.
sqlc-check: ## Échoue si le code généré par sqlc n'est pas à jour
	@tmp="$$(mktemp -d)"; trap 'rm -rf "$$tmp"' EXIT; \
	cp -R internal/store/db "$$tmp/db" && rm -rf internal/store/db || exit 1; \
	if ! $(SQLC) generate; then \
		rm -rf internal/store/db; cp -R "$$tmp/db" internal/store/db; exit 1; \
	fi; \
	if ! diff -r "$$tmp/db" internal/store/db; then \
		echo "le code généré par sqlc n'était pas à jour : il vient d'être régénéré, commitez internal/store/db" >&2; \
		exit 1; \
	fi

# CLI autonome de Tailwind CSS (sans Node ni npm), version fixée dans internal/web/tailwind/VERSIONS.
# Elle lit les templates, les fichiers Go et le JavaScript de internal/web (sources déclarées dans
# input.css) et n'écrit que les classes utilisées. À relancer après toute modification des templates.
css: ## Régénère internal/web/static/app.css (CLI autonome tailwindcss, DaisyUI vendored)
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

# Régénère dans un fichier temporaire et compare avec le fichier commité (la CI l'exige).
css-check: ## Échoue si internal/web/static/app.css n'est pas à jour
	@tmp="$$(mktemp)"; trap 'rm -f "$$tmp"' EXIT; \
	if ! $(TAILWIND) -i $(CSS_IN) -o "$$tmp" --minify >/dev/null 2>&1; then \
		echo "échec de $(TAILWIND) (installé ? version de internal/web/tailwind/VERSIONS)" >&2; exit 1; \
	fi; \
	if ! cmp -s "$$tmp" $(CSS_OUT); then \
		echo "$(CSS_OUT) n'est pas à jour : lancez « make css » puis commitez-le" >&2; exit 1; \
	fi

# En local, « make check » saute css-check si la CLI tailwindcss n'est pas installée (message clair) ;
# la CI l'installe et lance css-check sans condition.
css-check-if-available:
	@if command -v $(TAILWIND) >/dev/null 2>&1; then \
		$(MAKE) --no-print-directory css-check; \
	else \
		echo "css-check sauté : $(TAILWIND) introuvable (CLI autonome Tailwind, voir internal/web/tailwind/VERSIONS) ; la CI le vérifie" >&2; \
	fi

# go:embed ne remonte pas hors du package : internal/logo/art/ garde une copie des versions texte du
# logo, identique à assets/logo/*.txt, et internal/web/static/ une copie des logos et icônes de
# l'interface web (des tests de « make test » vérifient les deux).
logo: ## Recopie le logo de assets/logo dans internal/logo/art/ (texte) et internal/web/static/ (web)
	rm -f internal/logo/art/*.txt
	cp assets/logo/*.txt internal/logo/art/
	cd assets/logo && cp $(WEB_LOGOS) ../../internal/web/static/

check: fmt-check vet test sqlc-check css-check-if-available build ## Lance tous les contrôles (comme la CI)

# GoReleaser (.goreleaser.yaml) : binaires linux / darwin / windows en amd64 et arm64, archives et
# sommes SHA-256 dans dist/. Rien n'est publié : la release (brouillon) et l'image Docker sont faites
# par .github/workflows/release.yml, sur un tag vX.Y.Z.
release-check: ## Vérifie .goreleaser.yaml (goreleaser check)
	$(GORELEASER) check

release-snapshot: ## Construit une release locale dans dist/, sans rien publier
	$(GORELEASER) release --snapshot --clean

# Image Docker locale (Dockerfile) : watchoo compilé dans l'image avec les mêmes -ldflags, et gh.
docker: ## Construit l'image Docker locale (IMAGE=watchoo:dev par défaut), sans la pousser
	@$(SET_BUILD_VARS); set -x; \
	$(DOCKER) build --build-arg VERSION="$${version:-dev}" --build-arg COMMIT="$$commit" \
		--build-arg DATE="$$date" -t "$(IMAGE)" .

clean: ## Supprime les binaires compilés
	rm -rf bin dist
