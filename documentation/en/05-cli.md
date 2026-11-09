# 5. CLI

[← Contents](README.md)

## Global options

| Option | Effect |
| --- | --- |
| `--json` | JSON output to make every command scriptable |
| `-C <directory>` / `--workspace <directory>` | Works on the workspace of this directory instead of the one found from the current directory |
| `--verbose` | Detailed logs (debug level) on the error output. Without it, the level comes from the `WATCHOO_LOG_LEVEL` variable (`debug`, `info`, `warn` by default, `error`). No short letter: `-v` would be confused with `--version` |

All commands except `init`, `version` and `update` work on a **workspace**: the `.watchoo/` directory found in the
current directory or its parents (see [Workspaces](10-workspaces.md)).

**Message language**: the CLI and the TUI speak **English by default**. The `WATCHOO_LANG`
variable (`en` or `fr`) picks the language; without it, the system locale decides (`LC_ALL`,
`LC_MESSAGES`, then `LANG`: `fr` prefix → French); failing that, English. The web interface
language is set separately, by the workspace's `lang` setting; since `WATCHOO_LANG` is also the
variable of that setting, setting it forces the language everywhere (see
[Web interface](06-web-interface.md#interface-language)). The same texts exist in both
languages, served by the `internal/i18n` package with no dependency (see
[Architecture](03-architecture.md#languages-and-translations)).

**Repository access**: with the GitHub CLI `gh` installed and signed in (`gh auth login`), Watchoo goes
through the GitHub API; it gets the token only through `gh auth token` and never stores it. Without `gh`
(absent or signed out), it tracks the repositories through **local Git repositories** (the `git` binary
must be installed), using the machine's SSH key or Git credentials, without pull requests, releases nor
stars. The choice is made again at each sync (see
[Architecture](03-architecture.md#authentication-gh-optional) and
[Local Git repositories](03-architecture.md#local-git-repositories-v2)).

## Commands

| Command | Effect | Version |
| --- | --- | --- |
| `watchoo init` | Makes the current directory a workspace: checks `gh` (without `gh`: `git`), then creates `.watchoo/` with the config and the SQLite database. Refuses if the workspace already exists | MVP (without `gh`: V2) |
| `watchoo version` | Prints the Watchoo version, the compiled commit and its date, the Go version and the platform (`watchoo --version` does the same). Works outside a workspace and without `gh` | MVP |
| `watchoo repo add owner/repo [--branch main,dev,release/*] [--label "Alice Martin"] [--flemme 3] [--dead 10] [--conventions-since YYYY-MM-DD]` | Adds a repository to watch (all its branches by default), with an optional display label, own activity thresholds and own conventions start date | MVP (thresholds: V2; conventions date: 1.7.0) |
| `watchoo repo import file.csv [--dry-run]` | Adds a list of repositories in one go (e.g. those of a class of students), checks access to each one and prints a summary | MVP |
| `watchoo repo list` | Table: repository, last commit, author, state (Active / Slacking / Dead) | MVP |
| `watchoo repo remove owner/repo [--keep-history] [--yes]` | Stops tracking and deletes the repository history (and its local Git clone) after confirmation (`--keep-history` to keep them) | MVP |
| `watchoo sync [owner/repo]` | Syncs everything or a single repository | MVP |
| `watchoo log [--repo X] [--author Y] [--branch B] [--since 7d] [-n 20]` | Latest commits, filtered | MVP |
| `watchoo contributors [--repo X] [--since 30d \| --weeks 12]` | Contributor ranking; with `--weeks`, their commits week by week | MVP (`--weeks`: V2) |
| `watchoo prs [--repo X] [--state open\|merged\|closed\|all] [-n 20]` | Pull requests of the tracked repositories, from the most recently updated to the oldest | V2 |
| `watchoo releases [--repo X] [-n 20]` | Releases of the tracked repositories, then their tags (`-n` or `--limit`) | V2 |
| `watchoo languages [--repo X]` | Languages of the tracked repositories, with their percentage | 1.1.0 |
| `watchoo diagnostic [--repo X] [--author Y] [--since 30d]` | Reflection leads of the diagnosis, **for the trainer only**: commits and pull requests that declare the use of an AI tool (level "declared": signals D1, D2, D3, D5), commits whose rhythm and form converge (level "probable": at least two R signals and one F signal), and commits or days with a rhythm or form unusual for the learner (level "to review": one signal R1 to R6, or two signals F1 to F5; an F signal alone raises nothing), absolute signals A1 to A7 for projects delivered in few commits and traces of human work H1 to H4 that lower "probable" (phase 7.7), with their signals, the D4 context of the repositories and the histories still too short. A lead is never a verdict. Diagnosis disabled: the command says so, without an error (see [Supervision and diagnosis](12-supervision-diagnostic.md)) | 7.2 (rhythm: 7.3; form: 7.5) |
| `watchoo diagnostic backfill [--repo X]` | Measures on the local clones the history already stored without signals (commits synced before the diagnosis was enabled), and completes the form (F1 to F5) of the commits measured before phase 7.5, within the retention period, merges excluded | 7.3 |
| `watchoo conventions [owner/repo] [--commits] [--failing] [--contributor X] [--milestone vX.Y.Z]` | Rates of commits conforming to the [project conventions](13-conventions.md) (CC1 to CC4), **for the trainer only**: per repository, per contributor and, with `--commits`, per commit (short SHA, author and issue codes, never the message); since 1.8.0, branch names, pull request titles (number and codes, never the title), tags, milestones, releases as information, wording, footers and language of the commits, CC1 per interval between tags, CC3 trend and, with `--milestone`, the milestone review. No overall grade. Conventions disabled: the command says so, without an error | 1.7.0 (`--milestone`, indicators of phase 8.2: 1.8.0; file criteria: 1.9.0; history and versions of phase 8.4: 1.10.0) |
| `watchoo status` | Summary: up-to-date, inactive and in-error repositories, source of the syncs (GitHub API or local Git repositories), remaining API quota, the `gh` account in use and its scopes (without showing the token) | MVP (source: V2) |
| `watchoo serve [--addr 127.0.0.1:8080] [--interval 30m]` | Starts the web interface and the periodic sync (30 min by default, 15 min minimum) | MVP |
| `watchoo tui [--interval 30m]` | Opens the interactive mode in the terminal | MVP |
| `watchoo config export [-o file] [--force]` | Writes the shareable configuration of the workspace (tracked repositories and common settings) to a JSON file, for the other people supervising the same group | After MVP |
| `watchoo update [--check] [--yes]` | Self-updates Watchoo from the GitHub releases after confirmation (archive verified by SHA-256 checksum, binary replaced in one go); `--check` only checks. Works outside a workspace and without `gh` (see [Self-update](#self-update-watchoo-update)) | 0.6.0 |
| `watchoo config import file [--dry-run] [--yes]` | Merges a shared configuration file into the workspace, after a preview: adds the missing repositories, updates the others, rewrites the common settings; never removes anything | After MVP |

### Details

- **`repo add`** accepts `owner/repo` or a full GitHub URL. Without `--branch`, **all branches** of the
  repository are tracked, including those created later. `--branch` restricts tracking to a list and to
  patterns (`--branch main,dev,release/*`); a new branch matching a pattern is tracked automatically. See
  [Architecture](03-architecture.md#tracked-branches).
  `--branch ""` means all branches. `--flemme N` and `--dead N` set the **own activity thresholds** of
  the repository, in days without a commit, instead of the workspace ones
  (`activity.flemme_after_days`, `activity.dead_after_days`); each one is optional and independent,
  `0` means the workspace threshold. A threshold is at most 3,650 days and, when both are given,
  `--dead` must be strictly greater than `--flemme` (otherwise refusal before any call to GitHub).
  `--conventions-since YYYY-MM-DD` (1.7.0) sets the **start date of the [conventions](13-conventions.md)**
  for this repository, instead of `conventions.since`: a calendar date, read at midnight in the time zone
  of the workspace; `--conventions-since ""` goes back to the date of the workspace; another form or an
  impossible date is refused before any call to GitHub. An
  already tracked repository is not added twice: its label, its branches, its thresholds and its
  conventions start date are updated if they are given, and tracking resumes if it had been stopped. The
  report quotes the own thresholds ("thresholds: flemme 3 d, dead 10 d") and the own date ("conventions
  since 2026-09-01"); `repo list` adds below the table one line per repository that has its own date.
  The import file of `repo import` has no column for this date. Access is checked with
  GitHub (one request); `repo add` does not start a sync. Without `gh` (V2), access is checked with
  `git ls-remote`, cloning nothing: a failure gives "access denied: check the SSH key or the Git credentials of this machine"; the repository is
  recorded without a GitHub ID (`provider_id` unknown, resolved at the first sync through the API) and
  with the URL `https://github.com/owner/repo`.
- **`repo list`** prints all the repositories of the workspace, stopped tracking included. The REPOSITORY
  column shows the label if there is one, otherwise `owner/repo`; the author is the one of the last
  commit. Gray states: `stopped` (tracking stopped), `error`, `never` (never synced), `archived`,
  `empty` (synced without any commit).
- **`repo remove`** asks for confirmation (`[y/N]`) when standard input is a terminal; otherwise (script,
  pipe) the **`--yes` / `-y` option is required**. Any answer other than `o`, `oui`, `y` or `yes`, or a
  Ctrl+C during the question (taken into account immediately), cancels with a non-zero return code.
  Without `--keep-history`, the repository, its branches, commits and syncs are deleted from the database
  (the contributors remain), as well as its local Git clone (`repos/<name>` at the root of the workspace,
  V2) if it exists, and the clone of a same-named repository that takes this name is moved (see
  [Local Git repositories](03-architecture.md#local-git-repositories-v2)); with `--keep-history`, tracking
  stops (the repository is no longer synced by `watchoo sync`, it shows as `stopped`), the
  history in the database **and** the clone are kept, and `repo add` resumes it.
- **`sync`** prints a table per repository (status `ok`, `error`, `skipped` if it is already
  being synced, `deferred` if the quota is low, with the resume time in local time), the
  summary and the remaining quota. `watchoo sync owner/repo` syncs that repository even if its tracking
  is stopped. Non-zero return code if a repository is in error. A synced repository whose pull
  requests, releases or tags could not be read, or whose local Git clone is unavailable with `gh` (line
  stats then read through the API, limited), stays `ok`: the warning is in the DETAILS column (and
  `warnings` in JSON), and does not change the return code. Without `gh` (V2), the summary ends with
  "Source: local Git clones (gh absent or signed out; "gh auth login" to use the GitHub API instead)." and
  gives no quota; a repository that `git` cannot read is `error` with "access denied: check the SSH key or
  the Git credentials of this machine".
- **`--since`** accepts a relative duration: an integer followed by `d` (days), `h` (hours) or `m`
  (minutes), for example `7d`, `30d`, `12h`.
- **`log`** prints, for each commit, the date, the repository, the author, the short SHA, its lines
  (`+/-` column: "+12/-3", "+0/-0" for a commit really without any line, "-" if they are still
  unknown; 0.5.0), the AI tools signed on it (AI column, "-" if none; 1.2.0, see
  [AI signatures](02-features.md#ai-signatures)) and the first line of the message.
- **AI signatures in the TUI** (1.2.0): one line in a contributor's detail, "3/120 (Claude 2, Cursor 1)",
  only when a signature exists.
- **Languages in the TUI** (1.2.0): one line in a contributor's detail, "PHP 52.0%, Twig 33.5%, others
  14.5%" (five languages, then the rest grouped), "-" if no commit is analyzed yet.
- **`+/-` lines in the TUI** (0.5.0): per commit, "+12/-3", "+0/-0" or "-" (unknown); the totals
  (contributor, repository) sum the known commits and add "(N uncounted)" when a
  part of the commits has none yet, "-" if none has.
- **`log -n`** (or `--limit`) limits the number of commits printed (20 by default, `0` = all). A commit
  present on several branches appears only once.
- **`log --author`** compares the GitHub login, the name or an e-mail of the author, **exactly** (no
  substring), ignoring the case of ASCII letters ("ÉLODIE" does not find "élodie").
- **`log --branch`** shows the commits **seen on that branch** during the syncs: those it shares with the
  reference branch (often `main`) are attached to it only if they were seen while comparing it
  (see [Architecture](03-architecture.md#incremental-sync)).
- **AI column of `contributors`** (1.2.0): signed commits / analyzed commits ("3/120"), "-" if none is
  analyzed yet. A line under the table recalls, as soon as a signature is shown, that its absence proves
  nothing.
- **`contributors`** ranks the commit **authors** by number of commits. Lines added and deleted are read
  in the local Git clone of each repository, for all the commits (first import included); they sum the
  commits whose stats are known (see
  [Incremental sync](03-architecture.md#incremental-sync): only an unavailable clone leaves them
  unknown, except for the 100 commits per sync that the API then gives).
  **`--weeks N`** (1 to 52) limits the period to the last N weeks (Monday to Sunday, in the display
  time zone, the current week included) and gives, for each contributor of that period, their commits
  week by week: in text, one column per week (dated from the Monday, "10/12"; "12/10" in French) then the total; in JSON,
  the `weekly` key. With `--repo`, only the commits of the repository count, each one once whatever its
  branch. `--weeks` and `--since` do not go together (refusal).
- **`prs`** lists the pull requests recorded by the syncs (see
  [Incremental sync](03-architecture.md#incremental-sync)), from the most recently updated to the
  oldest: update (relative date), repository, number, state (`open`, `draft`, `merged`, `closed`; closed =
  without merge), author (login), title (first line,
  cleaned like a commit message). `--state` filters: `open` (open ones, drafts included), `merged`,
  `closed` (closed without merge) or `all` (default). `-n` (or `--limit`): 20 by default,
  `0` = all.
- **`releases`** prints two tables: the **releases** (publication, or creation for a draft, as a
  relative date; repository, tag, title, state `published`, `pre-release` or `draft`,
  author), from the newest to the oldest, then the **tags** (repository, tag,
  short SHA of the targeted commit and date of that commit if it is in the database, otherwise `-`),
  from the most recent commit to the oldest. `-n` limits each table (20 by default, `0` = all).
  Drafts are only visible if the `gh` account has write access to the repository (GitHub rule).
- **`languages`** (1.1.0) prints, per repository, its languages from the largest to the smallest: name,
  share of the bytes (one decimal) and size, on the default branch. They are computed at each sync from
  the local Git clone (see [Architecture](03-architecture.md#languages-of-a-repository-110)): a repository
  has none until its next sync. The TUI gives them in the detail of a repository.
- **Languages of `contributors`** (1.2.0): LANGUAGE column, the language in which the contributor added the
  most lines over the period and the repository of the ranking, "-" if none of their commits is analyzed
  yet; in JSON, all their languages. Rules: [Architecture](03-architecture.md#languages-of-a-contributor-120).
- **Share of the commits** (1.1.0): `contributors` gives the share of each contributor (SHARE column;
  `share` in JSON), relative to the commits of the ranking: with `--repo`, those of the repository; with
  `--since` or `--weeks`, those of the period. A commit without a known author does not count. The table
  of `--weeks` has no such column, the JSON does. The TUI gives this share in the detail of a repository.
- **`conventions`** (1.7.0) evaluates the commit messages with the strict parser of the charter
  (`conventions.commit.*` settings, see [Project conventions](13-conventions.md#51-commits-cc)). **No
  grade**: a table per repository gives the rate `n/m` (commits without blocking issue / commits evaluated)
  and the percentage ("—" without evaluated commit: unknown, not 0%), the informative view of the default
  branch alone, the number of excluded commits, the effective start date and the source of the scope;
  then, per repository, the start date and its origin (workspace or own to the repository), the exclusions
  by reason (merges, bots, GitHub initial commit, accounts of `conventions.excluded_accounts`), the commits
  before the start date (counted apart) and the issues by code with their label and severity (blocking or
  warning); finally a table of the contributors (rate and codes received, sorted by name, never ranked).
  The scope is **all the tracked branches**, each commit once, read from the existing local Git clone
  (`git rev-list` of the tracked `origin/<branch>`, without `fetch`); without a readable clone, it falls
  back on the commits attached to the tracked branches in the database (source "database", marked with an
  asterisk and the mention "may include rewritten commits"). A commit whose parent count is still unknown
  is recognized as a merge by its default message (a line says so). `owner/repo` limits to one tracked
  repository; `--contributor` to one author (exact login, name or e-mail); `--commits` adds the list of the
  evaluated commits (date, repository, short SHA, author, "conform" or "not conform" and codes) and
  `--failing` limits it to the commits that do not conform (it implies `--commits`). **The commit message
  is never shown**, neither in text nor in JSON. The exit code is `0` whatever the rates (no `--strict`
  option). Conventions disabled (`conventions.enabled: false`, the default): a message says so; in JSON,
  `enabled: false` and empty lists.
  Since 1.8.0 (phase 8.2, see [Project conventions](13-conventions.md#54-tags-and-semver-sv)),
  the detail of each repository adds: the **branches** (BR1: count "n/m conform, k to review" and source,
  `clone` or "database, tracked branches only", then the names by code with their label and severity, a
  head branch followed by its pull request numbers); the **pull requests** (PR1 and PR2: conventional
  titles of the open and the merged ones apart, default titles, those set aside, then each pull request that
  does not conform as `#number author (head branch): codes`; **never the title**; "unknown" in clone-only
  mode or for a repository the API has never seen); the **tags** (SV1, names by code), the **milestones**
  of `conventions.tags.milestones` (SV2: present, nonconforming name, absent) and the **releases** (SV6,
  information only); the **wording** of the subjects (CC5 warnings: past tense, "-ing" form, first
  person), the **footers** and issue references (CC6), the **language** of the subjects (CC7: `fr`, `en`,
  unknown, those outside `conventions.commit.language`, "mixed"); finally **CC1 between tags** (each
  interval between conforming final tags, `start→v0.1.0`, `v0.1.0→v1.0.0`…, then since the last one; read
  in the clone, "unknown" without a clone). The table of the contributors adds the two rates of the **CC3
  trend** (up to and since the last final tag of each repository, "—" when not measured), the conforming
  pull request titles and the conforming branches (those of their pull requests).
  `--milestone vX.Y.Z` (1.8.0) shows, instead of the overview, the **milestone review** of a milestone of
  `conventions.tags.milestones`: per repository, "k/n criteria conform, u unknown", then CC1 on the
  milestone interval, BR1, SV1 and SV2, each conform, not conform or unknown with its reason, never an
  overall grade; a repository without a start date is "not evaluable (missing start date)". The review is
  about repositories: `--contributor` does not restrict it. A milestone that is not configured is a usage
  error (exit code `1`) that lists the configured milestones.
  Since 1.9.0 (phase 8.3, see [Project conventions](13-conventions.md#55-changelog-cl)), the detail of
  each repository adds the **file criteria** measured by the sync on the tree of the head of the default
  branch of the local clone and kept in a cache: the revision measured (short SHA, date of the
  computation), then one grid per family, **CHANGELOG** (CL1 to CL8), **README** (RD1 to RD4),
  **documentation** (DOC1 to DOC5, DOC8) and **tooling** (TL1 to TL3), as "k/n criteria conform, u
  unknown", followed by each criterion not conform, unknown (with its reason) or carrying a warning, with its
  codes and their label. Nothing computed yet: "unknown (not computed yet, run watchoo sync)"; settings,
  rules or revision changed since: "to recompute"; no local clone: "no local clone". With a repository
  (`watchoo conventions owner/repo`, detailed view), the **path lists** are recomputed from the clone, never
  stored: documents not referenced by the INDEX (DOC3), dead links `source → target` (DOC4), badly named
  folders and files (DOC5), archives and office documents (DOC8), at most 20 paths per list in text. The
  milestone review adds CL9, CL8, RD1, RD2 (installation section only; the sections found are given for
  information), RD3, DOC1 and DOC3, read on the tree of the milestone tag; tag absent: "unknown" with the
  reason of SV1. Repository indicators, never credited to a person: `--contributor` gives them as
  "unknown (report restricted to contributors)".
  Since 1.10.0 (phase 8.4, see [Project conventions](13-conventions.md#54-tags-and-semver-sv)), the detail
  of each repository ends with its **history**, observed by the syncs while the conventions are enabled and
  never attributed to anyone: the **rewrites of the default branch** (BR4: count, date of the last one, then
  each event as `branch on date: old → new` short SHAs; "no rewrite of the default branch observed"
  otherwise, never "never rewritten"), the **tag history** (SV3: "n without rewrite observed since <first
  observation>, r rewritten, d deleted, p deleted then set again on the same commit", then each tag that changed), the **due dates** of the milestones
  (SV2: on time, late, or undetermined "set between X and Y"), the **pre-releases** (SV4 warnings: free form,
  set after the final version, precedence against the history), the **precedence** of the final versions
  (SV7: higher version set on an ancestor of a lower one; increment skipped as information), the **version
  bumps** (SV5: consistent, under-versioned with the expected bump, undetermined, unknown) and the
  **CHANGELOG freshness** (CL10, information: `feat` and `fix` commits since the last commit that touched
  the CHANGELOG, grayed out when the CHANGELOG of the last milestone is up to date). SV5 and CL10 are read
  from the local clone ("unknown (no local clone)" without one); without a clone, SV4 judges when a
  pre-release was set by the first observation of the tags and SV7 only keeps the skipped increment
  (information). With `--contributor`, this history
  is not shown. The milestone review adds SV3 after SV2 (12 criteria) and judges the due date in SV2.
- **`status`**: `gh` account and its scopes, **source** of the syncs (V2: "Source: GitHub API (gh signed
  in)" or "Source: local Git clones (repos/ in the workspace), …", with the `gh auth login` reminder),
  remaining quota (reading it is free; "not applicable: local Git clones, no quota" with the local Git
  repositories), repositories by state, repositories in error,
  last successful sync, tracked branches. Below 10% of remaining quota, the quota guard is reported
  ("Low quota: syncs postponed until HH:MM (less than 10% of the quota left).",
  `quota_low` in JSON), with the same rule as the engine. An absent or signed-out `gh` is reported
  without failing the command. The source is the one a sync started now would take (`gh auth token`,
  without contacting GitHub). The token is never displayed.
- **Outputs**: an aligned text table, without borders, headers in uppercase; relative dates
  ("2 h ago") in local time (`timezone` setting, otherwise the time zone of the system).
  Colors (headers in bold, gray states in gray) only on a terminal, without `NO_COLOR` nor `TERM=dumb`.
  On a terminal without Unicode, ASCII symbols (`! 42 d`; in French, headers without accents too:
  `DEPOT`); redirected output stays
  in UTF-8. Texts coming from the repositories or the database (commit messages, Git names, labels,
  error messages) are **cleaned** before being written, in text mode as in the TUI: escape sequences
  (ESC, CSI, OSC) removed, other control and formatting characters (line breaks, writing-direction
  marks) replaced with a space. A student can therefore neither change the window title nor clear the
  instructor's screen. JSON keeps the original text, escaped by the encoder.
- **Labels**: `repo add --label` applies the same rule as the import (valid UTF-8, no control character,
  200 characters at most); an invalid label is rejected before any call to GitHub.
- **`--json`**: every command that produces data writes a stable JSON object (English keys,
  `snake_case`, UTC RFC 3339 dates or `null`); its keys are listed in [JSON output](#json-output) and
  checked by the tests. Errors go to the error output.
- **Outside a workspace**: every command except `init`, `version`, `update` and `help` stops with "no Watchoo
  workspace here, run "watchoo init" (or designate a workspace with -C <directory>)".
- **`serve`** listens on `web.addr` (`127.0.0.1:8080` by default; `--addr` overrides it) and prints the
  URL to open (header card on a terminal). Outside the loopback (`127.0.0.0/8`, `::1`, `localhost`), it
  refuses to start without authentication: `web.auth.username` with the password in the
  `WATCHOO_WEB_PASSWORD` variable (12 characters at least, never in a file), or `web.trusted_proxy: true`
  behind an authenticating reverse proxy, with its public names in `web.allowed_hosts` (warning at
  startup; without `web.allowed_hosts`, refusal). Port already in use: clear message. `--interval` as for
  `tui` (`0` = manual sync only); at startup, a sync is started only if the last one is more than one
  interval old. `--json` is refused. Ctrl+C (or SIGTERM) shuts down cleanly: no more new requests, sync
  in progress cancelled and waited for, database closed afterwards. See
  [Web interface](06-web-interface.md).
- `watchoo help [command]` and the `-h` / `--help` option print the help in the language of the
  messages (English by default, French). There is no
  `completion` command (the one `cobra` adds by default is disabled: its help is in English).
- **`--version`**, option of `watchoo` alone (`watchoo init --version` is refused), has the same effect
  as `watchoo version`, `--json` included.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Any failure: invalid option or argument, missing workspace, error of the command, cancelled confirmation, Ctrl+C, and partial failures (`sync` with a repository in error, `repo import` and `config import` with a repository inaccessible or in error) |

The message is on the error output. A finer distinction between failures, if it ever comes, will
use new codes greater than `1` and will be a compatible change.

## Environment variables

Every setting of `config.yaml` can be overridden by a `WATCHOO_<SECTION>_<KEY>` variable (see
[Workspaces](10-workspaces.md)); lists are written separated by commas (`"dependabot/*, renovate/*"`).

| Variable | Role |
| --- | --- |
| `WATCHOO_NAME` | `name` setting |
| `WATCHOO_SYNC_AUTO`, `WATCHOO_SYNC_INTERVAL`, `WATCHOO_SYNC_PARALLELISM` | `sync.*` settings |
| `WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS`, `WATCHOO_ACTIVITY_DEAD_AFTER_DAYS` | `activity.*` settings |
| `WATCHOO_BRANCHES_EXCLUDE` | `branches.exclude` setting (list) |
| `WATCHOO_WEB_ADDR`, `WATCHOO_WEB_AUTH_USERNAME`, `WATCHOO_WEB_TRUSTED_PROXY`, `WATCHOO_WEB_ALLOWED_HOSTS` | `web.*` settings (`allowed_hosts`: list) |
| `WATCHOO_TIMEZONE` | `timezone` setting |
| `WATCHOO_LANG` | Language of the CLI and the TUI, and `lang` setting of the web interface (`en` or `fr`) |
| `WATCHOO_GITHUB_USER`, `WATCHOO_GITHUB_GH_PATH` | `github.*` settings |
| `WATCHOO_GIT_PROTOCOL` | `git.protocol` setting |
| `WATCHOO_DIAGNOSTIC_ENABLED`, `WATCHOO_DIAGNOSTIC_EXCLUDE`, `WATCHOO_DIAGNOSTIC_RETENTION_DAYS`, `WATCHOO_DIAGNOSTIC_SPEED_SIGNAL` | `diagnostic.*` settings (`exclude`: list; see [Supervision and diagnosis](12-supervision-diagnostic.md)) |
| `WATCHOO_CONVENTIONS_ENABLED`, `WATCHOO_CONVENTIONS_SINCE`, `WATCHOO_CONVENTIONS_COMMIT_TYPES`, `WATCHOO_CONVENTIONS_COMMIT_SCOPES`, `WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_SOFT`, `WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_HARD`, `WATCHOO_CONVENTIONS_COMMIT_FORBIDDEN`, `WATCHOO_CONVENTIONS_COMMIT_LANGUAGE`, `WATCHOO_CONVENTIONS_BRANCH_PATTERN`, `WATCHOO_CONVENTIONS_BRANCH_EXCLUDE`, `WATCHOO_CONVENTIONS_TAGS_MILESTONES`, `WATCHOO_CONVENTIONS_TAGS_PRERELEASE_FORMS`, `WATCHOO_CONVENTIONS_CHANGELOG_CATEGORIES_FR`, `WATCHOO_CONVENTIONS_README_SECTIONS`, `WATCHOO_CONVENTIONS_README_MIN_WORDS`, `WATCHOO_CONVENTIONS_DOCS_DIR`, `WATCHOO_CONVENTIONS_DOCS_MEDIA_DIR`, `WATCHOO_CONVENTIONS_DOCS_ARCHIVES_SCOPE`, `WATCHOO_CONVENTIONS_EXCLUDED_ACCOUNTS` | `conventions.*` settings (`types`, `scopes`, `forbidden`, `branch.exclude`, `tags.milestones`, `tags.prerelease_forms`, `readme.sections`, `excluded_accounts`: lists; `since`: `YYYY-MM-DD`; `branch.pattern`: RE2 expression, never split; `tags.milestones`: `vX.Y.Z` or `vX.Y.Z@YYYY-MM-DD`; `readme.sections`: keys among `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`, `license`; `readme.min_words`: at least 1; `docs.archives_scope`: `repository` or `documentation`; `tags.prerelease_forms`: at least one form, lowercase words; branch and tags settings: 1.8.0; changelog, README and documentation settings: 1.9.0; `tags.prerelease_forms`: 1.10.0; see [Project conventions](13-conventions.md#8-configuration)) |
| `WATCHOO_DIR` | Workspace to use (like `-C`) |
| `WATCHOO_WEB_PASSWORD` | Password of the web interface (never in a file) |
| `WATCHOO_LOG_LEVEL` | Log level (`debug`, `info`, `warn` by default, `error`) |
| `WATCHOO_NO_UPDATE_CHECK` | Disables the new-version check |
| `WATCHOO_VERSION`, `WATCHOO_INSTALL_DIR` | Install script only (`install.sh`) |

The other `WATCHOO_*` variables (`WATCHOO_DOCKER`, set by the Docker image; `WATCHOO_RELEASES_URL`, for
the tests of the install script) are internal and not part of the public surface.

## JSON output

With `--json`, every command that produces data writes **a single JSON object** (indented, followed by an
end of line) to standard output, and nothing else: no header card, no tip. Warnings, confirmation
questions and errors go to the error output; on error, standard output stays empty (except `repo import`
and `config import`, which write their summary then return a non-zero code if some repositories remain
inaccessible).

Common rules, which form a **stable contract** for scripts (a key is neither renamed nor removed without
a new major version since 1.0.0, see [Skills](11-skills.md)):

- keys in English, in `snake_case`; all the keys are always present (except in the document of
  `config export`, whose optional keys follow [Configuration sharing](#configuration-sharing-watchoo-config));
- dates in UTC, RFC 3339 to the second (`"2026-10-27T10:00:00Z"`), `null` if unknown;
- empty lists written `[]`, never `null`; a missing string is `""`;
- texts coming from the repositories (messages, names, labels) kept as is, escaped by the encoder;
- never a token. `tui` and `serve` refuse `--json`.

What the contract covers (1.0.0):

- **compatible changes** (MINOR): adding a key to an object, adding a value to a list of values
  (`state`, `status`, `outcome`, `action`…); a script ignores the keys it does not know and accepts an
  unknown value;
- **texts in the selected language** (`WATCHOO_LANG`): `state_label`, `message`, `error`, `reason`,
  `rate_limit_error` and the `warnings` messages; a script tests the codes and states, not these texts;
- **identifiers**: `id` is the local ID in the workspace's database (except `releases[].id`, the ID on
  the forge); it does not carry over to another workspace, nor after a `repo remove` without
  `--keep-history` followed by a `repo add`;
- **`flemme`** (state, thresholds, `--flemme` option, import column, setting) is the stable machine
  name of the "Slacking" state, whatever the language of the messages;
- **not covered**: the text output (columns, wording), the logs (`WATCHOO_LOG_LEVEL`) and the
  internal variables.

Top-level keys (the list is checked by a test, `internal/cli/json_schema_test.go`):

| Command | Top-level keys | Notes |
| --- | --- | --- |
| `watchoo init` | `name`, `root`, `dir`, `config_file`, `database`, `parent_workspace`, `github`, `source`, `warnings` | `github`: `host`, `login`, `token_source`, `scopes` (empty without `gh`); `source`: `api` or `git` (local Git repositories, without `gh`, V2); `parent_workspace` empty without a parent workspace |
| `watchoo version` | `version`, `commit`, `date`, `go`, `os`, `arch` | See below |
| `watchoo repo add` | `outcome`, `repository` | `outcome`: `added`, `already` or `updated`; `repository`: repository object |
| `watchoo repo import` | `file`, `format`, `dry_run`, `summary`, `entries` | `format`: `csv` or `list`; `summary`: `added`, `already_tracked`, `updated`, `inaccessible`, `errors`; each entry: `line`, `repo`, `outcome`, `repository` (`null` if inaccessible or in error), `error` |
| `watchoo repo list` | `repositories` | Each element: repository object plus `activity`, `commits`, `tracked_branches`, `last_commit` (commit object or `null`), `last_sync` (`null` if none) |
| `watchoo repo remove` | `action`, `repository`, `deleted_commits` | `action`: `deleted`, `stopped` or `already_stopped` |
| `watchoo sync` | `started_at`, `finished_at`, `summary`, `repositories`, `rate_limit`, `source` | `source`: `api`, `git` (local Git repositories, V2; `rate_limit` is then `null`) or `""` if the sync did not start; `summary`: `repositories`, `success`, `error`, `locked`, `deferred`, `new_commits`; each repository: `id`, `full_name`, `display_name`, `status` (`success`, `error`, `locked`: already being synced, `skipped` in text; `deferred`: low quota; `success` is `ok` in text), `new_commits`, `tracked_branches`, `stats_fetched`, `duration_ms`, `error`, `resume_at`, `warnings` (messages in the selected language: pull requests, releases or tags not synced, or local Git clone unavailable with `gh`, while the repository succeeded; `[]` otherwise); `stats_fetched`: commit stats computed in the clone, or requested from the API as a fallback |
| `watchoo log` | `commits` | From newest to oldest; commit objects |
| `watchoo contributors` | `contributors` | Each element: `id`, `name`, `login`, `display_name`, `avatar_url`, `commits`, `additions`, `deletions`, `repositories`, `share` (1.1.0: share of the commits of the ranking, as a percentage with one decimal), `last_commit_at`, `ai_signed_commits`, `ai_scanned_commits`, `ai_tools` (1.2.0: commits with a signature, commits analyzed, each tool with `tool` and `commits`; `0` is not evidence of work done without AI), `languages_scanned_commits`, `languages` (1.2.0: commits whose languages are read; each language with `name`, `lines` (lines added) and `percent` (share of their lines, one decimal), from the one with the most to the least; `[]` if none), `weekly` (with `--weeks N`: N `week` objects (Monday, `YYYY-MM-DD`, display time zone) and `count`, from the oldest to the newest; `[]` without `--weeks`) |
| `watchoo prs` | `pull_requests` | From the most recently updated to the oldest; pull request objects |
| `watchoo releases` | `releases`, `tags` | Release objects (from the newest to the oldest) and tag; `-n` limits each list |
| `watchoo languages` | `languages` | From one repository to the next, largest language first. Each element: `repository`, `name`, `bytes`, `percent` (share of the repository, one decimal) |
| `watchoo status` | `workspace`, `github`, `source`, `rate_limit`, `rate_limit_error`, `quota_low`, `repositories`, `errors`, `last_synced_at`, `tracked_branches` | `workspace`: `name`, `root`; `github`: `ok`, `host`, `login`, `token_source`, `scopes`, `error`; `source` (V2): `kind` (`api`, `git`, or `""` if `gh` is down) and `reason` (why the local Git repositories, or the `gh` error); `repositories`: `total`, `active`, `flemme`, `dead`, `error`, `never_synced`, `archived`, `empty`, `stopped`; each error: `repo`, `message` |
| `watchoo diagnostic` | `enabled`, `leads`, `contexts`, `histories`, `notice`, `filters` | `enabled`: `diagnostic.enabled` setting (false: empty lists); each lead: `kind` (`commit`, `pull_request` or `day`), `level` (`declared`, `probable` or `to_review`), `repository`, `sha`, `short_sha` (empty except for a commit), `number` (0 except for a PR), `title` (PR title, empty otherwise), `url`, `author` (`name`, `login`, `display_name`), `date` (commit author date, PR opening, or start of the day), `signals` (each signal: `id` among `D1`, `D2`, `D3`, `D5`, `R1` to `R6`, `F1` to `F5`, `A1` to `A7`, `H1` to `H4` (counter-indicators, phase 7.7); `value`: tool, bot account, branch or measure; `raw`, `median`, `z`: measure, personal median (F5: usual share of commits with tests) and modified z-score of the rhythm and form signals, `null` when there is none); each context: `repository`, `agent_config_since`; each history: `repository`, `author`, `commits`, `worked_days`, `sufficient`; `notice`: translated reminder; `filters`: `repository`, `author`, `since` |
| `watchoo diagnostic backfill` | `repositories`, `measured` | Each repository: `repository`, `measured` (commits measured), `merges` (merges set aside), `missing` (missing from the clone), `error` (empty on success); exit code `1` if a repository failed |
| `watchoo conventions` | `enabled`, `rules_version`, `repositories`, `contributors`, `commits`, `notice`, `filters`, `milestone` | 1.7.0 (`milestone` and the keys of phase 8.2: 1.8.0; `changelog`, `readme`, `docs`, `tooling` and `milestone.repositories[].files`: 1.9.0; the keys of phase 8.4: 1.10.0). `enabled`: `conventions.enabled` setting (false: empty lists); `rules_version`: version of the parser rules; each repository: `repository`, `source` (`clone` or `database`, a fallback that may include rewritten commits), `since` (effective start date `"YYYY-MM-DD"`, `null` = from the beginning), `since_origin` (`workspace`, `repository` or `null`), `commits` (`evaluated`, `conform`, `rate` (to the thousandth, `null` without evaluated commit), `excluded` (`merge`, `bot`, `initial_commit`, `excluded_account`), `issues` (each `code`, `severity`: `blocking` or `warning`, `count`, `shas`)), `before_since` (commits before the start date), `parents_unknown` (commits with an unknown parent count), `default_branch` (`name`, `evaluated`, `conform`, `rate`, informative view); each contributor: `author` (`name`, `login`, `display_name`), `commits` (same keys as for a repository); `commits` (`[]` without `--commits` or `--failing`): each `repository`, `sha`, `short_sha`, `author`, `date`, `conform`, `codes`, never the message; `notice`: translated reminder; `filters`: `repository`, `contributor`, `commits`, `failing`, `milestone` (`""` without `--milestone`). Since 1.8.0, each repository also has `traits` (`evaluated`, `wording` (`past_tense`, `progressive`, `first_person`), `footer`, `issue_reference`), `language` (`expected`, `fr`, `en`, `unknown`, `other`, `mixed`), `branches` (`source`, `checked`, `conform`, `review`, `items`: each `name`, `codes` (BR1; blocking `unknown_type`, `uppercase`, `underscore`, `space`, `accent`, `no_slash`, `pattern`; warning `single_segment`), `conform`, `review`, `in_source` (false: only known as a pull request head branch), `pull_requests` (numbers), `authors`), `pull_requests` (`known`, `reason` (`no_api` or `null`), `open` and `merged` (`evaluated`, `conform`, `default`), `skipped` (`closed`, `draft`, `bot`, `before_since`), `items`: each `number`, `category` (`open` or `merged`), `author`, `head_branch`, `codes` (CC2 codes of the header), `default_codes` (PR2: `forbidden`, `github_default`, `head_branch`), `conform`, never the title), `tags` (`checked`, `conform`, `items` (`name`, `codes` (SV1; blocking `no_v`, `upper_v`, `missing_component`, `leading_zero`, `empty_prerelease`, `invalid`; warning `build_metadata`), `conform`, `final` (conforming, without pre-release or build metadata: only a final tag bounds an interval)), `milestones` (`name`, `due`, `status`: `present`, `nonconforming_name` or `absent`, `tag`, `others`), `releases` (`known`, `reason` (`no_api` or `null`), `notes`: each `code` (SV6: `milestone_without_release`, `release_without_tag`, `prerelease_mismatch`), `tag`)), `intervals` (`known`, `reason` (`no_clone`, `clone_unreadable`, `filtered`, `no_final_tag` or `null`), `last_tag`, `items`: each `from` (`null` = from the beginning), `to`, `known`, `commits`, `before_since`, `missing`; `since_last_tag`); each contributor `traits`, `trend` (`known`, `repositories`, `unmeasured`, `before`, `since`, rates like `commits`), `pull_requests` (`open`, `merged`), `branches` (`checked`, `conform`, `review`); `milestone`: `null` without `--milestone`, otherwise `name`, `due`, `criteria` (`CC1`, `BR1`, `SV1`, `SV2`, then since 1.9.0 `CL9`, `CL8`, `RD1`, `RD2` (installation section only), `RD3`, `DOC1`, `DOC3`) and `repositories`: each `repository`, `evaluable`, `reason` (`no_since` or `null`), `since`, `status`, `tag`, `tag_codes`, `interval` (`from`, `to`, `due`, `commits`, `before_since`, `missing`; `null` when not measured), `branches`, `files` (1.9.0, `null` when not evaluable: `state` (`computed` or `unknown`), `reason` (`no_tag`, `not_due`, `tag_absent`, `not_computed`, `stale`, `no_clone` or `null`), `sha`, `computed_at`, `readme_expected` and `readme_found`, keys of the README sections, for information), `criteria` (each `id`, `status`: `conform`, `nonconform` or `unknown`, `reason` of an unknown criterion: `no_tag`, `not_due`, `tag_absent`, `no_clone`, `clone_unreadable`, `no_commits`, `partial`, `no_branch`, and for the file criteria `not_computed`, `stale`, `no_file`, `empty_repository`, `tree_truncated`, `blob_too_large`, `blob_unreadable`, `no_docs_dir`, `unread_documents`, `null` otherwise; see [Project conventions](13-conventions.md#71-milestone-review-rj)), `conform`, `unknown`, `total`. Since 1.9.0 (phase 8.3), each repository also has `changelog` (CL1 to CL8), `readme` (RD1 to RD4), `docs` (DOC1 to DOC5, DOC8) and `tooling` (TL1 to TL3), read from the cache of the sync: `state` (`computed` or `unknown`), `reason` (`not_computed`, `stale`, `no_clone`, `filtered` or `null`), `sha` and `computed_at` (revision measured, date of the computation; `null` when unknown), `conform`, `unknown`, `total`, `criteria` (each `id`, `status`, `reason` (the reason of the family, or `empty_repository`, `tree_truncated`, `no_file`, `blob_too_large`, `blob_unreadable`, `no_docs_dir`, `unread_documents`, `null` otherwise), `codes` (blocking: `absent`, `misnamed`, `elsewhere`, `no_h1`, `h1_title`, `no_keepachangelog`, `no_semver`, `unreleased_absent`, `unreleased_misplaced`, `unreleased_duplicate`, `unreleased_no_brackets`, `no_brackets`, `date_fr`, `no_date`, `invalid_date`, `future_date`, `invalid_version`, `version_order`, `date_order`, `unknown_category`, `duplicate_category`, `bullet_outside_category`, `empty_version`, `empty_category`, `empty_bullet`, `tag_without_section`, `section_without_tag`, `version_missing`, `missing_section`, `index_link_absent`, `index_link_dead`, `index_link_folder`, `changelog_link_absent`, `changelog_link_dead`, `too_short`, `no_table`, `missing_column`, `no_rows`, `unreferenced`, `duplicate_row`, `dead_link`, `case_mismatch`, `dir_name`, `file_name`, `archive`, `not_found`; warnings: `french_label`, `v_prefix`, `category_order`, `office_document`, `husky_no_prepare`, `translation_only`), `counts` (counters only: `versions`, `sections`, `tags`, `expected`, `found`, `expected_mask`, `found_mask`, `license_file`, `translations`, `words`, `min_words`, `tables`, `rows`, `columns`, `documents`, `links`, `sources`, `unread`, `dirs`, `files`, `configs`, `extends_conventional`, `tools_mask`, `templates`, and a counter per code), never a path, a file content or a cell of the INDEX; codes and counters by criterion, bit order of the masks: see [Project conventions](13-conventions.md#58-tooling-tl)); `docs` adds `lists` (`computed`, `reason` (`not_computed`, `stale`, `no_clone`, `clone_unreadable`, `tree_truncated` or `null`), `unreferenced`, `duplicate_rows`, `dead_links` (each `source`, `target`, `resolved`, `case_mismatch`), `bad_names`, `archives`, `office_documents`): filled only in the detailed view of a repository (`watchoo conventions owner/repo`), empty lists otherwise. Since 1.10.0 (phase 8.4), `tags.milestones` items also have `deadline` (`null` without a due date judged, otherwise `status`: `on_time`, `late` or `undetermined`, `due_end` (end of the due date, excluded), `first_seen_at` (first observation of the tag, `null` if never observed), `commit_at` (date of the target commit at the first observation of the tag, `null` if unknown or if the tag was rewritten since), `set_after` (lower bound of when it was set: the later of `commit_at` and the last complete read of the tags that did not see it, before its first observation; `null` if unknown)); each repository also has `rewrites` (BR4: `known` (always `true`), `reason` (always `null`, kept for symmetry), `count`, `last`, `events`: each `branch`, `old_sha`, `new_sha`, `observed_at`, most recent first, never an author), `tag_history` (SV3: `known`, `reason` (`not_observed` or `null`), `since` (first observation), `counts` (`stable`, `restored`, `rewritten`, `deleted`), `items`: each `name`, `status`, `sha` (`null` when deleted), `first_seen_at`, `rewrites`, `deletions`), `prereleases` (SV4, warnings: `known`, `reason` (`filtered` or `null`), `checked`, `conform`, `notes`: each `code` (`free_form`, `after_final`, `order`), `tag`, `other`), `bumps` (SV5: `known`, `reason` (`no_clone`, `filtered` or `null`), `counts` (`consistent`, `under`, `undetermined`, `unknown`), `items`: each `from`, `to`, `status`, `reason` (`no_base`, `clone_unreadable`, `partial`, `low_conform` or `null`), `actual` and `expected` (`patch`, `minor`, `major` or `null`), `evaluated`, `conform`, `breaking`, `features`, `missing`), `precedence` (SV7: `known`, `reason` (`no_clone`, `filtered` or `null`), `pairs`, `unknown`, `conform`, `notes`: each `code` (`precedence_ancestry`, `increment_skip` as information), `tag`, `other`) and `changelog_freshness` (CL10, information: `known`, `reason` (`no_clone`, `no_file`, `clone_unreadable`, `filtered` or `null`), `since` (SHA of the last commit that touched the CHANGELOG), `feat`, `fix`, `missing`, `grayed`); `milestone.criteria` gets `SV3` after `SV2`, and each repository of the review gets `deadline` (same keys) and `tag_history` (`status`, `first_seen_at`, `rewrites`, `deletions`; `null` when the tag is missing or never observed), with the reasons `deadline_undetermined` and `not_observed` |
| `watchoo update` | `current`, `latest`, `update_available`, `release_url` | Only with `--check` (see [Self-update](#self-update-watchoo-update)); `watchoo update --json` without `--check` is refused |
| `watchoo config export` | `format`, `version`, `exported_at`, `watchoo_version`, `workspace`, `settings`, `repositories` | The sharing document itself (see [Configuration sharing](#configuration-sharing-watchoo-config)), with or without `--json`; with `-o`, it goes to the file and standard output only receives one confirmation line in text (nothing with `--json`) |
| `watchoo config import` | `file`, `dry_run`, `applied`, `source`, `settings`, `repositories`, `summary`, `warnings` | `source`: `workspace_name`, `watchoo_version`, `exported_at` (as written in the file); `settings`: `changes` (each `key`, `before`, `after`, `env_var`), `rewrite`, `written`, `backup_file`; each repository: `index`, `repo`, `action` (`add`, `update`, `unchanged`, `inaccessible`, `error`), `reenable`, `before` (`label`, `branches`, `flemme_after_days`, `dead_after_days` (`null` = workspace threshold), `conventions_since` (1.7.0, `"YYYY-MM-DD"`, `null` = date of the workspace), `enabled`; `null` for an addition), `after`, `outcome` (like `repo import`, `""` as long as nothing is applied), `error`; `summary`: `added`, `already_tracked`, `updated`, `reenabled` (among `updated`), `inaccessible`, `errors` (predicted with `--dry-run`, obtained otherwise); each warning: `code` (`env_override`, `comments_lost`), `key`, `message` |

Shared objects:

- **repository**: `id`, `full_name`, `owner`, `name`, `label`, `display_name`, `url`, `default_branch`,
  `branch_patterns` (`[]` = all branches), `flemme_after_days` and `dead_after_days` (own thresholds of
  the repository, `null` = workspace threshold; the applied thresholds are in the activity object),
  `conventions_since` (1.7.0: own conventions start date of the repository, `"YYYY-MM-DD"`, `null` =
  date of the workspace), `enabled`, `archived`, `stars`, `open_issues`, `last_synced_at`,
  `last_commit_at`;
- **commit**: `repository` (`id`, `full_name`, `display_name`), `sha`, `short_sha`, `message`, `url`,
  `author` (`name`, `login`, `display_name`), `authored_at`, `committed_at`, `additions`, `deletions`,
  `stats_known` (0.5.0: `true` if `additions` and `deletions` have been computed, even at 0; `false` if
  they are still unknown, `additions` and `deletions` being 0 then), `ai_signatures` (1.2.0: `tool` and
  `kind` of each signature, `[]` if none) and `ai_scanned` (`false`: not analyzed yet, `ai_signatures` says
  nothing yet);
- **activity**: `state` (`active`, `flemme`, `dead`, `stopped`, `error`, `never_synced`, `archived`,
  `empty`), `state_label`, `gray`, `days_since_last_commit` (`null` without a commit), `flemme_after_days`,
  `dead_after_days`;
- **sync** (`last_sync`): `status` (`running`, `success`, `error`), `started_at`, `finished_at`,
  `new_commits`, `error`;
- **quota** (`rate_limit`, `null` if unknown): `limit`, `remaining`, `used`, `reset`.
- **pull request**: `repository` (`id`, `full_name`, `display_name`), `number`, `title`, `state`
  (`open`, `merged` or `closed`: closed without merge), `draft`, `author` (`login`, `""` if unknown),
  `url`, `base_branch`, `head_branch`, `created_at`, `updated_at`, `closed_at`, `merged_at` (`null` if
  not applicable);
- **release**: `repository`, `id` (ID on the forge), `tag_name`, `name`, `draft`, `prerelease`,
  `author` (`login`), `url`, `created_at`, `published_at` (`null` for a draft);
- **tag**: `repository`, `name`, `sha`, `short_sha`, `commit_url` and `committed_at` (link and date of
  the targeted commit, `""` and `null` if it is not in the database).

### Version

`watchoo version --json` (or `watchoo --version --json`):

```json
{
  "version": "v0.1.0",
  "commit": "1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d",
  "date": "2026-10-27T10:00:00Z",
  "go": "go1.27.3",
  "os": "linux",
  "arch": "amd64"
}
```

- `version`, `commit` (full SHA) and `date` (date **of the** compiled commit, for reproducible builds)
  are injected at build time by `-ldflags` on `internal/cli.Version`,
  `internal/cli.Commit` and `internal/cli.Date` (Makefile, GoReleaser and Dockerfile, same options; see
  [Distribution](03-architecture.md#distribution)).
- Without these options, they come from the Go build information when it exists:
  `go install github.com/studoo-app/watchoo/cmd/watchoo@v0.1.0` gives the module version; a
  `go build` in a Git repository gives the commit, its date and the module version (`v0.1.0` on a tag,
  a pseudo-version `v0.0.0-…` elsewhere, `dev` if the tree has uncommitted changes). Otherwise
  `version` is `dev`, `commit` is `""` and `date` is `null`.
- In text: `watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, linux/amd64)`; commit and date are
  omitted if unknown. The header card, the TUI, the web interface and the `User-Agent` of the GitHub
  calls show the same version.

## Header card

On a terminal, the general help, `watchoo version` and the report of `watchoo init` start with a
**header card**: a frame with rounded corners with, on the left, the owl in small format
(`ascii-mini.txt`, see [Logo and icons](06-web-interface.md#text-versions-cli-and-tui)) and, on its
right, "Watchoo", the version and a subtitle; below, sometimes, a tip. Status lines prefixed with a `●`
may follow the card. Example, `watchoo` run in a workspace:

```
╭────────────────────────────────────────────────────────────╮
│  █▄    ▄█  Watchoo v0.1.0                                  │
│  █ ●██● █  Who worked, on what, and when?                  │
│  ▀██▙▟██▀                                                  │
│                                                            │
│  Tip: "watchoo repo add owner/repo", then "watchoo sync".  │
╰────────────────────────────────────────────────────────────╯

● Workspace: licence-projet (~/supervision/licence-projet)

Watchoo watches GitHub repositories, mostly private, and answers three questions:
…
```

| Output | Card subtitle and tip | After the card |
| --- | --- | --- |
| General help: `watchoo`, `watchoo help`, `watchoo --help` / `-h` | "Who worked, on what, and when?"; tip `Tip: "watchoo init" makes this directory a workspace.` if no workspace is found, `Tip: "watchoo repo add owner/repo", then "watchoo sync".` in a workspace, `Tip: "watchoo help <command>" details a command.` if the workspace is unreadable | An empty line, the workspace status line, an empty line, then the usual help |
| `watchoo version`, `watchoo --version` | Go version and platform (`go1.27.3 · darwin/arm64`); no tip | Nothing |
| `watchoo init` successful | `Workspace "name" created`; tip `Tip: "watchoo repo add owner/repo" adds a first repository to track.` | An empty line, then the report as status lines: workspace, settings, database, `gh` account and its scopes (without `gh`: "Source: local Git clones (gh absent or not signed in)") |
| `watchoo status` | `Status of workspace "name"`; tip `Tip: "watchoo sync" updates the tracked repositories.` | An empty line, then the usual summary |

- **Scripts**: the card is printed only if standard output is a terminal, and never with `--json`.
  Redirected output (file, pipe) stays the same: help without a card, `watchoo version` on a single line
  (`watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, darwin/arm64)`), usual report of `init`. The
  error output (errors, warnings) never has a card, nor has the help of subcommands
  (`watchoo help init`).
- **Workspace status line** (general help): `● Workspace: <name> (<root>)`, with `~` for the home
  directory, or `● No workspace here`. The workspace is looked up as the other commands
  would do (current directory and its parents, `-C`, `WATCHOO_DIR`), locally, without running
  `gh`. The lookup never fails the help: an error (invalid settings, non-existent `-C` directory…) is
  summarized on the status line, `● Workspace: <message>`.
- **Width**: that of the content, without exceeding 80 columns or the width of the terminal (`COLUMNS`
  variable if it is a positive integer, otherwise the one given by the system). Text that is too long is
  broken at spaces. On a terminal of less than 40 columns, no card: the output is that of a redirected
  output. An accented letter takes one column, whether precomposed (`é`) or decomposed (`e` followed by
  a combining accent, a common form in directory names created under macOS, hence in the default name of
  a workspace).
- **Unicode**: if the locale is UTF-8 (first non-empty variable among `LC_ALL`, `LC_CTYPE` and `LANG`)
  or in Windows Terminal (`WT_SESSION`). Otherwise, the card and the status lines are pure ASCII:
  borders `+`, `-` and `|`, owl `ascii-mini-pur.txt`, straight quotes instead of « » (French), accents removed,
  bullet `*`.
- **Colors**: border, version, subtitle and status lines in gray, "Watchoo" in bold, owl pupils in
  green, command quoted in the tip in cyan. No color if `NO_COLOR` is set and non-empty, if `TERM=dumb`,
  nor under Windows in a console that does not accept ANSI sequences.
- **Version**: a number gets the `v` prefix (`0.1.0` becomes `v0.1.0`); `v0.1.0` or `dev` (binary
  compiled without a version) are printed as is.

## Self-update: `watchoo update`

Since 0.6.0, Watchoo announces its new versions and updates itself, from the releases published on
GitHub (public pages, without token or GitHub API: see
[Architecture](03-architecture.md#watchoo-self-update-060)). Neither a workspace nor `gh` is needed.

### New-version notice

At **every launch** of a command, Watchoo checks in the background whether a version newer than its
own is published. At the end of the command, it writes to the **error output**:

```
New Watchoo version available: v0.5.0 → v0.6.0. Update with "watchoo update".
```

- The check starts at the very beginning of the command and has **1.5 s** in total: a command is never
  delayed by more, and the notice is not written if the answer did not arrive in time. A command
  longer than that delay (`sync`, for example) does not wait: the answer is already there, and the
  notice is always written. Without a network, or on error, nothing is displayed (debug-level log with
  `--verbose`).
- `watchoo` alone (the header card and the general help) also has its notice, written after the help,
  like `watchoo repo` or `watchoo config` alone; not `watchoo --help` nor `watchoo help …`.
- No check with `--json`, when the error output is not a terminal (scripts, CI, pipe), for a version
  that is not a release version (`dev`, pseudo-version of `go install` or `go build`, pre-release),
  for `watchoo update` itself, `watchoo serve` and `watchoo tui` (which have their own) and the help.
- **`WATCHOO_NO_UPDATE_CHECK`**: set to a non-empty value other than `0` or `false`, it cuts off every
  check (commands, `serve` and `tui`). The Docker image sets it (`WATCHOO_NO_UPDATE_CHECK=1`): it updates
  itself by changing the tag.
- A pre-release is never offered; an installed version newer than the last published one gives no
  notice.
- **`watchoo serve`** checks at startup then every 30 min while it runs; a new version is announced
  once on the error output (same sentence) and the web interface shows a banner (see
  [Web interface](06-web-interface.md#new-version-banner-060)).
- **`watchoo tui`** checks once, in the background, when it opens (10 s at most, without delaying the
  display); if a newer version is published, the interactive mode announces it on the line under its
  header, while it stays open:
  "A new version of Watchoo is available: v0.6.2 → v0.7.0. Quit, then run "watchoo update"."
  (in the Docker image: "Change the tag of the Docker image to update it."); on a
  terminal too narrow, a short form: "Watchoo v0.7.0 available: quit, then run "watchoo update".".
  In French: « Nouvelle version de Watchoo disponible : v0.6.2 → v0.7.0. Quittez puis lancez
  « watchoo update ». ».
  Failure or delay exceeded: nothing. No extra notice on the error output when quitting. Same exceptions
  as the commands: `WATCHOO_NO_UPDATE_CHECK`, a version that is not a release version.

### `watchoo update --check`

Checks only, installing nothing: installed version, last published version and, if there is a newer
one, the link to its release. Return code 0 whether an update exists or not; non-zero if the last
version could not be read (network). With a development version, the comparison is impossible: the
last version is printed, `update_available` is `false`.

```
$ watchoo update --check
Installed version: v0.5.0
Latest version: v0.6.0
New version available: https://github.com/studoo-app/watchoo/releases/tag/v0.6.0
Update with "watchoo update".
```

`watchoo update --check --json`:

```json
{
  "current": "v0.5.0",
  "latest": "v0.6.0",
  "update_available": true,
  "release_url": "https://github.com/studoo-app/watchoo/releases/tag/v0.6.0"
}
```

### `watchoo update`

1. Refuses immediately, with a non-zero code and a message that says what to do:
   - in the **Docker image** (`WATCHOO_DOCKER` variable or `/.dockerenv` file): change the tag of
     the image, or `docker compose pull && docker compose up -d`;
   - for a **development version** (`dev`, pseudo-version, pre-release: binary compiled from
     the sources): `go install github.com/studoo-app/watchoo/cmd/watchoo@latest`, or
     `git pull && make build` in the clone;
   - if the **directory of the binary** is not writable: directory permissions, `sudo`, or
     reinstalling with `install.sh` and `WATCHOO_INSTALL_DIR` (see the README).
2. Reads the last version. Already up to date: "Watchoo v0.6.0 is up to date (latest published version: v0.6.0).", code 0.
3. Prints `v0.5.0 → v0.6.0`, the link to the release and to the `CHANGELOG.md` of that version, then
   asks for confirmation (`Install Watchoo v0.6.0? [y/N]`) on the error output. As for `repo remove`, `--yes` / `-y` skips
   the question, and it is **required** when standard input is not a terminal; an answer other than
   `o`, `oui`, `y` or `yes` cancels with a non-zero code, changing nothing.
4. Downloads the release archive for the system and the architecture
   (`watchoo_<version>_<os>_<arch>.tar.gz`, `.zip` under Windows) and the checksum file, **refuses** an
   archive whose SHA-256 checksum does not match, extracts the binary and replaces the running one in
   one go (temporary file in the same directory, then renaming; under Windows, the old one is first
   renamed to `watchoo.exe.old`).
5. Reminds that an already running `watchoo serve` keeps the old version until it is restarted, and
   that the database of each workspace is upgraded (migrations) at the next command.

`--json` is only accepted with `--check`.

## Bulk import: `watchoo repo import`

To add all the repositories of a class of students in one go, instead of one `repo add` per student.

### File format

A **CSV** file with a header line. Only the `repo` column is required.

```csv
repo,label,branches
alice/projet-licence,Alice Martin,
bob/projet-licence,Bob Durand,"main,dev"
https://github.com/chloe/projet-licence,Chloé Petit,
mon-org/projet-groupe-3,Groupe 3 (Dan et Eva),
```

| Column | Required | Content |
| --- | --- | --- |
| `repo` | yes | `owner/repo` or GitHub URL |
| `label` | no | Display label used instead of `owner/repo` (name of the student, the pair, the group) |
| `branches` | no | Branches to track, list and patterns like `--branch`; empty = all branches |
| `flemme` | no | Own Slacking threshold of the repository, in days without a commit, like `--flemme` (`0` = workspace threshold) |
| `dead` | no | Own Dead threshold, like `--dead`; greater than `flemme` on the same line |

A plain text file is also accepted: one repository per line, no header; empty lines and lines starting
with `#` are ignored.

### Behavior

1. Reads the whole file and **validates** it before adding anything: a malformed line stops the import,
   reporting the line number. A repository present twice in the file (`owner/repo` or URL,
   case-insensitive) is an error line: nothing is imported, the message gives the line of the duplicate
   and the one of the first occurrence.
2. For each repository, **checks access** with GitHub (one request per repository). An inaccessible
   private repository (a student who has not granted access yet) is reported, without blocking the
   others.
3. Adds the accessible repositories. A repository **already tracked** is not added twice; its `label` and
   its branches are updated if the file gives them. The import can therefore be run again safely, for
   example when students have finally granted access.
4. Prints a **summary**:

```
IMPORT  licence-projet.csv
  added             18
  already tracked    3
  updated            1
  inaccessible       2   carol/projet-licence, dave/projet-licence
  lines in error     0
```

- An **empty** `label`, `branches`, `flemme` or `dead` cell means "not provided": the existing value is
  kept (an import therefore cannot set a repository back to "all branches"; use
  `repo add owner/repo --branch ""`). To go back to the workspace threshold, write `0` in `flemme` or
  `dead`. An unreadable, negative threshold, beyond 3,650 days, or a `dead` lower than or equal to the
  `flemme` of the same line is an error line.
- `-` instead of the file reads the list from standard input.
- The summary then details each inaccessible or in-error repository, with its line number.
- `--dry-run`: runs the validation and the access checks, prints the summary ("to add" instead of
  "added"), but records nothing.
- `--json`: summary in JSON format.
- Non-zero return code if some repositories remain inaccessible or in error, for scripts.
- The import does not start a sync: follow with `watchoo sync`.

## Configuration sharing: `watchoo config`

Several people often supervise the same group (instructors of the same class of students), each with
their own workspace on their machine. `watchoo config export` writes the **shareable** configuration of
the workspace to a JSON file; `watchoo config import` merges it into another workspace. This shares the
list of repositories, their display labels and their branches, and the common settings, without ever
sharing student data or a machine-specific setting.

### File content

```json
{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-27T10:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {
    "name": "licence-projet"
  },
  "settings": {
    "sync": {
      "auto": true,
      "interval": "30m",
      "parallelism": 4
    },
    "activity": {
      "flemme_after_days": 5,
      "dead_after_days": 15
    },
    "branches": {
      "exclude": []
    },
    "timezone": "",
    "lang": "en",
    "conventions": {
      "since": "2026-09-01",
      "commit": {
        "types": ["feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"],
        "scopes": [],
        "max_subject_soft": 50,
        "max_subject_hard": 72,
        "forbidden": ["update", "maj", "wip", "test", "modif", "fix", "dépôt 1", "."],
        "language": "any"
      },
      "branch": {
        "pattern": "",
        "exclude": ["gh-pages", "dependabot/*", "renovate/*"]
      },
      "tags": {
        "milestones": ["v1.0.0", "v2.0.0", "v3.0.0"],
        "prerelease_forms": ["rc", "beta", "alpha"]
      },
      "changelog": {
        "categories_fr": true
      },
      "readme": {
        "sections": ["presentation", "group", "stack", "install", "run", "env", "tree", "license"],
        "min_words": 150
      },
      "docs": {
        "dir": "documentation",
        "media_dir": "Media",
        "archives_scope": "repository"
      }
    }
  },
  "repositories": [
    {
      "provider": "github",
      "repo": "alice/projet-licence",
      "label": "Alice Martin",
      "branches": ["main", "release/*"]
    }
  ]
}
```

| Key | Content |
| --- | --- |
| `format` | Always `watchoo-config` (required) |
| `version` | Version of the format, `1` (required). A greater version is refused: "file created by a newer version of Watchoo" |
| `exported_at`, `watchoo_version` | Export date (UTC, RFC 3339) and version of Watchoo that made it; informative, optional |
| `workspace.name` | Name of the exported workspace; informative: the import never renames the workspace |
| `settings` | Common settings, values **in effect** in the exported workspace (files and `WATCHOO_*` variables included): `sync.auto`, `sync.interval` (Go duration in quotes: `"30m"`, `"1h30m"`, `"0"`), `sync.parallelism`, `activity.flemme_after_days`, `activity.dead_after_days`, `branches.exclude`, `timezone`, `lang` (`"en"` or `"fr"`; optional key of version 1: a file without it remains valid), `conventions` (charter of the [project conventions](13-conventions.md#8-configuration): `since`, `"YYYY-MM-DD"` or `""`, and `commit`: `types`, `scopes`, `max_subject_soft`, `max_subject_hard`, `forbidden`, `language`; optional key of version 1, added in 1.7.0: a file without it remains valid, but a file containing it is refused by earlier versions, which accept no unknown key; and, since 1.8.0, `branch`: `pattern` (RE2 expression, `""` = built-in rule), `exclude`, and `tags`: `milestones` (`"vX.Y.Z"` or `"vX.Y.Z@YYYY-MM-DD"`), optional keys refused by the versions before 1.8.0; and, since 1.9.0, `changelog`: `categories_fr` (boolean), `readme`: `sections` (keys among `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`, `license`; `[]` = no section expected), `min_words` (at least 1), and `docs`: `dir` (folder relative to the repository root, without `.` nor `..`), `media_dir` (a single folder name), `archives_scope` (`"repository"` or `"documentation"`), optional keys refused by the versions before 1.9.0; and, since 1.10.0, `tags`: `prerelease_forms` (compliant pre-release forms, at least one, lowercase words), optional key refused by the versions before 1.10.0) |
| `repositories` | Repositories whose tracking is active (stopped repositories are not exported), sorted by `owner/repo`: `provider` (`github`), `repo` (`owner/repo`), `label` (display label, `""` without one), `branches` (tracked patterns, `[]` = all branches), and, **only for a repository that has them**, `flemme_after_days` and `dead_after_days` (own thresholds, in days) and `conventions_since` (1.7.0: own conventions start date, `"YYYY-MM-DD"`). These keys are optional in version 1 of the format: a file without own thresholds or date stays readable by the versions of Watchoo that do not know them; a file that contains `conventions_since` is refused by the versions before 1.7.0 |

**Never exported**: the settings specific to the machine or to the person (`name` of the file,
`github.*`: `gh` account and path of `gh`, which a file coming from elsewhere must never be able to
change; `git.*`: protocol of the local Git repositories, which depends on the credentials of the machine;
`web.*`: listen address, authentication, reverse proxy, accepted names; `diagnostic.*`;
`conventions.enabled`, a choice specific to the workspace, and `conventions.excluded_accounts`, which holds
logins), the token, the commits,
the contributors, the e-mails, the sync dates and any local path. The file therefore contains only
repository names, display labels and settings; it can still reveal the list of tracked students: share
it only with the people concerned.

### `watchoo config export`

- Without `-o`, writes the document to standard output (`watchoo config export > licence.json`), without
  a header card or tip, with or without `--json`.
- `-o file` (`--output`) creates the file in `0600` and prints one confirmation line (repositories and
  settings exported); it **refuses to overwrite** an existing file without `--force`. With `--json`,
  nothing is written to standard output: the file is the result.
- Contacts neither `gh` nor GitHub: everything comes from the database and the settings of the workspace.

### `watchoo config import file`

The file comes from another person: it is **untrusted data**, fully validated before changing anything.

1. **Validation** (nothing is changed on error, the message says why): valid JSON, a single object,
   1 MiB at most; `format` present and equal to `watchoo-config`; `version` equal to 1; no unknown key,
   at any level (a `github` or `web` key is therefore refused); 1,000 repositories at most; each
   repository: `provider` absent or `github`, `repo` of the form `owner/repo` (or GitHub URL), present
   only once (case-insensitive), valid `label` (like `repo add --label`), valid branch patterns (like
   `--branch`, no comma), valid own thresholds (like `--flemme` and
   `--dead`), valid conventions start date (like `--conventions-since`: `"YYYY-MM-DD"` or `""`);
   settings valid once merged with those of the
   workspace (same rules as `config.yaml`, e.g. `dead_after_days` greater than `flemme_after_days`),
   including once the set `WATCHOO_*` variables are reapplied: the preview therefore refuses everything
   that the write would refuse (e.g. `WATCHOO_ACTIVITY_DEAD_AFTER_DAYS=10` and an import of
   `flemme_after_days: 12`).
   `settings` and `repositories` are optional, each setting too: a file may contain only repositories,
   or only a time zone.
2. **Preview**, always printed before applying:
   - repositories **to add**, **to update** (display label, branches, own thresholds and conventions
     start date, before → after,
     `workspace` for the workspace threshold or date; stopped repository **to resume**), **unchanged**;
   - changed settings (key, current value → imported value); an identical setting is not listed. A
     setting masked by a set `WATCHOO_*` variable is compared with its value **in the files** (user file
     then `config.yaml`), not with the one of the variable: it is the one the import rewrites, and
     re-importing the same file changes nothing any more;
   - warnings: each `WATCHOO_*` variable that masks an imported setting, whether it changes or not (the
     value is written to `config.yaml`, or is already there, but the variable wins as long as it is
     set); `config.yaml` rewritten, therefore
     **its personal comments lost** (copy kept in `config.yaml.bak`).
3. **Confirmation**: on a terminal, `Apply these changes? [y/N]` (same answers as
   `repo remove`); otherwise (script, pipe, file read from standard input with `-`) the
   **`--yes` / `-y` option is required**: without it, the command stops before printing anything (the
   preview is then obtained with `--dry-run`). Without confirmation, nothing is changed. If the preview
   shows no change, the workspace says so ("Nothing to change: the workspace is already up to date.") without asking a
   question or requiring `--yes` in text output (in `--json` without `--dry-run`, `--yes` remains
   required).
4. **Applying**, in this order:
   - **settings**: if at least one setting changes, `.watchoo/config.yaml` is **rewritten** from the
     commented template of `watchoo init`, imported settings uncommented. The machine-specific keys
     already present in the file (`name`, `github.*`, `git.*`, `web.*`, `diagnostic.*`,
     `conventions.enabled`, `conventions.excluded_accounts`) and the common settings that the
     imported file does not give are **kept as is**. The old file is copied to
     `.watchoo/config.yaml.bak` (`0600`, replaced at each import). The new file is written next to it
     (`0600`), read back and validated, and compared with the expected values before replacing the old
     one (atomic write): on a mismatch, nothing is changed. If no setting changes, `config.yaml` is not
     touched;
   - **repositories**: like `repo import`. Access to each repository is checked with GitHub; a missing
     repository is added; an already tracked repository (looked up by its GitHub id then by
     `owner/repo`, so renames included) receives the `label` and the `branches` of the file, and its
     tracking **resumes if it was stopped** (like `repo add`). An empty `label` keeps the existing
     display label; `branches` absent (or `null`) keeps the existing branches, `[]` means all branches;
     `flemme_after_days` or `dead_after_days` absent (or `null`) keeps the existing threshold, `0` goes
     back to the workspace threshold; `conventions_since` absent (or `null`) keeps the existing date,
     `""` goes back to the date of the workspace.
     An inaccessible repository is reported without blocking the others. **No repository is ever
     removed** and no sync is started.
5. **Summary**: settings written (and `.bak` copy), then repositories added, already tracked, updated,
   inaccessible, in error (line "resumed" for the stopped repositories that
   resume), with the details of each failed repository (`repository #N  owner/repo: message`). Non-zero
   return code if some repositories remain inaccessible or in error (run the import again once access is
   granted: it is risk-free).

```
IMPORT  licence.json  (workspace "licence-projet", exported on 10/27/2026 with Watchoo v0.1.0)
Settings
  sync.interval              30m → 1h
  activity.dead_after_days   15 → 21
Repositories
  to add            18
  to update          1   bob/projet-licence (branches: all → main, dev)
  to resume          1   carol/projet-licence
  unchanged          3
! config.yaml will be rewritten from the "watchoo init" template: its personal comments will be lost (the old file is copied to config.yaml.bak)
(repository access will be checked against GitHub when applying)
Apply these changes? [y/N]
```

- **`--dry-run`**: validation and preview only, plus the access check of each repository (like
  `repo import --dry-run`); nothing is written, neither the database nor `config.yaml` (byte for byte).
  The preview without `--dry-run` does not check access (that is done when applying): a repository
  renamed on GitHub may appear there as "to add" and turn out to be "already tracked".
- **`--json`**: preview (with `--dry-run`) or summary in JSON format (see [JSON output](#json-output));
  without `--dry-run`, `--yes` remains required and the preview is not written separately.
- `-` instead of the file reads the document from standard input (`--yes` then required, standard input
  no longer being available for the question).
- Texts coming from the file (workspace name, labels, messages) are cleaned before being displayed, like
  those of the repositories.
- The rewritten settings apply to the commands run afterwards; an already open `watchoo serve` or
  `watchoo tui` takes them into account at its next launch.

## Interactive mode (TUI)

`watchoo tui` opens an interactive mode in the terminal (Bubble Tea). You browse the repositories, the
commits and the contributors with the keyboard, with a filter and a detail view, and you can start a
sync or add a repository to track from there. See
[Architecture](03-architecture.md#terminal-interactive-mode-tui).

- Only the `tui` command is interactive. All the others print a text table or JSON (`--json`), then hand
  back control.
- `watchoo tui` run outside a terminal stops with a clear message.
- While it is open, `watchoo tui` syncs automatically (30 min by default, can be disabled). One key
  starts a manual sync of all the repositories, another one of the selected repository.
- Colors are disabled outside a terminal and when `NO_COLOR` is set.
- `--interval` overrides `sync.interval` for the duration of the session: `0` for manual sync only,
  otherwise 15 min at least; without the option, the automatic sync follows `sync.auto` and
  `sync.interval`.
- During the TUI, logs are not written (they would break the display). `watchoo tui --json` is refused,
  like an extra argument; the command also stops if standard input or standard output is not a terminal.
- **Views**: Repositories (repository, last commit, author, branches, state), Commits (when,
  repository, author, short SHA, `+/-` of the commit, first line of the message; the last 200) and
  Contributors (name, login, commits, `+/-` totals, repositories, last commit). The
  **detail of a repository** shows the URL, the state and its thresholds, the last commit, the branches,
  the number of commits and their `+/-` lines, the last sync (or its error), stars and issues, the open
  pull requests and the last published release (V2), the last 15 commits (with their `+/-`) and the top
  5 contributors, with their `+/-` lines in the repository and their commits over the last 12 weeks
  ("12 WK" column) and a one-character curve per week, the most recent on the right ("PER
  WEEK": `▁` to `█`, or `.` `-` `=` `#` without Unicode; `▁` or `.` = no commit).
- **Detail of a contributor** (0.5.0, `enter` in the Contributors view; `esc` goes back to the
  list): name, login (the TUI displays no address, nor the one of the profile), number of attached Git
  identities (never the addresses); totals (commits, `+/-` lines or `-` if unknown, repositories, first
  and last commit); activity over 12 weeks (total and one-character curve per week, as
  above); its repositories (commits, `+/-`, curve per week, last commit, state of the repository); its
  15 last commits (when, repository, short SHA, `+/-`, first line of the message). Lines added and
  deleted are read in the local Git clone of each repository; `-` reports stats still
  unknown (unavailable clone), `+0/-0` a commit really without any line (reminder at the bottom of the
  detail). The detail scrolls with `↑` `↓`. **Selection**: the table of the main contributors of the
  detail of a repository is not selectable (the detail scrolls, it has no cursor); go through the
  Contributors view (filter `/`).
- **Header**: mini owl (green pupils), version and workspace, tabs, sync line (in progress with the
  progress, last sync, next automatic sync at HH:MM, or "manual sync (s)").
  When a new version of Watchoo is published, the line under the header announces it (see
  [New-version notice](#new-version-notice)).
- **Adding a repository** (`a`, Repositories view): an input line "Repository to track (owner/name
  or GitHub URL):" opens at the bottom of the screen; `enter` validates, `esc` cancels. The input accepts the same references as `watchoo repo add`
  (`owner/repo` or GitHub URL, same rules and same messages); an invalid reference is reported without
  calling anything and the input stays open to correct it (an input containing `@`, which can carry
  credentials, is cleared). The addition goes through the same core as `repo add` (access checked with
  GitHub, or without `gh` by reading the remote repository) and follows the rules of `repo add` without
  option: all branches, without label or own thresholds (label, branches and thresholds are set later
  with `repo add`, the import or the web Settings page). It runs in the background, one at a time.
  Summary at the bottom of the screen: "owner/name added" (or "owner/name already tracked: nothing to
  change", "owner/name already tracked: tracking resumed" for a stopped repository, "repository
  owner/name is inaccessible" with the reason, like `repo add`); the list is re-read, the repository selected and its sync started,
  like `S`. If a sync is already in progress, the addition still happens and the sync of the new
  repository starts at the end of the one in progress (a sync of all the repositories started in
  between replaces it). An already tracked repository is neither modified nor re-synced.
- **Sync**: only one at a time (an extra request is ignored and reported); at the end, a summary line
  (new commits, errors, repositories skipped or deferred with the local resume time, warnings about the
  pull requests, releases, tags or an unavailable local Git clone), then the views are reloaded. An
  automatic sync falling during a sync is skipped, the next one stays scheduled. Quitting during a sync
  cancels it: `watchoo tui` then waits for the engine to have closed its syncs
  ("sync canceled", a few seconds at most) before closing the database, so that no
  repository stays locked.

| Key | Effect |
| --- | --- |
| `tab` / `shift+tab`, `1` `2` `3` | Next / previous view, Repositories, Commits, Contributors views |
| `↑` `↓` or `k` `j`, `pgup` `pgdown`, `g` `G` | Move the selection, by page, to the start and end of the list |
| `enter` | Detail of the selected repository (Repositories or Commits view) or of the selected contributor (Contributors view, 0.5.0) |
| `esc` | Go back, or clear the filter |
| `/` | Filter the view (substring, case-insensitive); `enter` keeps the filter, `ctrl+u` clears it |
| `s` / `S` | Sync all the repositories / the selected repository |
| `a` | Add a repository to track (Repositories view); `enter` validates, `esc` cancels |
| `r` | Reload the database |
| `?` | Key help |
| `q`, `ctrl+c` | Quit (while typing a filter or a repository, only `ctrl+c` quits) |

## Output examples

`watchoo repo list`:

```
REPOSITORY         LAST COMMIT  AUTHOR  STATE
moi/projet-api     2 h ago      moi     -
moi/outils-reseau  5 d ago      bob     -
moi/scripts        9 d ago      moi     ~ 9 d
mon-org/site-web   42 d ago     alice   ⚠ 42 d
```

`-` = Active (0 to 5 days), `~` = Slacking (6 to 15 days), `⚠` = Dead (more than 15 days). See the
[activity rules](02-features.md#functional-rules).

## Typical workflow

Supervising the projects of a class of undergraduate students:

```bash
mkdir licence-projet
cd licence-projet
watchoo init
watchoo repo import promo-licence.csv
watchoo repo add retardataire/projet-licence --label "Zoé Leroy"
watchoo sync
watchoo repo list
watchoo contributors --since 7d
watchoo contributors --repo retardataire/projet-licence --weeks 6
watchoo serve
```

Sharing the configuration with a colleague, who imports it into their own workspace:

```bash
watchoo config export -o licence.json          # on your machine
watchoo config import licence.json --dry-run    # on the colleague's machine: preview
watchoo config import licence.json              # then apply, after confirmation
watchoo sync
```

From anywhere, check another workspace: `watchoo -C ~/supervision/master-projet status`.
