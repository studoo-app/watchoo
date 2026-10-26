# Tâches de développement de Watchoo. « make help » liste les cibles.
#
# La CI (.github/workflows/ci.yml) lance les mêmes cibles : un « make check » vert en local
# annonce une CI verte.

GO      ?= go
SQLC    ?= sqlc
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

.DEFAULT_GOAL := help
.PHONY: help build test vet fmt-check sqlc sqlc-check check clean

help: ## Liste les cibles disponibles
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  %-11s %s\n", $$1, $$2}'

build: ## Compile bin/watchoo (sans CGO, version injectée)
	@version="$$(printf '%s' "$$WATCHOO_VERSION" | LC_ALL=C tr -cd '[:alnum:]._+-')"; set -x; \
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -X $(PKG)/internal/cli.Version=$${version:-dev}" -o $(BIN) ./cmd/watchoo

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

check: fmt-check vet test sqlc-check build ## Lance tous les contrôles (comme la CI)

clean: ## Supprime les binaires compilés
	rm -rf bin dist
