# Changelog

All notable changes to Watchoo are recorded in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to
[semantic versioning](https://semver.org/spec/v2.0.0.html). While Watchoo is `0.y.z`,
an incompatible change bumps MINOR and any compatible change bumps PATCH (see
[Skills](documentation/en/11-skills.md#versions-and-changelogmd)).

## [Unreleased]

### Added

- **Bilingual interface**: the CLI, TUI and web interface are available in English (default) and
  French; the CLI/TUI language comes from `WATCHOO_LANG` or the system locale, the web language from
  the workspace's shared `lang` setting; no new dependency (the `internal/i18n` package).
- **Bilingual documentation**: the design documentation is now available in English and French,
  reorganized into mirror trees `documentation/en/` and `documentation/fr/` with the same numbering
  and a bilingual index ([INDEX.md](documentation/INDEX.md)); the root `README.md` is English with a
  `README.fr.md` French version; `AGENTS.md` is English. Any change to a document is mirrored in the
  other language in the same commit.

## [0.6.2] - 2026-10-27

New-version notice restored for `watchoo` alone and in `watchoo tui`, and no longer lost after
a long-running command.

### Fixed

- **New-version notice**: it now also shows for `watchoo` alone (after the card and
  the help) and in `watchoo tui`, on the line under the header ("Quittez puis lancez « watchoo update »." ("Quit then run "watchoo update".")); it is no longer lost, one time out of two, after a command longer than 1.5 s (`sync`).

## [0.6.1] - 2026-10-27

Adding a repository from the TUI (`a` key), the "GitHub" link restored for contributors known
before 0.5.0, and a new-version check every 30 min in `watchoo serve`.

### Added

- **Adding a repository from the interactive mode**: in `watchoo tui`, the `a` key (Repositories view) opens
  an input "Dépôt à suivre (owner/nom ou URL GitHub) :" ("Repository to track (owner/name or GitHub URL):"); `enter` confirms, `esc` cancels. Same
  references, same checks and same messages as `watchoo repo add` (all branches, no
  label nor own thresholds). Once the repository is added, the list is read again, the repository is selected and its
  sync started; during a sync, the new repository's sync starts at the end of the running sync.

### Changed

- `watchoo serve` checks whether a new version of Watchoo exists every 30 min (instead of
  every 24 h): the web interface banner appears shortly after a release is published.

### Fixed

- **Missing "GitHub" link** for contributors known before 0.5.0 (login recorded, profile address
  empty, because commits already in the database are not read again): the address is now filled in
  from the login at the next sync, through the API as well as through the local Git clones, with no extra
  request.

## [0.6.0] - 2026-10-27

New-version notice (CLI and web interface) and one-command Watchoo update,
`watchoo update`.

### Added

- **New-version notice**: at every command, Watchoo checks in the background (1.5 s at most, never
  delaying the command any further) whether a newer version is released, and announces it at the end
  on the error output. Nothing with `--json`, outside a terminal, for a development version, nor if the
  `WATCHOO_NO_UPDATE_CHECK` variable is set (this is the case in the Docker image). `watchoo serve`
  checks at startup then every 24 h, and the web interface then shows a discreet banner on
  every page, with the link to the release.
- **`watchoo update` command**: shows `v0.5.0 → v0.6.0` and the link to the release, asks
  for confirmation (`--yes` / `-y` to skip it), downloads the release archive for the system,
  verifies its SHA-256 checksum and replaces the binary in one piece. `watchoo update --check` only checks
  (`--json`: `current`, `latest`, `update_available`, `release_url`). Refused with an explanation in the Docker
  image (change the tag), for a version built from source and when the binary's directory
  is not writable. Only the public pages of Watchoo's releases are read: no
  token, no GitHub API, no quota.
- Docker image: `WATCHOO_NO_UPDATE_CHECK=1` and `WATCHOO_DOCKER=1` variables.

## [0.5.0] - 2026-10-27

Per-contributor page with a link to their GitHub profile, and lines added and removed of all
commits, computed from a Git clone of every repository.

### Added

- **Per-contributor page**: the web interface gains the `/contributors/{id}` page: name, login, avatar and
  number of known Git identities (never the addresses), totals (commits, lines added and removed,
  "—" if unknown, repositories, first and last commit), commits per day over 90 days, commits per
  week and per repository over 12 weeks (collapsible data table), their repositories (commits, lines,
  last commit, repository state), their paginated commits (50 per page) and their pull requests, matched through
  their GitHub account when it is known. A contributor's name leads there from the contributors
  ranking, the tables of the per-contributor charts, the contributors of a repository's page,
  the author of the commits in the histories and on the dashboard. In the TUI, `enter` on the
  Contributors view opens a contributor's detail (totals, `+/-` lines, curve over 12 weeks, their
  repositories, their latest commits); `esc` goes back to the list.
- **Link to the GitHub profile** of contributors whose account is known, wherever the web interface
  shows one (secondary "GitHub" link, new tab). The address is the one the GitHub API gives,
  stored in the new `contributors.profile_url` column (migration `00005`, applied
  automatically); for the contributors already in the database, it arrives with their next commit read by
  the API. No link for a contributor known only through Git (local Git clones).

### Changed

- **Always clone**: with `gh` too, every tracked repository now has its local Git clone in
  `repos/` (created at the first sync, updated at each one). The API remains the source of commits,
  metadata, logins, pull requests, releases and tags; the clone provides the **lines
  added and removed of all commits**, first import included, with no cap and no quota
  (`git diff-tree --numstat` in batches). The clone uses the machine's Git credentials
  (`git.protocol`), never the `gh` token; over HTTPS, `gh auth setup-git` lets `git`
  reuse the `gh` account. If a repository's clone is unavailable, the repository's sync still
  succeeds: its statistics are requested from the API as before (at most 100 new commits per
  sync), with a warning in the summary (`watchoo sync`, TUI, web sync indicator).
  `repos/` takes up roughly the size of the tracked repositories, and a repository's first sync is slower
  (full clone). Without `gh`, nothing changes, except that the statistics cover all
  commits.
- **Unknown or zero statistics**: the new `commits.stats_known` column (migration `00006`,
  applied automatically) distinguishes a commit whose lines are unknown ("—") from a commit
  actually at `+0 −0` (merge without changes, empty commit), which now displays its zeros. The
  commits already in the database without statistics are computed by the clone at the next sync. `watchoo log
  --json` (and the last commit of `repo list --json`) gains `stats_known` for each commit.
- **Lines per commit and totals everywhere**: every commit list shows the commit's `+/-` lines
  ("—" if unknown): dashboard feed, repository history, a contributor's commits, the
  Commits view and details of the TUI, the `+/-` column of `watchoo log`. Totals are given per contributor,
  per repository (dashboard card, header and contributors of a repository's page, TUI detail)
  and for the whole workspace (the dashboard's "Lines" counter), with "(dont N commits non comptés)" ("including N uncounted commits")
  when some commits do not have statistics yet.
- The web sync indicator counts the **warnings** of the last summary (details in a tooltip).

## [0.4.0] - 2026-10-27

V2: local Git repositories without `gh`, pull requests, releases and tags, per-repository activity
thresholds and per-contributor activity within a repository. End of the MVP.

### Added

- **Local Git repositories**: `gh` becomes optional. Without `gh` installed and signed in, Watchoo tracks
  repositories through Git clones **with working copy** in `repos/<name>`, at the workspace root (name clashes:
  `repos/<owner>-<name>`), updated at every sync by `git fetch` then reset to the default
  branch (a local change to a tracked file is overwritten, an untracked file kept; no hook,
  no submodule, no LFS download), with the machine's SSH key or Git credentials, without any prompt (`git` must be
  installed): commits, branches, statistics and tags, without quota, but without pull requests, releases,
  stars or GitHub login (contributors merged by e-mail). The source is chosen at every sync,
  based on `gh` only: signing in (`gh auth login`) switches back to the API, which then completes
  the identity and the metadata of the repositories added without `gh`. New `git.protocol` setting
  (`ssh` by default, or `https`), specific to the machine and never shared. `watchoo init` without `gh` creates
  the workspace and says so instead of stopping (it stops if `git` is missing too); `repo add` and
  `repo import` check access with `git ls-remote` ("accès refusé" ("access denied") otherwise); `repo remove` deletes
  the clone, `--keep-history` keeps it. Run from a clone, a command ignores a `.watchoo/` committed
  by the student and works on the workspace containing `repos/`. `watchoo status`, the `watchoo sync` summary and the Settings page
  (quota "sans objet" ("not applicable")) show the source; the `--json` outputs of `init`, `sync` and `status`
  gain the `source` key. A repository's page and the TUI detail hide stars, issues, pull requests
  and releases of a repository never read by the API. The database makes `repositories.provider_id` optional
  (migration `00004`, applied automatically, without data loss). The Docker image now contains
  `git` and `ssh`: it switches from a distroless base to a minimal Alpine system installed for
  the target architecture (still without emulation, non-root user).
- **Pull requests and releases**: every sync also reads the pull requests (incrementally:
  only those modified since the previous sync), the releases and the tags of every repository;
  when nothing changed, GitHub answers `304` and these reads cost nothing. New commands
  `watchoo prs [--repo X] [--state open|merged|closed|all] [-n 20]` and `watchoo releases [--repo X]
  [-n 20]` (releases then tags, with the date of the targeted commit), in text and `--json` (keys
  `pull_requests`, `releases` and `tags`). A repository's page of the web interface gains the "Pull
  requests" (open, latest merged, links to GitHub) and "Releases et tags" ("Releases and tags") cards; the dashboard
  shows each repository's open PRs, and the TUI repository detail its open PRs and its
  latest release. A failure on these lists does not fail the commit sync: it becomes a
  warning (DÉTAIL column of `watchoo sync`, `warnings` key of each repository in `--json`, TUI
  summary). The database goes to 10 tables (migration `00003`, applied automatically).

- **Per-repository activity thresholds**: a repository can have its own Flemme and Dead thresholds instead of
  the workspace's (`activity.*`). `watchoo repo add owner/repo --flemme 3 --dead 10` (`0` falls back to
  the workspace threshold), optional `flemme` and `dead` columns of the `watchoo repo import` file
  (empty cell = threshold kept), `flemme_after_days` and `dead_after_days` keys of repositories in
  configuration sharing (written only for a repository that has them), and a "Seuils
  d'activité de ce dépôt" ("This repository's activity thresholds") form on a repository's page of the web interface (empty field = workspace threshold).
  The page header and the TUI detail say whether the applied thresholds are the repository's own or the
  workspace's. The `--json` outputs gain the `flemme_after_days` and `dead_after_days` keys (`null` =
  workspace threshold) in the repository object and in the `config import` preview.
- **Per-contributor activity within a repository**: a repository's page of the web interface shows the
  commits per week over 12 weeks of its 8 most active contributors (stacked bars, the
  others grouped, collapsible data table), like the contributors page but for this single
  repository (all tracked branches, each commit once). The TUI repository detail gives, for
  its main contributors, their commits over 12 weeks and a week-by-week curve.
  `watchoo contributors --weeks N` (with or without `--repo`) limits the ranking to the last N weeks
  and shows each person's commits week by week; the `--json` output gains the `weekly` key
  (`week` / `count` objects, `[]` without `--weeks`) in each contributor.

## [0.2.0] - 2026-10-27

Configuration sharing between supervisors, install script and new logo.

### Added

- **Install script** `install.sh` (Linux, macOS): `curl -fsSL
  https://raw.githubusercontent.com/studoo-app/watchoo/main/install.sh | sh` downloads the latest
  release, verifies its SHA-256 checksum and installs `watchoo` into `~/.local/bin`, without sudo
  (`WATCHOO_VERSION`, `WATCHOO_INSTALL_DIR`).
- **Configuration sharing** between supervisors of the same group: `watchoo config export [-o
  file] [--force]` writes a versioned JSON file (`format: watchoo-config`, `version: 1`) with the
  tracked repositories (display label, branches) and the shared settings (`sync.*`, `activity.*`,
  `branches.exclude`, `timezone`), never `github.*`, `web.*`, token, e-mail or commit;
  `watchoo config import file [--dry-run] [--yes]` fully validates it, shows a preview, then
  merges without removing anything (repositories added or updated as with `repo import`, `config.yaml`
  rewritten from the commented template with a `config.yaml.bak` copy). Same feature on the Settings
  page of the web interface (export, preview, apply).
- **README**: "Upgrading" section (script, binaries, Docker, sources; automatic migrations,
  backing up the database before changing version).

### Changed

- Web interface: a form whose body exceeds the limit now gets `413` (body too large) instead of a
  misleading `403`; nothing is applied in either case.
- **New logo**: pixelated owl with magenta eyes and green pupils, with the "WATCHOO" name
  (`assets/logo/watchoo-logo-final*.svg`, dark-background variants). The web interface header
  shows the owl and the name, in a light or dark version depending on the theme; favicon, manifest
  and iOS home-screen icons redone on a white background; README updated. The text versions of the owl
  (CLI, TUI, `config.yaml`) do not change.

## [0.1.0] - 2026-10-27

First published release: the MVP (phases 1 to 5 of the [roadmap](documentation/en/07-roadmap.md)).

### Added

- **Workspaces**: `watchoo init` turns a directory into an independent workspace
  (`.watchoo/` at `0700`, config and SQLite database at `0600`, `.gitignore`) after checking `gh`;
  workspace discovery in the parent directories, `-C` option and `WATCHOO_DIR` variable; settings
  in three levels (user, workspace, `WATCHOO_<SECTION>_<KEY>` variables); `config.example.yaml`.
- **SQLite database** without CGO (`modernc.org/sqlite`), embedded goose migrations, sqlc queries, dates
  stored in UTC in a fixed format.
- **GitHub access** through the `gh` CLI only (token from `gh auth token`, account chosen by
  `github.user`, `GH_TOKEN` case for Docker and CI), and GitHub provider (go-github): pagination,
  stop at the first known commit, comparisons for the incremental sync, ETag cache, quota tracking, empty,
  renamed or transferred repositories, SSO.
- **Incremental sync engine**: all tracked branches (`--branch` patterns, excluded
  branches), no commit ever re-downloaded, bounded statistics, per-sync lock, quota guard,
  at most 4 repositories in parallel.
- **Commands** `repo add`, `repo import` (CSV or list file, access check, `--dry-run`,
  summary), `repo list`, `repo remove`, `sync`, `log`, `contributors`, `status` and `version`.
- **Activity states** of every repository: Actif, Flemme (more than 5 days without a commit), Dead (more
  than 15 days), thresholds adjustable per workspace (not yet per repository), and gray states (never
  synced, error, archived, tracking stopped, empty).
- **Interactive mode** `watchoo tui` (Bubble Tea): repositories, commits, contributors, filter and sync from
  the keyboard.
- **Local web interface** `watchoo serve` (HTMX, DaisyUI, Chart.js embedded): dashboard, repository
  detail (paginated history, branch and author filters, 90-day activity),
  contributors, settings (repository addition and removal, display label, interval, quota); periodic
  background sync and on-demand sync with progress.
- **Activity charts**: 90-day daily commits of the whole workspace on the dashboard, and
  weekly commits over 12 weeks of the 8 most active contributors on the contributors page,
  computed in the display timezone, readable in light and dark theme.
- **`--json` output** on every command that produces data, with a stable schema
  documented in [05-cli.md](documentation/en/05-cli.md#json-output) and verified by tests.
- `watchoo version` and `watchoo version --json` give the version, the commit and its date, the Go
  version, the system and the architecture.
- **Header card** with the owl in the terminal (only on a terminal, never with `--json`),
  logo and icons of the web interface.
- **Distribution**: Linux, macOS and Windows binaries in amd64 and arm64 (GoReleaser), archives with
  SHA-256 checksums; Docker image `ghcr.io/studoo-app/watchoo` (linux/amd64 and linux/arm64) with `gh`;
  `docker-compose.yml` example; `make release-check`, `make release-snapshot` and `make docker` targets.
- Full README, design documentation in `documentation/`, Apache 2.0 license.

### Security

- The GitHub token is only ever read from `gh`, never stored, and never appears in any
  log, error or output.
- Read-only: the provider transport refuses any GitHub call other than `GET` or `HEAD` before
  sending, and sends the token only to the API origin.
- Only the user's workspaces are accepted (Linux, macOS): a `.watchoo/` that does not
  belong to them or whose permissions exceed `0700` (`config.yaml`: `0600`), like the one a student
  would commit into their cloned repository, is refused before any read of its settings;
  `github.gh_path` must be a name looked up in the `PATH` or an absolute path.
- No commit message nor e-mail in the logs; texts from the repositories cleaned of terminal escape
  sequences and escaped on the web.
- Web interface: listens on `127.0.0.1` by default, authentication mandatory on any other
  address (basic authentication with `WATCHOO_WEB_PASSWORD`, or trusted reverse proxy with
  `web.allowed_hosts`), CSRF protection (cookie set only with the HTML pages), `Host` header
  check against DNS rebinding, strict CSP with no inline script, security headers.
- Docker image on a distroless base, non-root user (uid 65532), no token in the image;
  `gh` is the official binary verified by SHA-256; base images and GitHub actions pinned by
  digest.

[Unreleased]: https://github.com/studoo-app/watchoo/compare/v0.6.2...HEAD
[0.6.2]: https://github.com/studoo-app/watchoo/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/studoo-app/watchoo/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/studoo-app/watchoo/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/studoo-app/watchoo/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/studoo-app/watchoo/compare/v0.2.0...v0.4.0
[0.2.0]: https://github.com/studoo-app/watchoo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/studoo-app/watchoo/releases/tag/v0.1.0
