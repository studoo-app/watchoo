# syntax=docker/dockerfile:1
#
# Image Docker de Watchoo : watchoo et la CLI GitHub officielle (gh), sur une base distroless sans
# shell, en utilisateur non root.
#
#   make docker                                  # image locale watchoo:dev (version, commit, date injectés)
#   docker build -t watchoo:dev .                # idem, version « dev »
#
# Multi-architecture (linux/amd64, linux/arm64) sans émulation : les deux premières étapes tournent sur
# la plateforme de construction ($BUILDPLATFORM) et compilent / téléchargent pour la plateforme cible
# ($TARGETARCH) ; l'étape finale ne lance aucune commande.
#
# Le token GitHub n'est JAMAIS dans l'image : il est donné à l'exécution par la variable GH_TOKEN, que
# gh lit (laisser github.user vide dans ce cas). Exemple complet : README.md et
# documentation/03-architecture.md, « Distribution ».

# Images de base épinglées par version et par empreinte (digest de l'index multi-architecture).
ARG GO_IMAGE=golang:1.27.3-alpine3.24@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb
ARG RUNTIME_IMAGE=gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

# ── gh : binaire officiel de la release GitHub cli/cli, vérifié par SHA-256 ─────────────────────────
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS gh
ARG TARGETARCH
# Version fixée de gh et empreintes de ses archives Linux, recopiées du fichier
# gh_<version>_checksums.txt de la release. Pour monter de version : changer les trois valeurs.
ARG GH_VERSION=2.103.3
ARG GH_SHA256_AMD64=4c806978ffce1cec70853302a56d70ac6da4f19bae7adca4e1a8ab00633dd91d
ARG GH_SHA256_ARM64=ee32df2323c8847f5fac6cb6b4b69ca75828e9329a8f22f677021f7b3306ce63
WORKDIR /tmp/gh
RUN set -eu; \
    case "${TARGETARCH}" in \
      amd64) expected="${GH_SHA256_AMD64}" ;; \
      arm64) expected="${GH_SHA256_ARM64}" ;; \
      *) echo "architecture non prise en charge : ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    archive="gh_${GH_VERSION}_linux_${TARGETARCH}.tar.gz"; \
    base="https://github.com/cli/cli/releases/download/v${GH_VERSION}"; \
    wget -q -O "${archive}" "${base}/${archive}"; \
    wget -q -O checksums.txt "${base}/gh_${GH_VERSION}_checksums.txt"; \
    # 1. Empreinte fixée ci-dessus (indépendante du serveur de téléchargement).
    echo "${expected}  ${archive}" | sha256sum -c -; \
    # 2. Même empreinte dans le fichier de sommes publié avec la release.
    grep -E "^${expected}  ${archive}\$" checksums.txt >/dev/null \
      || { echo "empreinte absente du fichier de sommes de la release gh" >&2; exit 1; }; \
    tar -xzf "${archive}"; \
    install -m 0755 "gh_${GH_VERSION}_linux_${TARGETARCH}/bin/gh" /usr/local/bin/gh; \
    install -m 0644 "gh_${GH_VERSION}_linux_${TARGETARCH}/LICENSE" /usr/local/share/gh-LICENSE

# ── watchoo : compilé sans CGO, mêmes -ldflags que le Makefile et GoReleaser ────────────────────────
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download && go mod verify
COPY . .
# Version, SHA complet du commit et date du commit (ISO 8601 UTC), filtrés comme dans le Makefile.
ARG VERSION=dev
ARG COMMIT=
ARG DATE=
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    version="$(printf '%s' "${VERSION}" | LC_ALL=C tr -cd '[:alnum:]._+-')"; \
    commit="$(printf '%s' "${COMMIT}" | LC_ALL=C tr -cd '[:alnum:]')"; \
    date="$(printf '%s' "${DATE}" | LC_ALL=C tr -cd '[:alnum:]:-')"; \
    pkg=github.com/studoo-app/watchoo; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath \
      -ldflags "-s -w -X ${pkg}/internal/cli.Version=${version:-dev} -X ${pkg}/internal/cli.Commit=${commit} -X ${pkg}/internal/cli.Date=${date}" \
      -o /out/watchoo ./cmd/watchoo; \
    # Répertoire de l'espace (volume), à l'utilisateur nonroot (65532) de l'image finale, en 0700.
    mkdir -p /out/data && chmod 0700 /out/data

# ── image finale ────────────────────────────────────────────────────────────────────────────────────
FROM ${RUNTIME_IMAGE}
ARG VERSION=dev
LABEL org.opencontainers.image.title="watchoo" \
      org.opencontainers.image.description="Supervision de dépôts GitHub (CLI, TUI, interface web), en lecture seule" \
      org.opencontainers.image.source="https://github.com/studoo-app/watchoo" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=gh /usr/local/bin/gh /usr/local/bin/gh
COPY --from=gh /usr/local/share/gh-LICENSE /usr/share/doc/gh/LICENSE
COPY --from=build /out/watchoo /usr/local/bin/watchoo
COPY --from=build /src/LICENSE /src/NOTICE /usr/share/doc/watchoo/
COPY --from=build --chown=65532:65532 /out/data /data

# Espace de supervision dans le volume /data (base SQLite : données personnelles, à ne pas partager).
# gh ne cherche pas de mise à jour (aucun appel réseau inutile).
ENV WATCHOO_DIR=/data \
    GH_NO_UPDATE_NOTIFIER=1
# Utilisateur nonroot de distroless (uid / gid 65532), jamais root.
USER 65532:65532
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080

# Pas de HEALTHCHECK : l'image n'a ni shell ni client HTTP, toutes les routes exigent
# l'authentification et Watchoo n'a pas (encore) de route de santé. Voir « Distribution ».
ENTRYPOINT ["/usr/local/bin/watchoo"]
# Hors boucle locale : l'authentification est obligatoire (WATCHOO_WEB_AUTH_USERNAME et
# WATCHOO_WEB_PASSWORD, ou web.trusted_proxy et web.allowed_hosts), sinon serve refuse de démarrer.
CMD ["serve", "--addr", "0.0.0.0:8080"]
