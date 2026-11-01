<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo/watchoo-logo-final-nom-dark.svg">
    <img src="assets/logo/watchoo-logo-final-nom.svg" alt="Watchoo logo: a pixelated owl with magenta eyes and green pupils, and the name WATCHOO" width="400">
  </picture>
</p>

## **Who worked, on what, and when?**
Watchoo watches a list of GitHub repositories and shows you
their activity at a glance: new commits, contributors, and the repositories that stopped moving.

> **Status: v0.6.2, MVP complete and V2 released, plus the per-contributor page and the one-command update (`watchoo update`).** Phases 1 to 5 of the
> [roadmap](documentation/en/07-roadmap.md) are in place: command line, interactive mode
> (`watchoo tui`), local web interface (`watchoo serve`), `--json` output, binaries and Docker image. The
> V2 (phase 6) adds per-repository thresholds, per-contributor activity within a repository, pull requests,
> releases and tags, and tracking through local Git clones without `gh`. Watchoo is `0.y.z`: the interface may still change from one
> release to the next (see the [changelog](CHANGELOG.md)).

<p align="center">
  <img src="assets/screenshots/contributeurs.png" alt="Table of the workspace's activity over 90 days, Actif, Flemme and Dead counters, repository cards and latest commits (dummy data)" width="900">
  <br><sub>Activity table ...</sub>
</p>

## Who is it for?

- **Instructors** following their students' projects: who is moving forward, who is falling behind.
- **Developers and technical leads** following several repositories at once.

Watchoo works with **private** repositories, and all data stays on your machine.

## What Watchoo does

- 📥 Fetches new commits from each repository, on all its branches, never re-downloading
  what it already has.
- 👥 Shows who contributes, and how much, with activity charts, for the whole workspace or within a
  repository.
- 🚦 Gives every repository a state: **Actif** (Active), **Flemme** (Slacking, more than 5 days without
  a commit), **Dead** (more than 15 days); thresholds settable for the workspace or for each repository.
- 🔀 Tracks pull requests (open, merged), releases and tags.
- 🌿 Works with `gh` (GitHub API) or, without `gh`, with local Git clones that consume no quota.
- 🔄 Updates itself automatically, or on demand.
- 🔒 Read-only: Watchoo never changes anything on GitHub.

Three ways to use it, with a single program:

| Mode | Command | For |
| --- | --- | --- |
| Command line | `watchoo log`, `watchoo repo list`… (`--json` for scripts) | Quick answers, scripts |
| Interactive in the terminal | `watchoo tui` | Keyboard-driven navigation |
| Local web interface | `watchoo serve` | A dashboard in the browser |

## Prerequisites

Your choice (the source is chosen at every sync, with no setting):

