# 9. Testing strategy

[← Contents](README.md)

Principle: **the CI depends on no external service.** No real `gh`, no call to the GitHub API, no
token. Everything that leaves the binary is simulated.

## Overview

| What is tested | How | Where |
| --- | --- | --- |
| `gh` command (token, sign-in state) | **Mock of `gh`**: interface + fake binary (see below) | `internal/auth/` |
| GitHub API | Programmable fake GitHub server (`githubtest`, on `httptest`) | `internal/provider/github/` |
| Local Git repositories (V2) | Real Git repositories created with `git` in a temporary directory and served over `file://` (`gittest`); tests skipped only if `git` is absent (CI has it) | `internal/provider/git/`, `internal/sync/`, `internal/cli/` |
| Workspaces | Temporary directories: `init`, refusal if already initialized, search in the parents, `-C`, `WATCHOO_DIR`, nested workspace, `0700`/`0600` permissions | `internal/workspace/` |
| SQLite database and sqlc queries | Temporary SQLite database per test, goose migrations applied | `internal/store/` |
| Bulk import | Test CSV and text files: malformed line, duplicates, unreachable repository (`404`), idempotent rerun, `--dry-run` without writing | `internal/cli/` + `internal/sync/` |
| Sync engine | In-memory fake Provider (`providertest`) + temporary database | `internal/sync/` |
| Watchoo releases (self-update, 0.6.0) | Fake releases server (`selfupdatetest`, on `httptest`): redirect of `/latest`, archives and checksums in memory | `internal/selfupdate/` |
| CLI | Running the cobra commands, checking the text and JSON output | `internal/cli/` |
| Web | `httptest` on the chi router, checking the pages and fragments | `internal/web/` |
| TUI | Tests of the Bubble Tea model: messages sent, resulting state and rendering | `internal/tui/` |
| End to end | The full CLI with the fake `gh`, the real provider against `githubtest` and a temporary database | `internal/cli/e2e_test.go` |

## Mock of the `gh` command

