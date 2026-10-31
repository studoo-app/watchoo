# 5. CLI

[← Contents](README.md)

## Global options

| Option | Effect |
| --- | --- |
| `--json` | JSON output to make every command scriptable |
| `-C <répertoire>` / `--workspace <répertoire>` | Works on the workspace of this directory instead of the one found from the current directory |
| `--verbose` | Detailed logs (debug level) on the error output. Without it, the level comes from the `WATCHOO_LOG_LEVEL` variable (`debug`, `info`, `warn` by default, `error`). No short letter: `-v` would be confused with `--version` |

All commands except `init`, `version` and `update` work on a **workspace**: the `.watchoo/` directory found in the
current directory or its parents (see [Workspaces](10-workspaces.md)).

**Message language**: the CLI and the TUI speak **English by default**. The `WATCHOO_LANG`
variable (`en` or `fr`) picks the language; without it, the system locale decides (`LC_ALL`,
`LC_MESSAGES`, then `LANG`: `fr` prefix → French); failing that, English. The web interface
language is set separately, by the workspace's `lang` setting (see
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
| `watchoo repo add owner/repo [--branch main,dev,release/*] [--label "Alice Martin"] [--flemme 3] [--dead 10]` | Adds a repository to watch (all its branches by default), with an optional display label and own activity thresholds | MVP (thresholds: V2) |
| `watchoo repo import fichier.csv [--dry-run]` | Adds a list of repositories in one go (e.g. those of a class of students), checks access to each one and prints a summary | MVP |
| `watchoo repo add-org mon-org` | Adds all the repositories of an organization | V2 |
| `watchoo repo list` | Table: repository, last commit, author, state (Actif (Active) / Flemme (Slacking) / Dead) | MVP |
| `watchoo repo remove owner/repo [--keep-history]` | Stops tracking and deletes the repository history (and its local Git clone) after confirmation (`--keep-history` to keep them) | MVP |
| `watchoo sync [owner/repo]` | Syncs everything or a single repository | MVP |
| `watchoo log [--repo X] [--author Y] [--branch B] [--since 7d] [-n 20]` | Latest commits, filtered | MVP |
| `watchoo contributors [--repo X] [--since 30d \| --weeks 12]` | Contributor ranking; with `--weeks`, their commits week by week | MVP (`--weeks`: V2) |
| `watchoo prs [--repo X] [--state open\|merged\|closed\|all] [-n 20]` | Pull requests of the tracked repositories, from the most recently updated to the oldest | V2 |
| `watchoo releases [--repo X] [-n 20]` | Releases of the tracked repositories, then their tags | V2 |
| `watchoo status` | Summary: up-to-date, inactive and in-error repositories, source of the syncs (GitHub API or local Git repositories), remaining API quota, the `gh` account in use and its scopes (without showing the token) | MVP (source: V2) |
| `watchoo serve [--addr 127.0.0.1:8080] [--interval 30m]` | Starts the web interface and the periodic sync (30 min by default, 15 min minimum) | MVP |
| `watchoo tui [--interval 30m]` | Opens the interactive mode in the terminal | MVP |
| `watchoo config export [-o fichier] [--force]` | Writes the shareable configuration of the workspace (tracked repositories and common settings) to a JSON file, for the other people supervising the same group | After MVP |
| `watchoo update [--check] [--yes]` | Self-updates Watchoo from the GitHub releases after confirmation (archive verified by SHA-256 checksum, binary replaced in one go); `--check` only checks. Works outside a workspace and without `gh` (see [Self-update](#self-update-watchoo-update)) | 0.6.0 |
| `watchoo config import fichier [--dry-run] [--yes]` | Merges a shared configuration file into the workspace, after a preview: adds the missing repositories, updates the others, rewrites the common settings; never removes anything | After MVP |

### Details

- **`repo add`** accepts `owner/repo` or a full GitHub URL. Without `--branch`, **all branches** of the
  repository are tracked, including those created later. `--branch` restricts tracking to a list and to
  patterns (`--branch main,dev,release/*`); a new branch matching a pattern is tracked automatically. See
  [Architecture](03-architecture.md#tracked-branches).
  `--branch ""` means all branches. `--flemme N` and `--dead N` set the **own activity thresholds** of
  the repository, in days without a commit, instead of the workspace ones
  (`activity.flemme_after_days`, `activity.dead_after_days`); each one is optional and independent,
  `0` means the workspace threshold. A threshold is at most 3,650 days and, when both are given,
  `--dead` must be strictly greater than `--flemme` (otherwise refusal before any call to GitHub). An
  already tracked repository is not added twice: its label, its branches and its thresholds are updated
  if they are given, and tracking resumes if it had been stopped. The report quotes the own thresholds
  ("seuils : flemme 3 j, dead 10 j" ("thresholds: flemme 3 d, dead 10 d")). Access is checked with
  GitHub (one request); `repo add` does not start a sync. Without `gh` (V2), access is checked with
  `git ls-remote`, cloning nothing: a failure gives "accès refusé" ("access denied"); the repository is
  recorded without a GitHub ID (`provider_id` unknown, resolved at the first sync through the API) and
  with the URL `https://github.com/owner/repo`.
- **`repo list`** prints all the repositories of the workspace, stopped tracking included. The DÉPÔT
  column shows the label if there is one, otherwise `owner/repo`; the author is the one of the last
  commit. Gray states: `arrêté` (Stopped, tracking stopped), `erreur` (Error), `jamais` (Never synced),
  `archivé` (Archived), `vide` (Empty, synced without any commit).
- **`repo remove`** asks for confirmation (`[o/N]`) when standard input is a terminal; otherwise (script,
  pipe) the **`--yes` / `-y` option is required**. Any answer other than `o`, `oui`, `y` or `yes`, or a
  Ctrl+C during the question (taken into account immediately), cancels with a non-zero return code.
  Without `--keep-history`, the repository, its branches, commits and syncs are deleted from the database
  (the contributors remain), as well as its local Git clone (`repos/<name>` at the root of the workspace,
  V2) if it exists, and the clone of a same-named repository that takes this name is moved (see
  [Local Git repositories](03-architecture.md#local-git-repositories-v2)); with `--keep-history`, tracking
  stops (the repository is no longer synced by `watchoo sync`, it shows as `arrêté` (Stopped)), the
  history in the database **and** the clone are kept, and `repo add` resumes it.
- **`sync`** prints a table per repository (status `ok`, `erreur` (Error), `ignoré` (Skipped) if it is already
  being synced, `reporté` (Deferred) if the quota is low, with the resume time in local time), the
  summary and the remaining quota. `watchoo sync owner/repo` syncs that repository even if its tracking
  is stopped. Non-zero return code if a repository is in error. A synced repository whose pull
  requests, releases or tags could not be read, or whose local Git clone is unavailable with `gh` (line
  stats then read through the API, limited), stays `ok`: the warning is in the DETAILS column (and
  `warnings` in JSON), and does not change the return code. Without `gh` (V2), the summary ends with
  "Source : clones Git locaux (gh absent ou non connecté ; « gh auth login » pour passer par l'API GitHub)" ("Source: local Git repositories (gh absent or signed out; 'gh auth login' to go through the GitHub API)") and
  gives no quota; a repository that `git` cannot read is `erreur` with "accès refusé" ("access denied").
- **`--since`** accepts a relative duration: an integer followed by `d` (days), `h` (hours) or `m`
  (minutes), for example `7d`, `30d`, `12h`.
- **`log`** prints, for each commit, the date, the repository, the author, the short SHA, its lines
  (`+/-` column: "+12/-3", "+0/-0" for a commit really without any line, "-" if they are still
  unknown; 0.5.0) and the first line of the message.
- **`+/-` lines in the TUI** (0.5.0): per commit, "+12/-3", "+0/-0" or "-" (unknown); the totals
  (contributor, repository) sum the known commits and add "(N non comptés)" ("(N not counted)") when a
  part of the commits has none yet, "-" if none has.
- **`log -n`** (or `--limit`) limits the number of commits printed (20 by default, `0` = all). A commit
  present on several branches appears only once.
- **`log --author`** compares the GitHub login, the name or an e-mail of the author, **exactly** (no
  substring), ignoring the case of ASCII letters ("ÉLODIE" does not find "élodie").
- **`log --branch`** shows the commits **seen on that branch** during the syncs: those it shares with the
  reference branch (often `main`) are attached to it only if they were seen while comparing it
  (see [Architecture](03-architecture.md#incremental-sync)).
- **`contributors`** ranks the commit **authors** by number of commits. Lines added and deleted are read
  in the local Git clone of each repository, for all the commits (first import included); they sum the
  commits whose stats are known (see
  [Incremental sync](03-architecture.md#incremental-sync): only an unavailable clone leaves them
  unknown, except for the 100 commits per sync that the API then gives).
  **`--weeks N`** (1 to 52) limits the period to the last N weeks (Monday to Sunday, in the display
  time zone, the current week included) and gives, for each contributor of that period, their commits
  week by week: in text, one column per week (dated from the Monday, "12/10") then the total; in JSON,
  the `weekly` key. With `--repo`, only the commits of the repository count, each one once whatever its
  branch. `--weeks` and `--since` do not go together (refusal).
- **`prs`** lists the pull requests recorded by the syncs (see
  [Incremental sync](03-architecture.md#incremental-sync)), from the most recently updated to the
  oldest: update (relative date), repository, number, state (`ouverte` (Open), `brouillon` (Draft),
  `mergée` (Merged), `fermée` (Closed); closed = without merge), author (login), title (first line,
  cleaned like a commit message). `--state` filters: `open` (open ones, drafts included), `merged`,
  `closed` (closed without merge) or `all` (default). `-n` (or `--limit`): 20 by default,
  `0` = all.
- **`releases`** prints two tables: the **releases** (publication, or creation for a draft, as a
  relative date; repository, tag, title, state `publiée` (Published), `pré-release` (Pre-release) or
  `brouillon` (Draft), author), from the newest to the oldest, then the **tags** (repository, tag,
  short SHA of the targeted commit and date of that commit if it is in the database, otherwise `-`),
  from the most recent commit to the oldest. `-n` limits each table (20 by default, `0` = all).
  Drafts are only visible if the `gh` account has write access to the repository (GitHub rule).
- **`status`**: `gh` account and its scopes, **source** of the syncs (V2: "Source : API GitHub (gh
  connecté)" ("Source: GitHub API (gh signed in)") or "Source : clones Git locaux" ("Source: local Git
  repositories"), with the `gh auth login` reminder), remaining quota (reading it is free; "sans objet"
  ("not applicable") with the local Git repositories), repositories by state, repositories in error,
  last successful sync, tracked branches. Below 10% of remaining quota, the quota guard is reported
  ("Quota bas : synchros reportées jusqu'à HH:MM" ("Quota low: syncs deferred until HH:MM"),
  `quota_low` in JSON), with the same rule as the engine. An absent or signed-out `gh` is reported
  without failing the command. The source is the one a sync started now would take (`gh auth token`,
  without contacting GitHub). The token is never displayed.
- **Outputs**: an aligned text table, without borders, headers in uppercase; relative dates
  ("il y a 2 h" ("2 h ago")) in local time (`timezone` setting, otherwise the time zone of the system).
  Colors (headers in bold, gray states in gray) only on a terminal, without `NO_COLOR` nor `TERM=dumb`.
  On a terminal without Unicode, ASCII symbols and headers (`! 42 j`, `DEPOT`); redirected output stays
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
- **Outside a workspace**: every command except `init`, `version`, `update` and `help` stops with "aucun espace
  Watchoo ici, lancez « watchoo init » (ou indiquez un espace avec -C <répertoire>)" ("no Watchoo
  workspace here, run 'watchoo init' (or point to a workspace with -C <directory>)").
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
- `watchoo help [commande]` and the `-h` / `--help` option print the help in French. There is no
  `completion` command (the one `cobra` adds by default is disabled: its help is in English).
- **`--version`**, option of `watchoo` alone (`watchoo init --version` is refused), has the same effect
  as `watchoo version`, `--json` included.

## JSON output

With `--json`, every command that produces data writes **a single JSON object** (indented, followed by an
end of line) to standard output, and nothing else: no header card, no tip. Warnings, confirmation
questions and errors go to the error output; on error, standard output stays empty (except `repo import`
and `config import`, which write their summary then return a non-zero code if some repositories remain
inaccessible).

Common rules, which form a **stable contract** for scripts (a key is neither renamed nor removed without
an incompatible version: MINOR while Watchoo is in 0.y.z, MAJOR from 1.0.0, see [Skills](11-skills.md)):

- keys in English, in `snake_case`; all the keys are always present;
- dates in UTC, RFC 3339 to the second (`"2026-10-27T10:00:00Z"`), `null` if unknown;
- empty lists written `[]`, never `null`; a missing string is `""`;
- texts coming from the repositories (messages, names, labels) kept as is, escaped by the encoder;
- never a token. `tui` and `serve` refuse `--json`.

Top-level keys (the list is checked by a test, `internal/cli/json_schema_test.go`):

| Command | Top-level keys | Notes |
| --- | --- | --- |
| `watchoo init` | `name`, `root`, `dir`, `config_file`, `database`, `parent_workspace`, `github`, `source`, `warnings` | `github`: `host`, `login`, `token_source`, `scopes` (empty without `gh`); `source`: `api` or `git` (local Git repositories, without `gh`, V2); `parent_workspace` empty without a parent workspace |
| `watchoo version` | `version`, `commit`, `date`, `go`, `os`, `arch` | See below |
| `watchoo repo add` | `outcome`, `repository` | `outcome`: `added`, `already` or `updated`; `repository`: repository object |
| `watchoo repo import` | `file`, `format`, `dry_run`, `summary`, `entries` | `format`: `csv` or `list`; `summary`: `added`, `already_tracked`, `updated`, `inaccessible`, `errors`; each entry: `line`, `repo`, `outcome`, `repository` (`null` if inaccessible or in error), `error` |
| `watchoo repo list` | `repositories` | Each element: repository object plus `activity`, `commits`, `tracked_branches`, `last_commit` (commit object or `null`), `last_sync` (`null` if none) |
| `watchoo repo remove` | `action`, `repository`, `deleted_commits` | `action`: `deleted`, `stopped` or `already_stopped` |
| `watchoo sync` | `started_at`, `finished_at`, `summary`, `repositories`, `rate_limit`, `source` | `source`: `api`, `git` (local Git repositories, V2; `rate_limit` is then `null`) or `""` if the sync did not start; `summary`: `repositories`, `success`, `error`, `locked`, `deferred`, `new_commits`; each repository: `id`, `full_name`, `display_name`, `status`, `new_commits`, `tracked_branches`, `stats_fetched`, `duration_ms`, `error`, `resume_at`, `warnings` (messages in French: pull requests, releases or tags not synced, or local Git clone unavailable with `gh`, while the repository succeeded; `[]` otherwise); `stats_fetched`: commit stats computed in the clone, or requested from the API as a fallback |
| `watchoo log` | `commits` | From newest to oldest; commit objects |
| `watchoo contributors` | `contributors` | Each element: `id`, `name`, `login`, `display_name`, `avatar_url`, `commits`, `additions`, `deletions`, `repositories`, `last_commit_at`, `weekly` (with `--weeks N`: N `week` objects (Monday, `YYYY-MM-DD`, display time zone) and `count`, from the oldest to the newest; `[]` without `--weeks`) |
| `watchoo prs` | `pull_requests` | From the most recently updated to the oldest; pull request objects |
| `watchoo releases` | `releases`, `tags` | Release objects (from the newest to the oldest) and tag; `-n` limits each list |
| `watchoo status` | `workspace`, `github`, `source`, `rate_limit`, `rate_limit_error`, `quota_low`, `repositories`, `errors`, `last_synced_at`, `tracked_branches` | `workspace`: `name`, `root`; `github`: `ok`, `host`, `login`, `token_source`, `scopes`, `error`; `source` (V2): `kind` (`api`, `git`, or `""` if `gh` is down) and `reason` (why the local Git repositories, or the `gh` error); `repositories`: `total`, `active`, `flemme`, `dead`, `error`, `never_synced`, `archived`, `empty`, `stopped`; each error: `repo`, `message` |
| `watchoo update` | `current`, `latest`, `update_available`, `release_url` | Only with `--check` (see [Self-update](#self-update-watchoo-update)); `watchoo update --json` without `--check` is refused |
| `watchoo config export` | `format`, `version`, `exported_at`, `watchoo_version`, `workspace`, `settings`, `repositories` | The sharing document itself (see [Configuration sharing](#configuration-sharing-watchoo-config)), with or without `--json`; with `-o`, it goes to the file and standard output only receives one confirmation line in text (nothing with `--json`) |
| `watchoo config import` | `file`, `dry_run`, `applied`, `source`, `settings`, `repositories`, `summary`, `warnings` | `source`: `workspace_name`, `watchoo_version`, `exported_at` (as written in the file); `settings`: `changes` (each `key`, `before`, `after`, `env_var`), `rewrite`, `written`, `backup_file`; each repository: `index`, `repo`, `action` (`add`, `update`, `unchanged`, `inaccessible`, `error`), `reenable`, `before` (`label`, `branches`, `flemme_after_days`, `dead_after_days` (`null` = workspace threshold), `enabled`; `null` for an addition), `after`, `outcome` (like `repo import`, `""` as long as nothing is applied), `error`; `summary`: `added`, `already_tracked`, `updated`, `reenabled` (among `updated`), `inaccessible`, `errors` (predicted with `--dry-run`, obtained otherwise); each warning: `code` (`env_override`, `comments_lost`), `key`, `message` |

Shared objects:

- **repository**: `id`, `full_name`, `owner`, `name`, `label`, `display_name`, `url`, `default_branch`,
  `branch_patterns` (`[]` = all branches), `flemme_after_days` and `dead_after_days` (own thresholds of
  the repository, `null` = workspace threshold; the applied thresholds are in the activity object),
  `enabled`, `archived`, `stars`, `open_issues`, `last_synced_at`, `last_commit_at`;
- **commit**: `repository` (`id`, `full_name`, `display_name`), `sha`, `short_sha`, `message`, `url`,
  `author` (`name`, `login`, `display_name`), `authored_at`, `committed_at`, `additions`, `deletions`,
  `stats_known` (0.5.0: `true` if `additions` and `deletions` have been computed, even at 0; `false` if
  they are still unknown, `additions` and `deletions` being 0 then);
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
╭───────────────────────────────────────────────────────────────────╮
│  █▄    ▄█  Watchoo v0.1.0                                         │
│  █ ●██● █  Qui a travaillé, sur quoi, et quand ?                  │
│  ▀██▙▟██▀                                                         │
│                                                                   │
│  Astuce : « watchoo repo add owner/repo » puis « watchoo sync ».  │
╰───────────────────────────────────────────────────────────────────╯

● Espace : licence-projet (~/supervision/licence-projet)

Watchoo supervise des dépôts GitHub, surtout privés, et répond à trois questions :
…
```

| Output | Card subtitle and tip | After the card |
| --- | --- | --- |
| General help: `watchoo`, `watchoo help`, `watchoo --help` / `-h` | "Qui a travaillé, sur quoi, et quand ?" ("Who worked, on what, and when?"); tip "`watchoo init` fait de ce répertoire un espace de supervision" ("`watchoo init` turns this directory into a workspace") if no workspace is found, "`watchoo repo add owner/repo` puis `watchoo sync`" ("`watchoo repo add owner/repo` then `watchoo sync`") in a workspace, "`watchoo help <commande>` détaille une commande" ("`watchoo help <command>` details a command") if the workspace is unreadable | An empty line, the workspace status line, an empty line, then the usual help |
| `watchoo version`, `watchoo --version` | Go version and platform (`go1.27.3 · darwin/arm64`); no tip | Nothing |
| `watchoo init` successful | "Espace de supervision « nom » créé" ("Workspace "name" created"); tip "`watchoo repo add owner/repo` ajoute un premier dépôt à suivre" ("`watchoo repo add owner/repo` adds a first repository to track") | An empty line, then the report as status lines: workspace, settings, database, `gh` account and its scopes (without `gh`: "Source : clones Git locaux" ("Source: local Git repositories")) |
| `watchoo status` | "État de l'espace « nom »" ("State of the workspace "name""); tip "`watchoo sync` met à jour les dépôts suivis" ("`watchoo sync` updates the tracked repositories") | An empty line, then the usual summary |

- **Scripts**: the card is printed only if standard output is a terminal, and never with `--json`.
  Redirected output (file, pipe) stays the same: help without a card, `watchoo version` on a single line
  (`watchoo v0.1.0 (commit 1a2b3c4, 2026-10-27, go1.27.3, darwin/arm64)`), usual report of `init`. The
  error output (errors, warnings) never has a card, nor has the help of subcommands
  (`watchoo help init`).
- **Workspace status line** (general help): `● Espace : <nom> (<racine>)`, with `~` for the home
  directory, or `● Aucun espace de supervision ici`. The workspace is looked up as the other commands
  would do (current directory and its parents, `-C`, `WATCHOO_DIR`), locally, without running
  `gh`. The lookup never fails the help: an error (invalid settings, non-existent `-C` directory…) is
  summarized on the status line, `● Espace : <message>`.
- **Width**: that of the content, without exceeding 80 columns or the width of the terminal (`COLUMNS`
  variable if it is a positive integer, otherwise the one given by the system). Text that is too long is
  broken at spaces. On a terminal of less than 40 columns, no card: the output is that of a redirected
  output. An accented letter takes one column, whether precomposed (`é`) or decomposed (`e` followed by
  a combining accent, a common form in directory names created under macOS, hence in the default name of
  a workspace).
- **Unicode**: if the locale is UTF-8 (first non-empty variable among `LC_ALL`, `LC_CTYPE` and `LANG`)
  or in Windows Terminal (`WT_SESSION`). Otherwise, the card and the status lines are pure ASCII:
  borders `+`, `-` and `|`, owl `ascii-mini-pur.txt`, straight quotes instead of « », accents removed,
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
Nouvelle version de Watchoo disponible : v0.5.0 → v0.6.0. Mettez-la à jour avec « watchoo update ».
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
  for `watchoo update` itself, `watchoo serve` and `watchoo tui` (which have their own), the help and
  the completion.
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
  "Nouvelle version de Watchoo disponible : v0.6.1 → v0.6.2. Quittez puis lancez « watchoo update »."
  ("Nouvelle version de Watchoo disponible : v0.6.1 → v0.6.2. Quit and then run "watchoo update".")
  (in the Docker image: "Changez le tag de l'image Docker pour la mettre à jour." ("Change the tag of the Docker image to update it.")); on a
  terminal too narrow, a short form: "Watchoo v0.6.2 disponible : quittez puis lancez « watchoo update »." ("Watchoo v0.6.2 available: quit then run "watchoo update".").
  Failure or delay exceeded: nothing. No extra notice on the error output when quitting. Same exceptions
  as the commands: `WATCHOO_NO_UPDATE_CHECK`, a version that is not a release version.

### `watchoo update --check`

Checks only, installing nothing: installed version, last published version and, if there is a newer
one, the link to its release. Return code 0 whether an update exists or not; non-zero if the last
version could not be read (network). With a development version, the comparison is impossible: the
last version is printed, `update_available` is `false`.

```
$ watchoo update --check
Version installée : v0.5.0
Dernière version : v0.6.0
Nouvelle version disponible : https://github.com/studoo-app/watchoo/releases/tag/v0.6.0
Mettez-la à jour avec « watchoo update ».
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
2. Reads the last version. Already up to date: "Watchoo v0.6.0 est à jour (dernière version publiée : v0.6.0)." ("Watchoo v0.6.0 is up to date (last published version: v0.6.0)."), code 0.
3. Prints `v0.5.0 → v0.6.0`, the link to the release and to the `CHANGELOG.md` of that version, then
   asks for confirmation (`[o/N]`) on the error output. As for `repo remove`, `--yes` / `-y` skips
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
| `flemme` | no | Own Flemme (Slacking) threshold of the repository, in days without a commit, like `--flemme` (`0` = workspace threshold) |
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
  ajoutés          18
  déjà suivis       3
  mis à jour        1
  inaccessibles     2   carol/projet-licence, dave/projet-licence
  lignes en erreur  0
```

- An **empty** `label`, `branches`, `flemme` or `dead` cell means "not provided": the existing value is
  kept (an import therefore cannot set a repository back to "all branches"; use
  `repo add owner/repo --branch ""`). To go back to the workspace threshold, write `0` in `flemme` or
  `dead`. An unreadable, negative threshold, beyond 3,650 days, or a `dead` lower than or equal to the
  `flemme` of the same line is an error line.
- `-` instead of the file reads the list from standard input.
- The summary then details each inaccessible or in-error repository, with its line number.
- `--dry-run`: runs the validation and the access checks, prints the summary ("à ajouter" ("to add")
  instead of "ajoutés" ("added")), but records nothing.
- `--json`: summary in JSON format.
- Non-zero return code if some repositories remain inaccessible, for scripts.
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
    "timezone": ""
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
| `version` | Version of the format, `1` (required). A greater version is refused: "fichier créé par une version plus récente de Watchoo" ("file created by a newer version of Watchoo") |
| `exported_at`, `watchoo_version` | Export date (UTC, RFC 3339) and version of Watchoo that made it; informative, optional |
| `workspace.name` | Name of the exported workspace; informative: the import never renames the workspace |
| `settings` | Common settings, values **in effect** in the exported workspace (files and `WATCHOO_*` variables included): `sync.auto`, `sync.interval` (Go duration in quotes: `"30m"`, `"1h30m"`, `"0"`), `sync.parallelism`, `activity.flemme_after_days`, `activity.dead_after_days`, `branches.exclude`, `timezone` |
| `repositories` | Repositories whose tracking is active (stopped repositories are not exported), sorted by `owner/repo`: `provider` (`github`), `repo` (`owner/repo`), `label` (display label, `""` without one), `branches` (tracked patterns, `[]` = all branches), and, **only for a repository that has them**, `flemme_after_days` and `dead_after_days` (own thresholds, in days). These two keys are optional in version 1 of the format: a file without own thresholds stays readable by the versions of Watchoo that do not know them |

**Never exported**: the settings specific to the machine or to the person (`name` of the file,
`github.*`: `gh` account and path of `gh`, which a file coming from elsewhere must never be able to
change; `git.*`: protocol of the local Git repositories, which depends on the credentials of the machine;
`web.*`: listen address, authentication, reverse proxy, accepted names), the token, the commits,
the contributors, the e-mails, the sync dates and any local path. The file therefore contains only
repository names, display labels and settings; it can still reveal the list of tracked students: share
it only with the people concerned.

### `watchoo config export`

- Without `-o`, writes the document to standard output (`watchoo config export > licence.json`), without
  a header card or tip, with or without `--json`.
- `-o fichier` (`--output`) creates the file in `0600` and prints one confirmation line (repositories and
  settings exported); it **refuses to overwrite** an existing file without `--force`. With `--json`,
  nothing is written to standard output: the file is the result.
- Contacts neither `gh` nor GitHub: everything comes from the database and the settings of the workspace.

### `watchoo config import fichier`

The file comes from another person: it is **untrusted data**, fully validated before changing anything.

1. **Validation** (nothing is changed on error, the message says why): valid JSON, a single object,
   1 MiB at most; `format` present and equal to `watchoo-config`; `version` equal to 1; no unknown key,
   at any level (a `github` or `web` key is therefore refused); 1,000 repositories at most; each
   repository: `provider` absent or `github`, `repo` of the form `owner/repo` (or GitHub URL), present
   only once (case-insensitive), valid `label` (like `repo add --label`), valid branch patterns (like
   `--branch`, no comma), valid own thresholds (like `--flemme` and
   `--dead`); settings valid once merged with those of the
   workspace (same rules as `config.yaml`, e.g. `dead_after_days` greater than `flemme_after_days`),
   including once the set `WATCHOO_*` variables are reapplied: the preview therefore refuses everything
   that the write would refuse (e.g. `WATCHOO_ACTIVITY_DEAD_AFTER_DAYS=10` and an import of
   `flemme_after_days: 12`).
   `settings` and `repositories` are optional, each setting too: a file may contain only repositories,
   or only a time zone.
2. **Preview**, always printed before applying:
   - repositories **to add**, **to update** (display label, branches and own thresholds, before → after,
     `workspace` for the workspace threshold; stopped repository **to resume**), **unchanged**;
   - changed settings (key, current value → imported value); an identical setting is not listed. A
     setting masked by a set `WATCHOO_*` variable is compared with its value **in the files** (user file
     then `config.yaml`), not with the one of the variable: it is the one the import rewrites, and
     re-importing the same file changes nothing any more;
   - warnings: each `WATCHOO_*` variable that masks an imported setting, whether it changes or not (the
     value is written to `config.yaml`, or is already there, but the variable wins as long as it is
     set); `config.yaml` rewritten, therefore
     **its personal comments lost** (copy kept in `config.yaml.bak`).
3. **Confirmation**: on a terminal, `Appliquer ces changements ? [o/N]` (same answers as
   `repo remove`); otherwise (script, pipe, file read from standard input with `-`) the
   **`--yes` / `-y` option is required**: without it, the command stops before printing anything (the
   preview is then obtained with `--dry-run`). Without confirmation, nothing is changed. If the preview
   shows no change, the workspace says so ("Rien à changer" ("Nothing to change")) without asking a
   question or requiring `--yes` in text output (in `--json` without `--dry-run`, `--yes` remains
   required).
4. **Applying**, in this order:
   - **settings**: if at least one setting changes, `.watchoo/config.yaml` is **rewritten** from the
     commented template of `watchoo init`, imported settings uncommented. The machine-specific keys
     already present in the file (`name`, `github.*`, `git.*`, `web.*`) and the common settings that the
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
     back to the workspace threshold.
     An inaccessible repository is reported without blocking the others. **No repository is ever
     removed** and no sync is started.
5. **Summary**: settings written (and `.bak` copy), then repositories added, already tracked, updated,
   inaccessible, in error (line "dont repris" ("of which resumed") for the stopped repositories that
   resume), with the details of each failed repository (`dépôt n° N  owner/repo : message`). Non-zero
   return code if some repositories remain inaccessible or in error (run the import again once access is
   granted: it is risk-free).

```
IMPORT  licence.json  (espace « licence-projet », exporté le 27/10/2026 avec Watchoo v0.1.0)
Réglages
  sync.interval              30m → 1h
  activity.dead_after_days   15 → 21
Dépôts
  à ajouter         18
  à mettre à jour    1   bob/projet-licence (branches : toutes → main, dev)
  à reprendre        1   carol/projet-licence
  inchangés          3
! config.yaml sera réécrit : ses commentaires personnels seront perdus (copie dans config.yaml.bak)
Appliquer ces changements ? [o/N]
```

- **`--dry-run`**: validation and preview only, plus the access check of each repository (like
  `repo import --dry-run`); nothing is written, neither the database nor `config.yaml` (byte for byte).
  The preview without `--dry-run` does not check access (that is done when applying): a repository
  renamed on GitHub may appear there as "à ajouter" ("to add") and turn out to be "déjà suivi"
  ("already tracked").
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
- **Views**: Dépôts (Repositories) (repository, last commit, author, branches, state), Commits (when,
  repository, author, short SHA, `+/-` of the commit, first line of the message; the last 200) and
  Contributeurs (Contributors) (name, login, commits, `+/-` totals, repositories, last commit). The
  **detail of a repository** shows the URL, the state and its thresholds, the last commit, the branches,
  the number of commits and their `+/-` lines, the last sync (or its error), stars and issues, the open
  pull requests and the last published release (V2), the last 15 commits (with their `+/-`) and the top
  5 contributors, with their `+/-` lines in the repository and their commits over the last 12 weeks
  ("12 SEM." ("12 WKS.") column) and a one-character curve per week, the most recent on the right ("PER
  WEEK" ("PER WEEK"): `▁` to `█`, or `.` `-` `=` `#` without Unicode; `▁` or `.` = no commit).
- **Detail of a contributor** (0.5.0, `entrée` in the Contributeurs view; `échap` goes back to the
  list): name, login (the TUI displays no address, nor the one of the profile), number of attached Git
  identities (never the addresses); totals (commits, `+/-` lines or `-` if unknown, repositories, first
  and last commit); activity over 12 weeks (total and one-character curve per week, as
  above); its repositories (commits, `+/-`, curve per week, last commit, state of the repository); its
  15 last commits (when, repository, short SHA, `+/-`, first line of the message). Lines added and
  deleted are read in the local Git clone of each repository; `-` reports stats still
  unknown (unavailable clone), `+0/-0` a commit really without any line (reminder at the bottom of the
  detail). The detail scrolls with `↑` `↓`. **Selection**: the table of the main contributors of the
  detail of a repository is not selectable (the detail scrolls, it has no cursor); go through the
  Contributeurs view (filter `/`).
- **Header**: mini owl (green pupils), version and workspace, tabs, sync line (in progress with the
  progress, last sync, next automatic sync at HH:MM, or "Synchro manuelle (s)" ("Manual sync (s)")).
  When a new version of Watchoo is published, the line under the header announces it (see
  [New-version notice](#new-version-notice)).
- **Adding a repository** (`a`, Dépôts view): an input line "Dépôt à suivre (owner/nom ou URL
  GitHub) :" ("Repository to track (owner/name or GitHub URL):") opens at the bottom of the screen;
  `entrée` validates, `échap` cancels. The input accepts the same references as `watchoo repo add`
  (`owner/repo` or GitHub URL, same rules and same messages); an invalid reference is reported without
  calling anything and the input stays open to correct it (an input containing `@`, which can carry
  credentials, is cleared). The addition goes through the same core as `repo add` (access checked with
  GitHub, or without `gh` by reading the remote repository) and follows the rules of `repo add` without
  option: all branches, without label or own thresholds (label, branches and thresholds are set later
  with `repo add`, the import or the web Settings page). It runs in the background, one at a time.
  Summary at the bottom of the screen: "owner/nom ajouté" ("owner/name added") (or "déjà suivi"
  ("already tracked"), "suivi repris" ("tracking resumed") for a stopped repository, "inaccessible"
  with the reason, like `repo add`); the list is re-read, the repository selected and its sync started,
  like `S`. If a sync is already in progress, the addition still happens and the sync of the new
  repository starts at the end of the one in progress (a sync of all the repositories started in
  between replaces it). An already tracked repository is neither modified nor re-synced.
- **Sync**: only one at a time (an extra request is ignored and reported); at the end, a summary line
  (new commits, errors, repositories skipped or deferred with the local resume time, warnings about the
  pull requests, releases, tags or an unavailable local Git clone), then the views are reloaded. An
  automatic sync falling during a sync is skipped, the next one stays scheduled. Quitting during a sync
  cancels it: `watchoo tui` then waits for the engine to have closed its syncs
  ("synchro annulée" ("sync cancelled"), a few seconds at most) before closing the database, so that no
  repository stays locked.

| Key | Effect |
| --- | --- |
| `tab` / `maj+tab`, `1` `2` `3` | Next / previous view, Dépôts, Commits, Contributeurs views |
| `↑` `↓` or `k` `j`, `pg.préc` `pg.suiv`, `g` `G` | Move the selection, by page, to the start and end of the list |
| `entrée` | Detail of the selected repository (Dépôts or Commits view) or of the selected contributor (Contributeurs view, 0.5.0) |
| `échap` | Go back, or clear the filter |
| `/` | Filter the view (substring, case-insensitive); `entrée` keeps the filter, `ctrl+u` clears it |
| `s` / `S` | Sync all the repositories / the selected repository |
| `a` | Add a repository to track (Dépôts view); `entrée` validates, `échap` cancels |
| `r` | Reload the database |
| `?` | Key help |
| `q`, `ctrl+c` | Quit (while typing a filter or a repository, only `ctrl+c` quits) |

## Output examples

`watchoo repo list`:

```
DÉPÔT                 DERNIER COMMIT     AUTEUR      ÉTAT
moi/projet-api        il y a 2 h         moi         -
moi/outils-reseau     il y a 5 j         bob         -
moi/scripts           il y a 9 j         moi         ~ 9 j
mon-org/site-web      il y a 42 j        alice       ⚠ 42 j
```

`-` = Actif (0 to 5 days), `~` = Flemme (6 to 15 days), `⚠` = Dead (more than 15 days). See the
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
watchoo config export -o licence.json          # chez soi
watchoo config import licence.json --dry-run    # chez la collègue : aperçu
watchoo config import licence.json              # puis application, après confirmation
watchoo sync
```

From anywhere, check another workspace: `watchoo -C ~/supervision/master-projet status`.