- **Recommended**: the [GitHub CLI `gh`](https://cli.github.com/), installed and signed in (`gh auth login`),
  with an account that can access the repositories to follow. Watchoo goes through the GitHub API; it asks
  `gh` for the token (`gh auth token`) and never stores it. **[Git](https://git-scm.com/) is also recommended alongside
  `gh`**: every tracked repository is cloned into `repos/` (see below) to count the lines
  added and removed in all commits, without quota. The clone uses your Git credentials (SSH key
  by default; over HTTPS, `git.protocol: https`, and `gh auth setup-git` so that `git` reuses the
  `gh` account), never the token. When no clone is possible, the sync continues through the API alone and
  line counts are only known for 100 new commits per repository and per sync (warning in the
  summary).
- **Without `gh`** (not released yet): [Git](https://git-scm.com/), with an SSH key (or Git
  credentials over HTTPS, `git.protocol: https`) that can access the repositories. Watchoo tracks them through local clones in
  `repos/<name>`, at the workspace root (name clashes: `repos/<owner>-<name>`), without quota, but without
  pull requests, releases or stars. With or without `gh`, `repos/` takes up roughly the size of the
  tracked repositories, and a repository's first sync clones it in full. `github.com` must be known to SSH (`ssh -T git@github.com` once):
  Watchoo never lets `git` prompt for anything. These clones have a working copy, to read the students'
  code, but **they belong to Watchoo**: at every sync, the working copy is reset
  to the default branch and any local change to a tracked file is **overwritten** (untracked
  files are kept). Work in your own clone. If the workspace lives in a Git
  repository, add `repos/` to its `.gitignore`.

## Installation

### In one command (Linux, macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/studoo-app/watchoo/main/install.sh | sh
```

The [script](install.sh) downloads the latest release for your system, verifies its SHA-256
checksum, then installs `watchoo` into `~/.local/bin`, without sudo. It touches neither your PATH nor
`gh`, and warns you if either is missing. Two settings, placed before `sh`: `WATCHOO_VERSION=v0.6.2`
for a specific version, `WATCHOO_INSTALL_DIR=…` for another directory. You can read the
script before running it: `curl -fsSL …/install.sh -o install.sh`.

### Release binaries

On Windows, or to do everything by hand: the [GitHub releases](https://github.com/studoo-app/watchoo/releases) offer Watchoo for Linux, macOS
and Windows, in amd64 and arm64: `watchoo_<version>_<os>_<arch>.tar.gz` (`.zip` for Windows), with the
SHA-256 checksum file `watchoo_<version>_checksums.txt`.

```bash
# example: Linux amd64
curl -fsSLO https://github.com/studoo-app/watchoo/releases/download/v0.6.2/watchoo_0.6.2_linux_amd64.tar.gz
curl -fsSLO https://github.com/studoo-app/watchoo/releases/download/v0.6.2/watchoo_0.6.2_checksums.txt
sha256sum --check --ignore-missing watchoo_0.6.2_checksums.txt
tar -xzf watchoo_0.6.2_linux_amd64.tar.gz watchoo
sudo install -m 0755 watchoo /usr/local/bin/watchoo
watchoo version
```

On macOS, `shasum -a 256 --check --ignore-missing` replaces `sha256sum`. The binaries are not
signed yet.

### Docker

The `ghcr.io/studoo-app/watchoo` image (linux/amd64 and linux/arm64) contains Watchoo and `gh` (and `git`
for tracking without `gh`, from the next release), runs as
a non-root user and serves the web interface on port 8080. The workspace is the `/data` volume. The GitHub token is **never in the image**: it is given at runtime by `GH_TOKEN`. Since
the interface listens outside the container's loopback, a password is **mandatory**.

```bash
export GH_TOKEN=…               # GitHub token, preferably fine-grained and read-only
export WATCHOO_WEB_PASSWORD=…   # web interface password
docker volume create watchoo-data
docker run --rm -e GH_TOKEN -v watchoo-data:/data ghcr.io/studoo-app/watchoo:0.6.2 init
docker run --rm -e GH_TOKEN -v watchoo-data:/data ghcr.io/studoo-app/watchoo:0.6.2 repo add owner/repo
docker run -d --name watchoo --restart unless-stopped \
  -e GH_TOKEN -e WATCHOO_WEB_PASSWORD -e WATCHOO_WEB_AUTH_USERNAME=prof \
  -e WATCHOO_TIMEZONE=Europe/Paris \
  -v watchoo-data:/data -p 127.0.0.1:8080:8080 \
  --read-only --tmpfs /tmp --tmpfs /home/nonroot:uid=65532,gid=65532,mode=0700 \
  --cap-drop ALL --security-opt no-new-privileges:true \
  ghcr.io/studoo-app/watchoo:0.6.2
```

Then open <http://127.0.0.1:8080/> (user `prof`). The
[`docker-compose.yml`](docker-compose.yml) file gives the equivalent for `docker compose`. The volume holds
personal data (names, e-mails and commit messages from private repositories): **do not share it**.
Details: [Distribution](documentation/en/03-architecture.md#distribution).

### From source

With Go (version stated in `go.mod`):

```bash
go install github.com/studoo-app/watchoo/cmd/watchoo@v0.6.2
```

Or in a clone of the repository:

```bash
make build    # produces bin/watchoo, to copy into a PATH directory
make check    # gofmt, go vet, tests, sqlc code and CSS up to date, build
make docker   # local Docker image watchoo:dev
```

### Upgrading

Upgrading Watchoo means replacing the binary: your workspaces stay as they are, and the
database is upgraded on its own (migrations) at the first command run with the new version.
Read the [changelog](CHANGELOG.md) first: in `0.y.z`, a minor release may change
the interface.

- **`watchoo update`** (the simplest for an install done through the script or a release
  binary): the command shows the installed and latest versions, asks for confirmation, downloads
  the release archive, verifies its SHA-256 checksum and replaces the binary in one piece;
  `watchoo update --check` only checks. If the binary's directory is not writable
  (an installation in `/usr/local/bin`, for example), run it again with the required rights or use the
  install script below. Every command also reports, on the error output, that a
  new version is available; set `WATCHOO_NO_UPDATE_CHECK=1` to turn this check off.
- **Install script**: run the same command again (with the same `WATCHOO_INSTALL_DIR` if you had
  chosen one). The binary is replaced in one piece; a `watchoo serve` already running keeps the old
  version until it is restarted.
- **Release binaries**: download and verify the new archive as above, then replace the binary.
- **Docker** (`watchoo update` is refused there, and the new-version check is turned off): change the image tag (`ghcr.io/studoo-app/watchoo:<version>`), then recreate the
  container (`docker rm -f watchoo` then the same `docker run -d …`); with compose, change the tag in
  `docker-compose.yml` then `docker compose pull && docker compose up -d`. The volume is kept.
- **From source**: `go install github.com/studoo-app/watchoo/cmd/watchoo@latest`, or
  `git pull && make build` in the clone (`watchoo update` refuses to replace a development
  version).

Then check with `watchoo version`. Migrations only go one way: an upgraded
database may no longer open with an older version. To be able to go back, copy
`.watchoo/watchoo.db` (or the Docker volume) before upgrading.

## Quick start: following a class of students

```bash
mkdir licence-projet && cd licence-projet
watchoo init                              # crée l'espace de supervision (.watchoo/) ; sans gh : clones Git
watchoo repo add alice/projet-licence --label "Alice Martin"
watchoo repo import promo-licence.csv     # ou tous les dépôts d'un coup
watchoo sync                              # récupère les commits
watchoo repo list                         # qui est Actif, en Flemme ou Dead
watchoo log --since 7d                    # les commits de la semaine
watchoo contributors                      # qui contribue, et combien
watchoo prs --state open                  # les pull requests ouvertes
watchoo releases                          # releases et tags
watchoo tui                               # naviguer au clavier
watchoo serve                             # tableau de bord sur http://127.0.0.1:8080/
```

The import file is a CSV (only the `repo` column is required) or a plain list, one repository
per line:

```csv
repo,label,branches
alice/projet-licence,Alice Martin,
bob/projet-licence,Bob Durand,"main,dev"
my-org/group-project-3,Group 3,
```

```
REPOSITORY             LAST COMMIT        AUTHOR      STATE
Alice Martin           2 h ago            alice       -
Bob Durand             9 d ago            bob         ~ 9 d
Group 3                42 d ago           dan         ⚠ 42 d
```

Every directory initialized with `watchoo init` is an independent group: a class, a module, your
own projects… Data never mixes. Every command that produces data accepts `--json`
([schema](documentation/en/05-cli.md#json-output)).

Supervising with others? `watchoo config export -o licence.json` writes the tracked repositories and the
shared settings to a JSON file (no commits, e-mails or token); on the colleague's machine, `watchoo
config import licence.json` shows a preview then merges, without removing anything. Same on the Settings
page of `watchoo serve` ([details](documentation/en/05-cli.md#configuration-sharing-watchoo-config)).

## Configuration

Every workspace has its `.watchoo/config.yaml` file, commented (template:
[`config.example.yaml`](config.example.yaml)). Priority order, from lowest to highest: default
values, user settings (`~/.config/watchoo/config.yaml`), workspace file, environment
variables `WATCHOO_<SECTION>_<KEY>` (e.g. `WATCHOO_SYNC_INTERVAL=1h`), then command-line
options.

| Setting | Default | Purpose |
| --- | --- | --- |
| `sync.interval` | `30m` | Automatic sync of `serve` and `tui` (15 min minimum, `0`: manual) |
| `activity.flemme_after_days` / `dead_after_days` | `5` / `15` | Activity thresholds of the workspace (per repository: `repo add --flemme / --dead`, import columns, repository page) |
| `branches.exclude` | `[]` | Ignored branches (e.g. `dependabot/*`) |
| `timezone` | system timezone | Display timezone and day/week grouping |
| `web.addr` | `127.0.0.1:8080` | Web interface address |
| `web.auth.username` | empty | Basic authentication (password in `WATCHOO_WEB_PASSWORD`) |
| `github.user` | active `gh` account | `gh` account to use (empty with `GH_TOKEN`) |
| `github.gh_path` | `gh` | Name looked up in the `PATH` or absolute path of `gh` (no relative path) |
| `git.protocol` | `ssh` | Without `gh`: clones over SSH (`git@github.com:…`) or `https` (Git's credential helper) |

`watchoo -C <directory>` or the `WATCHOO_DIR` variable select the workspace without moving into it
([Workspaces](documentation/en/10-workspaces.md)). On Linux and macOS, Watchoo
refuses a `.watchoo/` that is not yours or whose permissions exceed `0700` (`config.yaml`: `0600`):
a `.watchoo/` committed into a student's repository you cloned is never used.

## Security and privacy

- **Token**: obtained only through `gh auth token` (or `GH_TOKEN` read by `gh`), never stored, never
  shown in logs, errors or outputs. Without `gh`, Watchoo reads and stores no
  credential: `git` uses your SSH key or your credential helper, with no prompt.
- **Read-only**: Watchoo refuses any GitHub call that would change something, before it is
  even sent; on the `git` side, it only runs `git fetch`, reads and the update of its own
  clones, never `push`.
- **Local data**: everything stays in `.watchoo/` (permissions `0700`, files `0600`), one database per workspace.
  It often comes from private repositories and holds students' personal data: never
  commit `.watchoo/` (a `.gitignore` is created), never share the database or the Docker volume. No
  commit message nor e-mail in the logs.
- **Web interface**: listens on `127.0.0.1` by default; on any other address, `serve` refuses to
  start without authentication (basic or trusted reverse proxy). CSRF protection, `Host` header
  check, strict CSP.

Details: [Architecture](documentation/en/03-architecture.md) and
[Web interface](documentation/en/06-web-interface.md#security).

## Learn more

- [Full documentation](documentation/en/README.md): vision, features, architecture, roadmap.
- [Changelog](CHANGELOG.md).
- [AGENTS.md](AGENTS.md): instructions for the coding agents working on the project.

Watchoo is written in Go: a single binary, with no dependency to install (except `gh`, or `git`).

## License

Watchoo is distributed under the [Apache 2.0 license](LICENSE). The name "Watchoo" and the owl logo are not
part of it: a modified and redistributed version must carry another name (see [NOTICE](NOTICE)).

> 🇫🇷 [Version française](README.fr.md)