Watchoo depends on `gh` to obtain the token, and to choose between the GitHub API and the local Git
clones (see [Architecture](03-architecture.md#authentication-gh-optional)). So that the CI needs
neither `gh` nor a GitHub account, `gh` is simulated at two levels.

### Level 1 — Go interface (unit tests)

Access to `gh` goes through a small interface in `internal/auth/`, for example:

- get the token;
- get the sign-in state (account, scopes).

The real implementation runs `gh` (`os/exec`). The tests of the other packages (sync, CLI, web, TUI)
use a **fake in-memory implementation** that returns the wanted response. No process is launched.

### Level 2 — fake `gh` binary (tests of the real implementation)

To test the code that really launches `gh` (arguments, reading of the output, return codes, errors), a
**fake `gh` executable** is used:

- a small Go program in `internal/auth/testdata/fakegh/`, compiled at the beginning of the tests;
- the path of `gh` is set in the config (`github.gh_path`, `gh` by default), which lets the tests point
  to the fake binary without touching the `PATH`;
- its behavior is chosen by an environment variable specific to the tests (for example
  `FAKEGH_SCENARIO`).

Covered scenarios:

| Scenario | Behavior of the fake `gh` | What Watchoo must do |
| --- | --- | --- |
| Signed in | `auth status` succeeds, `auth token` returns a fake token | Use the token |
| Not signed in | `auth status` fails with the message of `gh` | `init` and `status` point to `gh auth login` |
| `gh` missing | Nonexistent path | Clear message: install `gh` |
| Several accounts | `auth token --user X` returns a different token | Honor `github.user`; refuse a `github.user` written with a different casing (`--user` distinguishes uppercase) |
| `GH_TOKEN` only (Docker, CI) | `auth status` knows the account only through `GH_TOKEN`; `auth token` without `--user` returns that token, `auth token --user X` fails | Use the token without `--user`; refuse a `github.user` that `gh` only knows through `GH_TOKEN` |
| Token refused | `auth status` gives the `error` state with `HTTP 401: Bad credentials` | `init` and `status` point to `gh auth login` |
| GitHub unreachable | `auth status` gives the `error` state (network error) or `timeout` | Clear error, without suggesting `gh auth login` |
| Token changed between two syncs | Two calls return two different tokens | Use the new token without restarting |
| Unexpected output / timeout | Empty output, or `gh` that does not answer | Clear error, maximum delay enforced |

Rules:

- the fake token is a dummy value (e.g. `gho_fake_for_tests`);
- a test checks that this fake token **never appears in the logs** nor in the CLI, web or TUI output.

## Simulated GitHub API

Two fakes, written in phase 2 and reused by the following phases:

| Fake | Package | Used to test |
| --- | --- | --- |
| Programmable GitHub server | `internal/provider/github/githubtest` | The real GitHub provider (transport, ETag cache, quota, errors); in phase 3, the sync engine end to end |
| In-memory `Provider` | `internal/provider/providertest` | The sync engine, the CLI, the web and the TUI, without HTTP |

- **`githubtest`**: an `httptest` server on `127.0.0.1` that simulates the routes used (repository,
  branches, commits, commit with statistics, compare, pull requests, releases, tags, `/rate_limit`)
  from a state prepared by the test: repositories, commit graph (merges included), branches,
  force-push, rename or transfer (`301`), access refused (`404`) or SSO (`403`), empty repository
  (`409`). It reproduces the JSON format of GitHub, the pagination (`Link` header), the ETags and the
  `304`s (without quota), the quota (`X-RateLimit-*` headers, exhaustion, reset), the secondary limits
  (`Retry-After`), and keeps a log of the received requests (without the value of `Authorization`). A
  response can also be forced for a given path.
- **`providertest`**: implements `provider.Provider` with the same semantics as the GitHub provider
  (validation, order of the commits, `Stop`, `Since`, `Limit`, empty repository, `ErrUnknownBase`,
  renames), plus the injection of errors, the counting of calls and a simulated quota.
- `internal/provider/github/testdata/`: a few dummy JSON responses in the exact format of GitHub, for
  the tests of the conversion of types.
- **Scenarios** (the real provider against `githubtest`,
  `internal/provider/github/scenario_test.go`): pagination of the branches and the commits; incremental
  sync (first import through the list of commits, new branch through compare from the head of the
  default branch, including a branch that has merged `main`, compare between the old and the new head,
  including a merge that brings commits older than the old head, and the loss of commits that stopping
  at the first known SHA would cause); force-push (`ErrUnknownBase`, then fallback); `304` responses
  with ETag (quota not counted, cache shared between two providers of the same token and cleared by
  another token); exhausted quota; secondary limit; `404` on a private repository without access;
  `403` tied to SSO; `401`; renamed or transferred repository; empty repository; archived repository;
  4 repositories synced in parallel (under `-race`).
- **Read-only test**: the simulated server makes the test fail as soon as it receives a `POST`,
  `PATCH`, `PUT` or `DELETE` request, and a test checks that the transport of the provider refuses them
  before sending.
- **Leak test**: in all these scenarios, the logs (captured at Debug level) and the errors contain
  neither the fake token, nor an `Authorization` header, nor a commit message, name or email.

## Phase 3: engine, CLI and TUI

- **Store** (`internal/store/`, temporary database): sync lock (8 concurrent goroutines, only one takes
  it; stale `running` closed as "synchro interrompue" ("sync interrupted")), writing of a sync in one
  transaction (rolled back entirely on error), `INSERT … ON CONFLICT DO NOTHING`, identity merging,
  `last_commit_at` on the tracked branches, read views (repository, author, branch, period, limit
  filters; ranking; summaries).
- **Activity and import** (`internal/activity/`, `internal/repoimport/`, no database): global and
  per-repository thresholds, 5/6 and 15/16 day bounds, calendar days (midnight crossing in Paris,
  daylight saving time), priority of the gray states; CSV (BOM, CRLF, quotes, columns in disorder),
  plain list, comments, duplicates, each error with its line number, no login credentials in the
  messages; `flemme` and `dead` columns (empty cell, `0`, unreadable value, Dead less than or equal to
  Flemme).
- **Per-repository thresholds**: creation, update, threshold kept when not provided and reset to the
  workspace threshold with `0` (`internal/sync`, `DryRun` included), `repo add --flemme / --dead` and
  import columns (`internal/cli`, state recomputed by `repo list`), export, preview and import of the
  sharing (`internal/configshare`), form of the repository page (`internal/web`: saving, rejected
  input, `404`, CSRF).
- **Per-contributor activity in a repository** (V2): commits per week and per contributor
  (`activity.CountContributorWeeks`: timezone, bounds of the window, missing contributor) and
  single-line curve (`activity.Sparkline`, Unicode and ASCII); `store.AuthorCommitTimes` filtered by
  repository (a commit on two branches counted once, other repositories excluded); chart of the
  repository page (`TestRepoContributorsChart`: series of that repository only, absent from the
  fragments; repository without a commit over the period); detail of the TUI ("12 SEM." ("12 WKS.")
  and "PAR SEMAINE" ("PER WEEK") columns, fake `DataSource` and real database); `watchoo contributors
  --weeks` (text, `--repo`, `weekly` JSON, `[]` without the option, bounds and refusal with `--since`,
  `TestContributorsWeeks`).
- **Contributor page** (0.5.0): store (`TestContributorProfile`: totals, identities counted without
  address, contributor without a commit, unknown → `ErrNotFound`; `TestContributorRepos`; `AuthorID`
  filter of `ListRecentCommits` / `CommitPage`; `ContributorCommitTimes`; `AuthorProviderID` filter of
  the pull requests); `activity.CountRepoWeeks`; per-repository chart (`buildRepoChart`); web page
  (`internal/web/contributor_test.go`: content, totals and "—", repositories and badges, paginated
  commits, pull requests matched by account, card absent without an account, contributor without a
  commit, `404`, HTMX fragments, hostile names and messages escaped, no email address nor name in the
  log, links from the ranking, the repository page, the history and the dashboard; step "7 bis" of the
  web end-to-end test); TUI (opening with `entrée` (Enter) in the Contributors view, rendering,
  `échap` (Esc), help, fake `DataSource` and real database).
- **Links to the profiles** (0.5.0): `html_url` read by the GitHub provider (`toPerson`), empty for the
  local Git provider; `profile_url` column (migration `00005` on an existing database, value kept when
  an identity arrives without an address); "GitHub" link present with a known login, absent without an
  account, non-`https://` address refused (`profileURL`). Completion of the addresses (0.6.1):
  `ProfileURL(login)` of the GitHub, local Git and `providertest` providers (normal, empty, forbidden
  characters, bot login; no request), `provider.ValidLogin`; `store.BackfillProfileURLs` (only the
  empty addresses of the forge contributors with a login are completed, known addresses kept, second
  pass without effect); sync (address cleared as before 0.5.0, completed without a new commit by the
  API then by the local Git clones, no login in the logs).
- **Pull requests, releases and tags** (V2): GitHub provider against `githubtest` (conversion, order,
  pagination and caps of 1,000, early stop `UpdatedSince` on a single page, `Limit`, free `304`s,
  empty repository, `409`, unusable response, `5xx`, `404`, invalid arguments without a request;
  `pulls_test.go`), routes of `githubtest` (`state`, `sort`, `direction`, pagination) and fake
  `providertest` (sorting, `UpdatedSince`, injected error `ErrUnsupported`, quota); schema `00003`
  (uniqueness, `CHECK`, cascades), writing by `ApplyRepoSync` (creation, update, replacement of the
  releases and tags, keeping when the list is not read again, refusal of invalid data without writing
  anything) and reads (state filters, tags dated by their commit, summary of the repositories); engine
  (`extras_test.go`: first pass then incremental, isolated failure as a warning with commits kept and
  resumption at the same point, `ErrUnsupported` without warning nor clearing, low and exhausted
  quota, cancellation); CLI (`watchoo prs` and `watchoo releases` in text and in JSON, filters,
  invalid options, trapped titles neutralized, warnings of `watchoo sync`); web (cards and
  `repo-pulls` fragment, escaping of a trapped title, empty states, open PRs on the dashboard); TUI
  ("Pull requests" line of the detail, warnings of the summary). The two end-to-end tests also read
  PRs, releases and tags (`304` when nothing has changed).
- **Engine** (`internal/sync/`, `providertest` + temporary database): first import, incremental sync,
  unchanged head (no commit request), new branch, merge that brings older commits, force-push (branch
  and reference branch), deleted branch, `branch_patterns` and `branches.exclude`, default branch
  changed or renamed (old one deleted), patterns that exclude the default branch after the fact (new
  reference read again in full, `last_commit_at` and activity state kept), invalid label refused,
  renamed, transferred or empty repository, `404` and SSO, low quota before and during the sync,
  `RateLimitError`, secondary limit, lock (two engines on the same database), bounded parallelism,
  rules of the statistics, token asked again at each sync and masked, cancellation. One scenario goes
  through the real provider and `githubtest`.
- **CLI** (`internal/cli/`, `authtest` or fake `gh`, `providertest`): each command in text and in
  `--json` (key set frozen by the tests), outside a workspace, unknown repository, invalid arguments,
  `--since` and `-n`, import (`--dry-run`, summary, return code, invalid file), locked or deferred
  repositories, `gh` signed out, header card of `status`, low quota reported by `status` (text and
  `quota_low`), confirmation of `repo remove` (`--yes` outside a terminal), `repo add --label` with a
  refused control character, text output of `log`, `contributors` and `repo list` without any escape
  sequence when messages and Git names contain some (`internal/textclean` tested separately).
- **End to end** (`internal/cli/e2e_test.go`): real `auth.GH` with the fake `gh`, real GitHub provider
  against `githubtest`, real engine, temporary database: `init` → `repo add` → `repo import` (with
  `--dry-run`, an already tracked repository and an unreachable one) → `sync` → `log` (filters) →
  `contributors` → `repo list` (Actif (Active) / Flemme (Slacking) / Dead) → `status` → new commits and
  incremental sync (compare, never the full list; nothing when nothing has changed; only `GET`s) →
  `repo remove` (`--keep-history`, then deletion). No token, commit message or email in the logs.
- **TUI** (`internal/tui/`): `Init` / `Update` / `View` called directly (without `teatest`), with a
  fake `Syncer` that can be suspended and a real temporary database: views, ASCII and color modes,
  scrolling, filter, detail, sync (progress, refusal during a sync, errors, deferral in Paris time),
  automatic sync, quitting during a sync, help; adding a repository (`a`: input opened then canceled by
  `échap` (Esc), invalid reference refused without a call to the core, repository added with a message,
  reading back, selecting and syncing the repository, already tracked without a sync, tracking resumed,
  unreachable and error displayed as with `repo add`, adding during a sync: sync of the new repository
  queued then started, key without effect outside the Repositories view) with a fake add, and with the
  real engine, a temporary database and `providertest` (repository saved, synced, displayed). One test
  launches `tui.Run` with the real engine, a temporary database and a blocking provider: quitting
  during the sync waits for its end, the `sync_run` is no longer `running` and the repository is not
  locked. `watchoo tui`: refusal outside a terminal, `--json`, values of `--interval`.
- **Leaks**: for each command of the CLI, the harness checks that the fake token appears neither on the
  outputs nor in the logs; the tests of the engine also check the absence of name, email, login and
  commit message in the logs and the reports.

## Phase 4: web interface

- **Store and activity**: paginated history with total (`CommitPage`: bounds, filters), commit dates
  since a date (`CommitTimes`), commits per contributor and per repository, end of the last sync;
  daily activity series (`activity.DailyCounts`: 90 days, timezone, clock change, dates outside the
  window).
- **Coordinator** (`internal/sync`, fake `Syncer` that can be suspended, simulated timer): sync at
  startup only if the last one dates back more than an interval, periodic sync, manual sync refused
  during a sync, progress, global error, interval change (bounds), shutdown that cancels and waits for
  the sync, shutdown timeout exceeded; one scenario with the real engine.
- **Web** (`internal/web/`, `httptest` on the full router, temporary database filled with dummy data,
  real engine on `providertest` and `authtest`): each page (200, key content, commit message containing
  `<script>` escaped), 404 and 405 in French, pagination and filters, HTMX fragments (`HX-Request`
  header), `POST` without a CSRF token, with a wrong token or from a foreign origin refused (`403`,
  nothing is modified), no `GET` that modifies, foreign `Host` refused (`421`, page that shows nothing
  of the workspace), including behind `web.trusted_proxy` (names of `web.allowed_hosts` accepted),
  basic authentication (right and wrong credentials, static files included), security headers (CSP
  without `unsafe-inline` nor `unsafe-eval`), access log without query string, oversized body,
  recovered panic, concurrent manual sync refused, progress and `HX-Trigger: sync-finished` (also for a
  sync finished before the response to the `POST`), sync buttons never disabled at render time, clean
  shutdown of `Serve` (running sync canceled), static files (Content-Type, cache, `ETag` and `304`),
  logo copies identical to `assets/logo`, `app.css` present and up to date, checksums of
  `tailwind/VERSIONS`, and neither token nor email in any response nor in the log.
- **Pages**: dashboard (Dead, Flemme, Actif sorting then gray, counters, feed capped at 20, commit URL
  in `javascript:` neutralized, empty workspace); detail (each state, stopped tracking without a sync
  button, pagination over 120 commits with bounds, branch and author filters kept from one page to the
  next, filter containing an email ignored, `HX-Push-Url`, chart data in the display timezone);
  contributors (ranking, commits per repository, avatar only from `avatars.githubusercontent.com` with
  `referrerpolicy` and `loading`, escaping); settings (add, already tracked, refused inputs without
  creating anything, unreachable repository `422`, GitHub error `502` without detail, edit, removal
  confirmation in `GET` that changes nothing, stopping tracking, resume, deletion refused during a
  sync, interval and its bounds, quota fragment, `?msg=` messages never echoed).
- **End to end** (`internal/web/e2e_test.go`): full router, real engine and real coordinator, real
  GitHub provider on `githubtest`, fake `gh` (`authtest`); adding repositories from the settings
  (`POST` with CSRF; hidden repository refused; without a token: `403`), manual sync through the
  button, `HX-Trigger: sync-finished`, dashboard with the three states in the right order, detail with
  pagination and filters, new sync without any commit re-downloaded, contributors, stopping tracking
  then deletion from the settings; only `GET`s towards GitHub, and neither token nor email in the
  responses or the log.
- **CLI** (`watchoo serve`): `--json` and arguments refused, invalid `--interval` and `--addr`, refusal
  outside the loopback without authentication (no listening socket opened), rules of the password, port
  already in use, real startup on a free port of `127.0.0.1` (fragment and CSS served), header card,
  warning of the reverse proxy, basic authentication, sync at startup then no new sync after a recent
  sync, clean shutdown (code 0).

## Phase 5: MVP polish

- **Activity** (`internal/activity/series_weeks_test.go`): commits per week from Monday to Sunday
  (`WeeklyCounts`) in the display timezone (Paris, transitions to winter and summer time, end of year,
  UTC+14, invalid number of weeks), summary of a per-day series (total, busiest day).
- **Store** (`TestAuthorCommitTimes`): author and date of each commit, unknown author excluded, dates
  in UTC, `since` bound, filter by repository.
- **Web charts** (`internal/web/charts_test.go`): series of the 8 most active contributors and grouping
  of the others (ties, author outside the ranking, names cleaned and truncated, `[]` rather than
  `null`); JSON of the dashboard and contributors pages, absent from the fragments and from an empty
  workspace; name containing `</script>` escaped.
- **Web touch-ups**: `HEAD` accepted on all the `GET` routes, `405` on the `POST` routes, `404` on an
  unknown page (`routes_test.go`); CSRF cookie set on the HTML pages and never on the static files
  (`TestCSRFCookieOnlyOnHTML`); quota read interrupted by a canceled context logged at debug, other
  failures as a warning (`TestSettingsQuotaCanceled`).
- **`--json` output** (`internal/cli/json_schema_test.go`): the 10 commands that produce data write a
  single JSON object with exactly the expected keys; these keys are compared against the table of
  [JSON output](05-cli.md#json-output) (`TestJSONSchemaDocumented` reads `05-cli.md`: keep the format
  of its lines); empty workspace (`[]`, `null`), the three outcomes of `repo remove`, entries in error.
- **Version** (`TestVersionBuildMetadata`): complete `-ldflags`, `-ldflags` taking precedence over the
  build information, `go install …@v0.1.0`, local `go build`, modified tree (`dev`), commit outside a
  tag (pseudo-version), unreadable date or outside UTC, nothing known; `--version --json` identical to
  `version --json`, header card with the same version.
- **Distribution** (outside `go test`, checked by hand before the release, without a real token):
  `make release-check`, `make release-snapshot` (6 archives and their content, checksums file,
  `version --json` of the binary), `make docker` (user 65532, `WATCHOO_DIR=/data`, `gh --version`) and
  a trial of the container with the fake `gh` compiled for Linux (`init`, refusal of `serve` without
  authentication outside the loopback, `401` without credentials then `200`, clean shutdown).

## Local Git repositories (V2)

No external service: the `internal/provider/git/gittest` package creates real bare "remote"
repositories (`<temp>/<owner>/<name>.git`) and a working copy to fill them (dated commits, from a
dummy identity, branches, merges, lightweight and annotated tags, force-push, branch deletion, change
of default branch), with a Git configuration isolated from that of the developer (empty
`GIT_CONFIG_GLOBAL`, `GIT_CONFIG_NOSYSTEM`, no automatic maintenance). The provider hooks into them
through `git.Options.BaseURL` (`file://…`).

- **Provider** (`internal/provider/git`): `Repository` by `ls-remote` without cloning anything (default
  branch, GitHub URL, `Partial`); clone with a working copy in `repos/<name>`, `repos/` with `0700`
  permissions; branches read in `refs/remotes/origin/` (a local branch created in the clone does not
  count), `ListCommits` (HEAD, branch, SHA, `Limit`, `Since`, `Stop`, unknown branch `ErrNotFound`),
  message, identities, UTC dates and parents; `CompareCommits` with a merge, base equal to the head,
  unknown base (`ErrUnknownBase`); statistics (`diff-tree --numstat`: simple commit, root, merge
  against the first parent, unknown commit); `CommitStatsBatch` (0.5.0: same values as `CommitStats`
  for a simple commit, a root, a merge and an empty commit at `+0 −0`, SHA absent from the clone
  omitted, more than 500 SHAs in several batches, canceled context); `Prepare` which checks out the
  given default branch without `ls-remote`; tags (lightweight and annotated, dereferenced); `fetch`
  that follows a deleted branch, a force-push, an added tag and a new default branch (a single update
  per Provider); empty repository (non-nil empty lists); invalid references refused without launching
  `git` (`git` missing in this test); missing repository: `ErrAccessDenied` without a prompt, no
  leftover of the clone, URL absent from the message; closed port: `ErrUnavailable`; 16 concurrent
  calls on 4 repositories; working copy that follows the remote (local modification of a tracked file
  overwritten, untracked file kept, new default branch checked out); empty repository cloned without
  any extracted file, then filled at the first commit; no hook executed (global `core.hooksPath` with
  a `post-checkout` hook, `submodule.recurse`); namesake rule (`Layout`: order of addition, case
  ignored, name already taken by another repository, `.git` never given, addition that moves nothing);
  occupied location (directory of the user, clone of another repository) never touched, empty directory
  replaced; `RemoveClone` (case, unsigned directory kept, namesake moved to `repos/<name>` then found
  again by the next sync); `Check`; no name, email, message nor URL in the logs; `ErrUnsupported` for
  pull requests, releases, `RepositoryByID` and quota.
- **Migration `00004`** (`internal/store/migration_provider_id_test.go`): database filled at schema
  `00003` (all the child tables), migration, no row lost, clean `foreign_key_check` and
  `integrity_check`, foreign keys re-enabled, cascade still active, case ignored; several `provider_id`
  `NULL`, `0` refused, `Partial` write that keeps URL, stars, issues, archived and identifier,
  identifier resolved by an "API" sync.
- **Engine** (`internal/sync/gitsource_test.go`): choice of the source (gh signed in, absent, not
  signed in, unusable `github.user`: clones; timeout and unreadable response: error); adding without
  `gh` (`--dry-run` that plans once a repository written with a different case, missing repository
  "accès refusé" ("access denied"), second addition found by name); first import (branches, commits
  specific to a branch, tags, contributor without a login), incremental sync with statistics,
  repository that became unreachable, return of `gh` that resolves `provider_id` and the metadata;
  repository known by the API synced without `gh` (stars, issues, archived, URL and identifier kept,
  no call to the API).
- **Statistics through the clone** (0.5.0, `internal/sync/clonestats_test.go`, `providertest` for the
  API and `gittest` for the same history, at the same SHAs): with `gh`, first import of which **all**
  the commits have their statistics (merge against the first parent, empty commit known at `+0 −0`),
  no `CommitStats` call to the API, clone created in `repos/` on the default branch of the API; old
  unknown commits (database from before the migration `00006`) computed at the next sync, a known
  `+0 −0` commit never computed again; more than 100 new commits at once, all computed; unavailable
  clone (occupied location; `git` absent, clone provider impossible): repository successful, warning,
  fallback to the API (first import without statistics, then the new commits), then catch-up when the
  clone comes back; without `gh`, statistics of all the commits from the first import; cancellation
  during the computation.
- **Migration `00006`** (`internal/store/migration_stats_known_test.go`): database at schema `00005`
  with commits with non-null and null statistics; `Up` marks as known the first ones only, `Down`
  removes the column and the index while keeping the rows and the values; writing (`ApplyRepoSync`:
  new commit known or not, statistics of a commit already in the database); paginated list of the
  unknown SHAs.
- **Displayed lines** (`internal/web/lines_test.go`, `TestLineFormats` of the TUI, `TestCommitLines`
  and `stats_known` of `log --json` in the CLI): an unknown commit displays "—" ("-" in a terminal), a
  known commit at `+0 −0` displays its zeros, everywhere a commit is listed (history of a repository,
  dashboard feed, contributor page); totals per repository, per contributor and for the workspace with
  "(dont N commits non comptés)" ("including N uncounted commits"), "—" if no commit is known; badge
  of the warnings of the sync indicator, detail in the tooltip.
- **CLI** (`internal/cli/gitsource_test.go` and tests of `init`, `status`, `sync`, `repo`): `repo add`
  without clone, `sync` (text and `--json`, `source: "git"`, no quota; working copy in
  `repos/projet`, `repos/` with `0700`), `log`, `status` (source), namesake in `repos/bob-projet` that
  contains a committed `.watchoo/`: `status` run from that clone works on the workspace, `repo remove
  --keep-history` that keeps the clone, `repo remove` that deletes it and moves the namesake to
  `repos/projet`; `init` without `gh` (fake client, fake binary, real `auth.GH` to a nonexistent path)
  that creates the workspace and warns, without `git` it stops, refused token it stops; `source` key of
  the JSON outputs (`init`, `sync`, `status`).
- **Workspaces** (`internal/workspace`): a `.watchoo/` under the `repos/` of a workspace (at any
  depth, `0700` permissions) is ignored by `Find`, which returns the enclosing workspace; `Init` is
  refused there; a nested workspace elsewhere is still found.
- **Web and TUI**: repository page without a forge identifier (stars, issues, pull requests and
  releases hidden), quota fragment "sans objet" ("not applicable") without `gh`; matching TUI detail.

## Configuration sharing

- **Core** (`internal/configshare`): export → import round trip into another workspace (repositories,
  labels, branches, settings); export without stopped repository, sorted, without token, email, path
  nor `github` / `web` key; every validation refusal (invalid JSON or followed by other text, more
  than 1 MiB, `format` missing or different, newer or null version, unknown key at each level,
  `github` / `web` in `settings`, more than 1,000 repositories, duplicate regardless of case, invalid
  `repo`, `provider` other than `github`, invalid label, invalid pattern or with a comma, unreadable
  duration, inconsistent settings once merged); preview: add, update (before → after), unchanged,
  stopped repository to resume, settings only modified, warnings (`WATCHOO_*` variable, lost
  comments); preview without writing (database and `config.yaml` identical byte for byte); apply:
  unreachable repository reported without blocking the others, identical settings not rewritten.
- **Rewrite of `config.yaml`** (`internal/workspace`): `name`, `github.*`, `git.*`, `web.*` keys and
  non-imported settings kept, `.bak` identical to the old file, `0600` permissions, idempotence (two
  imports give the same file), YAML injection impossible (timezone and patterns with quotes, `#`, `:`,
  line breaks, `\n`), file read back equal to the expected values, failure without modification,
  template of `watchoo init` unchanged.
- **CLI** (`watchoo config`): export to the standard output and with `-o` (`0600`, refusal to overwrite
  without `--force`), import in text and in `--json` (keys of the table
  [JSON output](05-cli.md#json-output)), `--dry-run`, confirmation at the terminal, `--yes` required
  otherwise, non-zero return code if unreachable repositories remain.
- **Web**: export (headers, safe file name, content, `GET` without effect), preview without writing,
  `apply` without a CSRF token refused (`403`), document modified between the preview and `apply`
  revalidated, size exceeded (`413`), no token in the responses.
- **End to end** (`internal/cli/e2e_config_test.go`, fake `gh` and `githubtest`): workspace A
  (`repo add`, `config export -o`), workspace B (`config import --dry-run` without any writing, refusal
  outside a terminal without `--yes`, `--yes` with an unreachable repository and code `1`, repositories
  and `config.yaml` checked, `.bak`, sync, re-import without change), workspace C (preview then
  "Appliquer" ("Apply") in a real `watchoo serve`, CSRF token and cookie included); neither token nor
  email in the responses and the logs.

## Logo and header card

- The copies embedded in `internal/logo/art/` are identical byte for byte to `assets/logo/*.txt`
  (otherwise: `make logo`).
- Card: all the lines have the same displayed width (accents, « », block characters), wrapping of the
  tip, bound of 80 columns, no card below 40, 100% ASCII fallback, colors only when allowed.
- CLI: card displayed only on a simulated terminal, never with `--json`, on stderr nor in a redirected
  output (the output of `watchoo version` outside a terminal does not change).

## Self-update (0.6.0)

No test reaches GitHub: the root of the releases is that of a fake `httptest` server
(`internal/selfupdate/selfupdatetest`), which redirects `/latest` to `/tag/vX.Y.Z` and serves `tar.gz`
/ `zip` archives and a checksums file built in memory.

- `internal/selfupdate`: latest version read from the redirect (release page never downloaded),
  missing release, invalid tag or pre-release ignored; SemVer comparison table (`v` prefix,
  pre-releases, `dev`, pseudo-versions, installed version more recent); wrong SHA-256 checksum or
  archive missing from the checksums file refused; archive without a binary refused; capped size;
  replacement of a fake temporary "executable" (content, `0755` permissions, no forgotten temporary
  file; Windows variant with `.old`); directory not writable; HTTP refused outside the loopback,
  redirect to HTTP refused; Docker and development version refused; `WATCHOO_NO_UPDATE_CHECK`;
  `Monitor` (immediate check, single announcement, version kept).
- CLI: notice written to stderr when a newer version exists (simulated terminal), absent with `--json`,
  outside a terminal, with `WATCHOO_NO_UPDATE_CHECK`, for `dev`, for an up-to-date version, for
  `update` itself, and when the server answers after the deadline (the command does not wait any
  longer); notice after plain `watchoo` (card and help); notice always written when the command lasts
  longer than the deadline and the answer has arrived (no coin toss between answer and deadline); no
  CLI notice for `watchoo tui`, which receives its check function (sentence "Quittez puis lancez …"
  ("Quit then run …"), Docker included; absent with `WATCHOO_NO_UPDATE_CHECK` or a `dev` version);
  `update --check` in text and in JSON (contract of `json_schema_test.go`); `update` confirmed,
  refused, with `--yes`, without a terminal nor `--yes`, already up to date, in Docker, on a `dev`
  version.
- Web: banner present on the pages when a new version is known (Docker text included), absent otherwise
  and on the public error pages.
- TUI: the line under the header announces the new version when the injected check function reports
  one (a single check, with a deadline; Docker sentence, ASCII, short form at 80 columns, wrapping on
  a very narrow terminal, escape sequences cleaned); nothing when Watchoo is up to date, on error,
  when quitting before the answer or without a function (check disabled).

## In CI

On every push and every pull request:

1. `go vet` and the tests (`go test ./...`, with the `-race` concurrency detector).
2. `sqlc generate` then check that no generated file changes (committed code up to date).
3. `make css-check` (standalone Tailwind CLI of the version pinned in `internal/web/tailwind/VERSIONS`,
   installed with SHA-256 verification): the committed `app.css` is up to date.
4. No `GH_TOKEN` / `GITHUB_TOKEN` variable is provided to the tests: a test that would need the real
   GitHub fails instead of passing silently.
5. `git` is installed on the CI machines: the tests of the local Git repositories always run there (they
   are only skipped if `git` is absent, locally).

On a `vX.Y.Z` tag, the release workflow runs the same checks again (`make check`) before GoReleaser and
the Docker image ([Distribution](03-architecture.md#distribution)).

## Installation script

`cmd/watchoo/install_test.go` runs `install.sh` with `sh` against a fake releases server (`httptest`,
`WATCHOO_RELEASES_URL` variable, the only one allowed to use HTTP), with no network: latest version
read from the redirect of `/latest`, pinned version without `v`, default directory `~/.local/bin`,
binary installed with `0755` and no forgotten temporary file; wrong SHA-256 checksum or archive missing
from the checksums file refused without installing anything; nonexistent version, invalid version,
empty `HOME`. The tests are skipped outside Linux / macOS or without `sh`, `curl` and `tar`. In CI
(Ubuntu), `sh` is `dash`: the script stays POSIX.
