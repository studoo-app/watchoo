# Changelog

All notable changes to Watchoo are recorded in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to
[semantic versioning](https://semver.org/spec/v2.0.0.html). Since `1.0.0`, the public surface
is stable: an incompatible change bumps MAJOR, a compatible feature MINOR and a compatible fix
PATCH (see [Skills](documentation/en/11-skills.md#versions-and-changelogmd)).

## [Unreleased]

### Added

- Web interface: a **Documentation** link in the footer of every page, to the contents of the online
  documentation in the page's language (`documentation/en/` in English, `documentation/fr/` in French).

## [1.6.0] - 2026-11-04

EN / FR language selector in the web interface; in the diagnosis, the lines behind each signal on the
review sheet, and a score on the repository page and in the contributors list. No incompatible change: no
migration, no setting changed; the new `POST /lang` route and the `watchoo_lang` cookie are additions.

### Added

- Web interface: EN / FR language selector in the header. The choice applies to the current browser
  only (cookie `watchoo_lang`, `POST /lang`), takes precedence over the workspace's `lang` setting and
  never changes `config.yaml`; without a choice, the workspace's language applies (English by default).
- **Diagnosis, phase 7.8 (lines behind the signals)**: the review sheet of a commit lead quotes, under each
  signal, the added lines that triggered it, with their file and line number (comments for F1, documented
  functions for F2, test files for F5, decorative lines for A5, comments without accents for A6, TODO and
  debug calls for H1, one-letter variables for H2; 20 per signal at most). They are read from the local clone
  when the sheet is shown, like the diff, and never stored or logged.
- **Diagnosis on the repository page** (`/repos/{id}`, only when the diagnosis is enabled): a "Diagnosis"
  card with the score of the repository (sum of the points of the signals of its leads: D = 10, R and
  A1-A4 = 3, F and A5-A7 = 1, H = 0), always next to the number of leads per level and the table of the
  triggered signals (leads, weight, points), the 10 latest leads linked to their review sheet and a link
  to all the leads of the repository. Refreshed at the end of a sync (`repo-diagnostic` fragment).

### Changed

- **Contributors list**: the "AI signatures" column is replaced by the **AI score** of each contributor (same
  points, on the leads they authored across all repositories), with their number of leads per level below
  it; "—" when the diagnosis is disabled. The list stays sorted by commits. The AI signatures stay on a
  contributor's page and on the commits.

## [1.5.0] - 2026-11-04

Absolute signals and traces of human work in the diagnosis, for projects delivered in few commits. No
incompatible change: the database is migrated at the first command (migration `00015`); then run
`watchoo diagnostic backfill` (or "Measure the history") so that the commits already measured get the new
measures.

### Added

- **Diagnosis, phase 7.7 (absolute signals and counter-indicators)**: leads for projects delivered in few
  commits, which the rhythm and form signals (compared with the learner's own history) could not see.
  Absolute signals: massive commit (A1, 500 source lines or more), repository delivered in one go (A2), push
  after 14 days of silence (A3), revert then replacement within the hour (A4), decorative comments (A5),
  French comments without accents (A6), code in its final state with no work-in-progress marker nor
  one-letter variable (A7). A1 to A4 count as rhythm signals, A5 to A7 as form signals. Traces of human work
  (H1 TODO/FIXME/debug, H2 one-letter variables, H3 initial skeleton of files, H4 iterative fixes) lower a
  "probable" lead to "to review" and never remove it. New measures in `commit_signals` (migration `00015`),
  numbers only; the forms measured before are measured again by `watchoo diagnostic backfill`. Thresholds
  from a teachers' reference, not calibrated yet.

## [1.4.0] - 2026-11-04

Supervision and diagnosis: reflection leads on AI-assisted code production, for the trainer only, off by
default (`diagnostic.enabled`). No incompatible change: the database is migrated at the first command
(migrations `00011` to `00014`); enable the diagnosis after informing the learners, then run
`watchoo diagnostic backfill` to measure the history already cloned.

### Changed

- **Line statistics from local clones detect renames** (`git diff-tree -M`), like GitHub's: a moved file
  counts only its changed lines, for the `+/-` lines as for the languages of a contributor. Statistics
  already stored are not recomputed.

### Fixed

- **Web header**: on medium-width screens, a long workspace name no longer runs under the navigation menu;
  it is cut with an ellipsis.

### Added

- **Diagnosis, phase 7.1 (foundations)**: with the new `diagnostic.enabled` setting (disabled by
  default), each sync measures, for the new commits read from the local clones, the data of the
  diagnosis signals: lines and files after exclusion (`diagnostic.exclude`, lock files, vendored and
  generated files by default; binaries always left out), AI tools declared in the commit message
  (`Co-authored-by:` trailers and signatures), bot author, agent configuration file. Merge commits
  are left out. The values go into the new `commit_signals` table (migration `00011`), without message,
  e-mail address or file path, and are purged after `diagnostic.retention_days` (365 by default). Nothing
  is displayed yet: a signal is never a verdict, and the review comes in the next phases. Settings:
  `WATCHOO_DIAGNOSTIC_ENABLED`, `WATCHOO_DIAGNOSTIC_EXCLUDE`, `WATCHOO_DIAGNOSTIC_RETENTION_DAYS`; never
  shared by `watchoo config`. See
  [Supervision and diagnosis](documentation/en/12-supervision-diagnostic.md).
- **Diagnosis, phase 7.2 (declarative signals)**: new `watchoo diagnostic [--repo X] [--author Y]
  [--since 30d]` command (text and `--json`), for the trainer who supervises the workspace: the reflection
  leads of level "declared", commits and pull requests that declare the use of an AI tool, newest first,
  with their signals and raw values (D1 co-author trailer, D2 signature in the commit message or the PR
  description, D3 bot author, D5 PR branch named by an agent), and the date an agent configuration file was
  first added to each repository (D4, context only). A lead is never a verdict. Each declared tool is kept
  with the signal that found it (`D1:claude`); for pull requests, only the tools found in the description
  are kept, never the description (`pull_requests.tool_ids`, new `repository_signals` table, migration
  `00012`). With the diagnosis disabled, the command says so.
- **Diagnosis, phase 7.3 (rhythm signals)**: `watchoo diagnostic` also shows leads of level "to review":
  commits or days whose rhythm is unusual for the learner, compared with their own history in the
  repository over 90 days (modified z-score above 3.5): net lines of a day (R1), commit size (R2),
  addition ratio (R3), files per commit (R5), regime break (R6), and apparent speed (R4) with the new
  `diagnostic.speed_signal` setting (off by default). Each signal shows its raw value, the personal median
  and the z-score (`raw`, `median`, `z` in `--json`); authors with fewer than 20 measured commits in 90
  days are reported as "not enough history". New `watchoo diagnostic backfill [--repo X]` command: measures
  on the local clones the commits synced before the diagnosis was enabled, within the retention period.
- **Diagnosis, phase 7.4 (human review on the web)**: a "Diagnosis" page in `watchoo serve` (navigation
  entry only with the diagnosis enabled), for the trainer: the reflection leads with their review status,
  a filter by repository and a "Measure the history" button (backfill in the background). Each lead has a
  review sheet: signals with raw value, personal median and z-score, the author's throughput over 90 days
  with the flagged day, neighbouring commits and the diff read from the local clone (never stored), and a
  review form (decision, cause, confidence, note). Reviews go into the new `diagnostic_reviews` table
  (migration `00013`) and follow `diagnostic.retention_days`. New routes: `GET /diagnostic`,
  `GET /diagnostic/lead`, `POST /diagnostic/review`, `POST /diagnostic/backfill`.
- **Diagnosis, phase 7.5 (form signals)**: the added lines of each measured commit are read from the local
  clone and analysed in memory; only numbers are kept (11 new columns of `commit_signals`, migration
  `00014`), never code, identifiers, paths or messages. Five form signals, each learner being compared with
  their own history: comment density (F1), systematically documented functions (F2), lexical diversity of
  the identifiers, MATTR (F3), message style break (F4), tests delivered with the code when unusual (F5). New
  level `probable` (at least two rhythm signals and one form signal); two form signals give `to review`,
  one alone raises nothing. `watchoo diagnostic backfill` also completes the form of the commits measured
  before. The content classifier (F6) is postponed.
## [1.3.1] - 2026-11-03

Dedicated message for an unknown SSH host key with local Git clones. No incompatible change.

### Fixed

- **Unknown SSH host key** (local Git clones): "Host key verification failed" no longer gives the
  misleading "check the SSH key" advice. The message now says that the server fingerprint is unknown or
  changed and to run `ssh -T git@github.com` once to accept it (for example when the SSH configuration
  sends `github.com` to `ssh.github.com` port 443). The repository is still treated as inaccessible, as
  before. On a repository's page, the advice under a local Git clone warning now points to the cause
  shown above.

## [1.3.0] - 2026-11-03

Sync warnings of each repository in the web interface. No incompatible change: the database is migrated
at the first command; the warnings appear from the next sync.

### Added

- **Sync warnings per repository** (web): an orange badge with the number of warnings on the dashboard
  card (local Git clone unavailable, line statistics or languages not computed, pull requests, releases
  or tags not read, AI signatures not analyzed), with its consequence under the last commit; a repository
  in error shows the first line of its error. On the repository's page, an alert gives the full message
  of each warning of the last sync (or of the failed sync) and, for the local Git clone, what to check.
  The warnings are recorded with the sync, so they survive a restart of `watchoo serve`.
- **Database**: migration `00010` (`sync_runs.warnings`).

### Changed

- **Sync indicator** (web): the warnings badge opens a menu of the repositories concerned, each linked to
  its page, instead of a truncated tooltip; its short labels follow the language of the interface.

## [1.2.0] - 2026-11-03

AI signatures declared in the commits, and languages of the contributors. No incompatible change: the
database is migrated at the first command, and the existing commits are analyzed at the next sync, without
any request to the forge.

### Added

- **AI signatures**: `watchoo log`, `watchoo contributors`, the TUI and the web interface show the AI
  tools **declared** in the commits (co-author line, footer, agent account) for Aider, Claude, Codex,
  GitHub Copilot and Cursor. A signature says a tool was declared, not who wrote the code, and its absence
  proves nothing: no score, no percentage, never "human". Existing commits are analyzed at the next
  sync, without any request to the forge.
- **Languages of a contributor**: lines added in each language by the commits of a contributor, with their
  percentage, read from the local Git clone in the same pass as the `+/-` lines (no extra request). New
  "Languages" card on a contributor's page and main language of each of their repositories, LANGUAGE
  column of `watchoo contributors`, "Languages" line in the contributor detail of the TUI. Merge commits,
  dependencies and generated files do not count. Existing commits are read again from the clone at the
  next sync; their `+/-` lines do not change.
- **`--json`**: `ai_signatures` and `ai_scanned` on commits; `ai_signed_commits`, `ai_scanned_commits`,
  `ai_tools`, `languages_scanned_commits` and `languages` on `contributors`.
- **Database**: migration `00008` (`commits.ai_rules_version`, table `commit_ai_signatures`) and migration
  `00009` (`commits.languages_version`, table `commit_languages`).

### Fixed

- **Release notes**: the footer of the GitHub release (changelog link, Docker image) is in English.

## [1.1.0] - 2026-11-02

Languages and share of the contributors for each tracked repository. No incompatible change: the
database is migrated at the first command, and the languages of a repository appear at its next sync.

### Added

- **Languages of a repository**: languages of the default branch with their percentage (share of the
  bytes), computed at each sync from the local Git clone, with or without `gh` and without any quota.
  New `watchoo languages [--repo X]` command (text and `--json`), "Languages" card on a repository's
  page, "Languages" line in the repository detail of the TUI. Dependencies, generated files, lockfiles,
  data and prose are ignored; the figures are close to those of GitHub, not identical.
- **Share of the contributors**: share of the commits of each contributor as a percentage, in
  `watchoo contributors` (SHARE column, `share` key in JSON), on a repository's page and in the
  repository detail of the TUI.
- **Database**: `repository_languages` table (migration `00007`).

## [1.0.0] - 2026-11-01

First stable release: after one more week of real usage, the public surface (commands, options,
exit codes, `--json` keys, `config.yaml` settings and `WATCHOO_*` variables, database schema, web
routes) is reviewed, documented and declared stable. From now on, an incompatible change requires a
new major version. No incompatible change compared with 0.7.0: workspaces, databases and scripts
keep working as they are.

### Changed

- **Versioning**: SemVer applies fully: incompatible change → MAJOR, compatible feature → MINOR,
  compatible fix → PATCH; what is to be removed is first marked deprecated.
- **"Slacking" in English**: `watchoo status` (text), the help of `repo add --flemme` and the comment
  of `config.yaml` now say "Slacking" in English, like the rest of the interface; `flemme` remains the
  machine name (state, option, column, setting, JSON keys).
- **Dates in English**: the text outputs of `watchoo sync` (resume time), `watchoo status` (last sync)
  and `watchoo contributors --weeks` (columns) write the month before the day in English
  (`10/27/2026`, `10/12`); unchanged in French.
- **Documentation of the public surface**: exit codes (`0` success, `1` any failure, partial
  failures included), the complete list of the `WATCHOO_*` variables, what the JSON contract covers
  (compatible additions, texts in the selected language, local IDs, `flemme`), the values of `status`
  in `sync --json`, and the complete list of the web routes; the test of the JSON contract now
  checks the English documentation as well as the French one.

### Removed

- From the documentation only (never built): `watchoo repo add-org` (planned for V3) and the CSV import
  in the web interface (use `watchoo repo import`).

## [0.7.0] - 2026-11-01

Bilingual interface: English by default, French on demand, for the CLI, the TUI, the web interface
and the install script; bilingual documentation.

### Added

- **Bilingual interface**: the CLI, TUI and web interface are available in English (default) and
  French; the CLI/TUI language comes from `WATCHOO_LANG` or the system locale, the web language from
  the workspace's new `lang` setting (`en` by default, shared through `watchoo config export /
  import`); no new dependency (the `internal/i18n` package). The `config.yaml` written by
  `watchoo init` and `config.example.yaml` are now commented in English (in French when the language
  is French).
- **Bilingual install script**: `install.sh` speaks English by default, French with `WATCHOO_LANG=fr`
  or a French system locale.
- **Bilingual documentation**: the design documentation is now available in English and French,
  reorganized into mirror trees `documentation/en/` and `documentation/fr/` with the same numbering
  and a bilingual index ([INDEX.md](documentation/INDEX.md)); the root `README.md` is English with a
  `README.fr.md` French version; `AGENTS.md` is English. Any change to a document is mirrored in the
  other language in the same commit.

### Changed

- **English by default**: messages, help, tables, dates ("2 h ago", "~ 9 d") and web pages are now
  in English unless French is selected (`WATCHOO_LANG=fr`, a French system locale, or `lang: fr` for
  the web). `--json` keys and state values do not change; message values follow the language.

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

[Unreleased]: https://github.com/studoo-app/watchoo/compare/v1.6.0...HEAD
[1.6.0]: https://github.com/studoo-app/watchoo/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/studoo-app/watchoo/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/studoo-app/watchoo/compare/v1.3.1...v1.4.0
[1.3.1]: https://github.com/studoo-app/watchoo/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/studoo-app/watchoo/compare/v1.2.0...v1.3.0
[1.2.0]: https://github.com/studoo-app/watchoo/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/studoo-app/watchoo/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/studoo-app/watchoo/compare/v0.7.0...v1.0.0
[0.7.0]: https://github.com/studoo-app/watchoo/compare/v0.6.2...v0.7.0
[0.6.2]: https://github.com/studoo-app/watchoo/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/studoo-app/watchoo/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/studoo-app/watchoo/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/studoo-app/watchoo/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/studoo-app/watchoo/compare/v0.2.0...v0.4.0
[0.2.0]: https://github.com/studoo-app/watchoo/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/studoo-app/watchoo/releases/tag/v0.1.0
