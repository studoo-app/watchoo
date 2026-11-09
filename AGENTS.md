# AGENTS.md

Instructions for coding agents working on Watchoo ([agents.md](https://agents.md/) format).

## Project

Watchoo is a single Go binary that watches GitHub repositories, **mostly private**, and answers
three questions: what moved, when, and who did it. It offers a CLI, an interactive mode
(`watchoo tui`) and a local web interface (`watchoo serve`). Every monitored group (e.g. a class's
projects) lives in its own directory, initialized by `watchoo init`.

**Current state: v1.6.1, stable release (phases 1 to 6 of the [roadmap](documentation/en/07-roadmap.md) in place: MVP, configuration sharing and V2, then the per-contributor page, updating Watchoo, the bilingual interface — English by default, French —, the stabilization of the public surface, then the languages and the share of the contributors, then the AI signatures and the languages of the contributors, then the sync warnings per repository in the web interface, in v1.3.1 a dedicated message for an unknown SSH host key, then in v1.4.0 the supervision and diagnosis, phases 7.1 to 7.5: declared, rhythm and form signals, reflection leads and human review, off by default, then in v1.5.0 phase 7.7: absolute signals and counter-indicators, then in v1.6.0 the EN / FR language selector of the web interface and phase 7.8 of the diagnosis: lines behind the signals on the review sheet, read when shown, and a diagnosis score per repository and per contributor, then in v1.6.1 the link to the documentation in the language of the web interface, the icons of the AI score, the AI score and the leads of the commits on a contributor's page; not released yet, phase 8.1 of the project conventions: compliance of the commits with the Conventional Commits charter, `watchoo conventions`, off by default, and the A1, A2 and F4 adjustments of the diagnosis when it is on, then phase 8.2: branch names, pull request titles, tags and milestones, releases as information, CC5 to CC7, CC1 per range between tags, CC3 trend, first milestone review `watchoo conventions --milestone` and the `/conventions` web page, computed when reading without migration, then phase 8.3: CHANGELOG, README, documentation and tooling criteria read by the sync from the Git objects of the local clone (head of the default branch and milestone tags) and cached in `repository_conventions`, migration 00017, with the file criteria of the milestone review, then phase 8.4: tag observations and rewrites of the default branch recorded by the sync when the conventions are on (`tag_observations`, `branch_rewrites`, migrations 00018 and 00019, 19 tables), due dates of the milestones, tag history SV3, pre-releases SV4 (`conventions.tags.prerelease_forms`), version bumps SV5, precedence SV7 and CHANGELOG freshness CL10 computed when reading, SV3 in the milestone review, then phase 8.5: merge SHA, fork origin and sections of the pull request descriptions (the description read in memory only, never stored) and allowed merge methods read from the `GET /pulls` and `GET /repos` responses already requested, migration 00020 (still 19 tables), one full reread of the pull request list per repository when the conventions are on, commits outside a pull request PR3 (first parent of the default branch of the clone, credited to the commit author), PR5 (`conventions.pr.body_sections`, `conventions.pr.require_closes`), PR6, BR2, BR5, forks set aside from PR1, PR2, PR5, PR6, BR1 and BR2, PR3 in the milestone review, then phase 8.6: pull request reviews read by the sync when the conventions are on in API mode (`PullRequestReviews`, `GET /pulls/{n}/reviews`, once per merged pull request, at most 100 per repository and per sync, under the quota reserve), reduced to counters, migration 00021 (still 19 tables, no reviewer login stored), PR4 (`conventions.pr.min_reviews`, `conventions.pr.reviewer_must_contribute`, applied when reading), merged without review, reviews received per contributor, PR4 in the milestone review, then phase 8.7: leads that invite a reread without judging, outside every grid, rate and the milestone review, CL11, CL12, RD5, DOC6, DOC7 and TL4 cached by the sync from the tree of the head (`FileRulesVersion` 2), DOC9, DOC10 and TL5 read from the clone's history in the detailed view of a repository only, `leads` key in `--json`, collapsed "Leads" block on the repository page, no migration nor setting; phase 8 done, except branch protection BR3, postponed). Since 1.0.0, an incompatible change to the public surface (commands, flags, exit codes, `--json` keys, settings, `WATCHOO_*` variables, database schema, web routes) requires a MAJOR version.**
`watchoo init` creates a workspace; `repo add / import / list / remove`, `sync`, `log`,
`contributors`, `languages`, `prs`, `releases`, `status`, `version`, `update`, `diagnostic` and `conventions` work in text and `--json` (stable schema, documented in
[05-cli.md](documentation/en/05-cli.md#json-output) and verified by `internal/cli/json_schema_test.go`);
`watchoo tui` opens the interactive mode; `watchoo serve` serves the local web interface (dashboard and
contributors with activity charts, repository detail, a contributor's page, settings, and `/conventions` when the conventions are on) and syncs in the background. The
business core is in `internal/sync` (engine: incremental sync, lock, quota, repository addition;
`Coordinator`: a single sync at a time in a long-running process), `internal/activity` (Actif /
Flemme / Dead and gray states, activity series by day and by week) and `internal/repoimport` (refs,
`--branch` patterns, import files); `internal/cli`, `internal/tui` and `internal/web` only
display. Two end-to-end tests run the commands (`internal/cli/e2e_test.go`) and the
web pages (`internal/web/e2e_test.go`) with the fake `gh` and `githubtest`. Distribution: GoReleaser
(`.goreleaser.yaml`), Docker image (`Dockerfile`, `docker-compose.yml`), release workflow on tag
(`.github/workflows/release.yml`); history in [CHANGELOG.md](CHANGELOG.md). V2: per-repository
activity thresholds, per-contributor activity within a repository, pull requests / releases / tags, and local Git
repositories (`internal/provider/git`: clones with a working copy in `repos/` at the workspace root, used when `gh` is absent or not
signed in). The MVP exit criterion (one week of real usage) is validated: the MVP is complete.
Since 0.6.0: new-version notice on every command and in `watchoo serve`, and
the `watchoo update` command (`internal/selfupdate`, the only documented exception to the GitHub rule: public
pages of Watchoo's own releases, without token or API).

## Reference documentation

`documentation/` is the **source of truth**. Read it before coding; in case of doubt or
contradiction, it takes precedence over any other source, skills included. It is bilingual
(English in `documentation/en/`, French in `documentation/fr/`): any change to a document is
mirrored in the other language **in the same commit**. The two trees contain the same files with
the same numbering; the index is [INDEX.md](documentation/INDEX.md).

| Topic | File |
| --- | --- |
| Vision, scope | [01-vision.md](documentation/en/01-vision.md) |
| Features and rules (Actif / Flemme / Dead states…) | [02-features.md](documentation/en/02-features.md) |
| Architecture, stack, technical decisions | [03-architecture.md](documentation/en/03-architecture.md) |
| Data model (19 tables) | [04-data-model.md](documentation/en/04-data-model.md) |
| CLI, TUI, bulk import | [05-cli.md](documentation/en/05-cli.md) |
| Web interface | [06-web-interface.md](documentation/en/06-web-interface.md) |
| Roadmap by phases | [07-roadmap.md](documentation/en/07-roadmap.md) |
| Testing | [09-testing.md](documentation/en/09-testing.md) |
| Workspaces (`watchoo init`) | [10-workspaces.md](documentation/en/10-workspaces.md) |
| Skills | [11-skills.md](documentation/en/11-skills.md) |
| Supervision and diagnosis (signals, guardrails, phase 7) | [12-supervision-diagnostic.md](documentation/en/12-supervision-diagnostic.md) |
| Project conventions (compliance indicators, milestone review, phase 8) | [13-conventions.md](documentation/en/13-conventions.md) |

A decision that changes what is documented is made **in the documentation first**, with the
maintainer's approval, then in the code.

## Working method

- Move forward **phase by phase**; only move to the next one when the previous one works end to end.
- Before writing the code of a phase, propose the tree and the plan.
- At the end of every phase: green tests, clean `go vet`, summary of what works.
- Never add a dependency outside the documented stack without approval.

## Stack (summary)

Go (latest stable, fixed in `go.mod`) · cobra + viper · go-github (without x/oauth2) · SQLite via
`modernc.org/sqlite` (no CGO) · goose migrations · sqlc queries · net/http + go-chi/chi ·
html/template + HTMX + DaisyUI (CSS generated by the standalone Tailwind CLI) · embedded Chart.js ·
Bubble Tea + bubbles + lipgloss · log/slog · errgroup · the `git` binary (local Git repositories, run
through `os/exec`). Details and rationale:
[03-architecture.md](documentation/en/03-architecture.md).

## Code layout (target)

```
cmd/watchoo/          entry point
internal/cli/         cobra commands (display only)
internal/tui/         Bubble Tea interactive mode (display only)
internal/web/         chi handlers, templates, static (embed), tailwind/ (DaisyUI CSS, VERSIONS)
internal/workspace/   workspaces: init, .watchoo/ discovery, settings
internal/auth/        gh access (token, sign-in state)
internal/provider/    Provider interface, types, errors; providertest/ (fake Provider)
                      github/: GitHub implementation; github/githubtest/ (fake GitHub server)
                      git/: local Git repositories (git binary); git/gittest/ (test repositories over file://)
internal/sync/        sync engine (business core)
internal/activity/    activity rules (Actif / Flemme / Dead, gray states), series, short formats
internal/diagnostic/  diagnosis (phase 7): per-commit signals (file exclusions, declared AI tools, bot
                      author, agent configuration file), rhythm signals R1-R6 (modified z-score), form
                      signals F1-F5 (measured in memory on the added lines), absolute signals A1-A7 and
                      counter-indicators H1-H4 (phase 7.7), reflection leads ("declared",
                      "probable", "to review"); keeps neither message, PR description, path nor code
internal/repoimport/  repository refs, --branch patterns, import files
internal/configshare/ configuration sharing (JSON export, validation, preview, import)
internal/aisignature/ AI signatures declared in the commits (catalog, detection, pure functions), bot
                      accounts (IsBot) and GitHub committer (web-flow, noreply@github.com)
internal/conventions/ project conventions (phase 8): strict Conventional Commits parser (CC2 codes),
                      exclusions and aggregation (CC1-CC4), start date, report builder (pure functions,
                      branches BR1, pull request titles PR1-PR2, tags SV1-SV2-SV6, CC5-CC7, milestone review;
                      file criteria CL/RD/DOC/TL from a tree and blobs, minimal Markdown reader;
                      history BR4-SV3, due dates of SV2, SV4-SV5-SV7, CL10 (phase 8.4);
                      merges PR3-PR5-PR6-BR2-BR5 (phase 8.5); review counters and PR4 (phase 8.6);
                      leads CL11-CL12-RD5-DOC6-DOC7-TL4 (tree) and DOC9-DOC10-TL5 (history) (phase 8.7);
                      Reader / CloneReader / HistoryReader / LeadHistoryReader interfaces); never keeps nor
                      displays a commit message
internal/semver/      SemVer 2.0.0 parsing and precedence (selfupdate, phase 8)
internal/textclean/   cleaning of displayed texts (escape sequences), CLI and TUI
internal/selfupdate/  updating Watchoo itself: latest release (redirection of /releases/latest),
                      verified download (SHA-256), binary replacement; selfupdatetest/ (fake server)
internal/store/       SQLite: migrations/, queries/ (sqlc), db/ (generated)
internal/model/       business structs
internal/logo/        owl in text versions (copy of assets/logo/) and header card
```

## Commands

```bash
make build        # bin/watchoo, no CGO, version injected by -ldflags
make test         # go test -race ./...
make vet          # go vet ./... (and the fake gh of the tests, ignored by ./...)
make fmt-check    # fails if gofmt -l finds a file
make sqlc         # sqlc generate
make sqlc-check   # fails if internal/store/db is out of date
make check        # all the checks, like the CI (.github/workflows/ci.yml)
make css          # DaisyUI CSS generated by the standalone Tailwind CLI (make css-check: up to date?)
make logo         # copies assets/logo into internal/logo/art/ and internal/web/static/ (embedded copies)
make release-check    # goreleaser check (.goreleaser.yaml)
make release-snapshot # goreleaser release --snapshot --clean: local release in dist/, nothing published
make docker           # local Docker image (IMAGE=watchoo:dev), never pushed
```

- `sqlc generate` after any change to `internal/store/queries/` or the migrations; the generated code
  (`internal/store/db/`) is committed and **never edited by hand**.
- `make css` after any change to the templates; `internal/web/static/app.css` is committed.
- `make logo` after any change to `assets/logo/` or `assets/icon/`; tests verify that the copies are up to date.
- The `-ldflags` (`internal/cli.Version`, `Commit`, `Date`) are written in the `Makefile`, the
  `.goreleaser.yaml` and the `Dockerfile`: keep them identical.
- Any visible change goes into the `[Unreleased]` section of `CHANGELOG.md` (skills `semver` and
  `semver-changelog`, see [11-skills.md](documentation/en/11-skills.md)). No tag, release or
  `docker push` without an explicit request.

## Architecture rules

- **No business logic** in `cli`, `tui` or `web`: these layers call the business core.
- Every GitHub access goes through the `Provider` interface; nothing GitHub-specific outside of
  `internal/provider/github/` (the only package importing go-github) and `internal/provider/git/` (clone
  and page URLs of github.com repositories). Exceptions: `internal/selfupdate/` reads the public
  pages of Watchoo's own releases (standard library, no token and no GitHub API); the catalog of
  `internal/aisignature/` compares account names and no-reply addresses, with no access to the forge;
  `internal/conventions/` recognizes the github.com URLs of the same repository
  (`github.com/<owner>/<name>/blob|tree|raw/<ref>/…`) in the Markdown links (RD3, DOC3, DOC4), as text,
  with no access to the forge.
- Types generated by sqlc never leave `internal/store/`; they are converted to `internal/model`.
- Web handlers stay plain `http.Handler`.
- **Incremental** sync: never re-download a commit already in the database; use ETags;
  respect the rate limit; at most 4 repositories synced in parallel.
- Dates stored as ISO 8601 UTC text, fixed format `2006-01-02T15:04:05Z`; conversion to local time
  at display only.
- One SQLite database per workspace (`.watchoo/watchoo.db`).

## Security (non-negotiable)

- `gh` is optional: without `gh` installed and signed in, repositories are tracked by local clone. When
  the API is used, the GitHub token comes **only** from `gh auth token`. Never read
  `GITHUB_TOKEN` or store the token (config, database, file, keychain).
- Local Git repositories: Watchoo never reads, asks for or stores any credential (git uses the SSH key
  or the user's credential helper, with no interactive prompt).
- The token never appears in logs, errors, CLI output, the TUI or the web.
- **Read-only**: no GitHub call in `POST`, `PATCH`, `PUT` or `DELETE` (the provider transport
  refuses them before sending); in local Git mode, only `clone`, `fetch` and read commands,
  never `push`.
- The data comes from private repositories and holds students' personal data:
  `.watchoo/` and `repos/` (local Git clones) at `0700`, files at `0600`; **no commit message nor e-mail in the logs**.
- Web: listens on `127.0.0.1` by default; authentication mandatory on any other address.

## Tests

- Every new or changed piece of code has its tests.
- **No external service** in tests or CI: `gh` is mocked (Go interface + fake binary in
  `internal/auth/testdata/fakegh/`, path set by `github.gh_path`), the GitHub API is simulated by the
  fake `githubtest` server (on `httptest`), the provider by `providertest`, local Git repositories by
  `gittest` (real repositories created in a temporary directory, served over `file://`; the `git` binary is
  required), the database is a temporary SQLite.
- Scenario details: [09-testing.md](documentation/en/09-testing.md).

## Style

- Code formatted with `gofmt`; `go vet` without warning.
- Go identifiers in English, per Go conventions.
- User-facing messages (CLI, TUI, web) are **bilingual: English by default, French** available; every
  displayed text goes through the `internal/i18n` catalog, and any new message is added in both
  languages in the same commit. Commit messages in French.
- Errors wrapped with context (`fmt.Errorf("…: %w", err)`); structured logs with `log/slog`.

## Skills

The project's skills come from [skills.sh](https://www.skills.sh/), installed in `.agents/skills/`
(links in `.claude/skills/` and `.vibe/skills/`), locked by `skills-lock.json`.

- `goland` for Go development; `semver` and `semver-changelog` for versions and `CHANGELOG.md`.
- Never install or update any skill without approval. See [11-skills.md](documentation/en/11-skills.md).

## Git

- Commit messages in the **Conventional Commits** format, in French: `feat: …`, `fix: …`, `docs: …`,
  `test: …`, `chore: …`.
- Do not commit or push without an explicit request.
- Never commit a `.watchoo/` database, a token or real student data (the tests use dummy data).
