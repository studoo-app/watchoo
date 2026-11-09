# 2. Features

[← Contents](README.md)

The MVP is limited to **tracking, syncing and displaying**; everything else comes later.

## MVP

| Feature | Detail |
| --- | --- |
| Add / remove a tracked repository | `owner/repo` or GitHub URL input; all branches tracked by default, restrictable by list or pattern. Repositories are added by `watchoo repo add`, the import, the web Settings page or the `a` key of the TUI (Repositories view: all branches, sync started right away) |
| Bulk import of repositories | CSV file (repository, display label, branches): a whole class of students at once, access checked, summary, rerunnable without duplicates |
| Commit sync | **Incremental** fetch since the last known SHA |
| List of latest commits | Message, author, date, short SHA, GitHub link; filters by repository, author, period |
| Contributors view | Number of commits per person and per repository, date of last activity |
| Repository status | Last commit, state (Active / Slacking / Dead), default branch, stars, open issues |
| Automatic sync | Configurable periodic task: 30 min by default, 15 min minimum (in `serve` and `tui`), can be disabled |
| Manual sync | At any time, all repositories or just one: `watchoo sync`, button in the web, key in the TUI |
| Activity charts (web) | Commits per day over 90 days for the workspace and for each repository; commits per week over 12 weeks for the 8 most active contributors |

## V2

| Feature | Detail |
| --- | --- |
| Per-contributor activity within a repository | Chart of each contributor's commits inside a single repository. **Released in v0.4.0**: on a repository's page (web), commits per week over 12 weeks of the repository's 8 most active contributors, like the contributors page; total and weekly curve of the main contributors in a repository's TUI detail; `watchoo contributors [--repo X] --weeks N` (commits week by week, `weekly` key in JSON). All tracked branches count, each commit once |
| Per-repository activity thresholds | Set the Slacking / Dead thresholds of a repository (`repo add` option, import column, web setting). **Released in v0.4.0**: `repo add --flemme N --dead N`, the `flemme` and `dead` columns of the import, configuration sharing, a repository page's form |
| Pull requests and releases | Open / merged PRs, tags, releases. **Released in v0.4.0**: read at every sync (PRs incrementally, since the last known change; releases and tags in full, free when nothing changed), `watchoo prs [--repo X] [--state open\|merged\|closed\|all]` and `watchoo releases [--repo X]` (text and `--json`), "Pull requests" and "Releases and tags" cards on a repository's page (web), open PRs on the dashboard, open PRs and the latest release in the TUI detail. A failure on these lists does not prevent the commit sync: it becomes a warning |
| Local Git repositories | Track a repository from a local clone (`git clone`, then periodic `git fetch`): commits, branches and tags with no GitHub API call. Used when `gh` is not installed or signed in. **Released in v0.4.0**: the choice is made again at every sync based on `gh` only; clones with a working copy in `repos/<name>` at the workspace root (name clashes: `repos/<owner>-<name>`; local changes to tracked files are overwritten at every sync) (SSH by default, `git.protocol: https` possible), using the machine's Git credentials, with no prompt; `watchoo init` without `gh` warns instead of stopping; the source is shown by `watchoo status`, the `watchoo sync` summary and the Settings page; stars, issues, pull requests and releases are hidden for a repository never read by the API; `repo remove` deletes the clone (`--keep-history` keeps it); Docker image with `git` |

## 0.5.0

The next V2 release, decided by the maintainer: it completes the contributors view.

| Feature | Detail |
| --- | --- |
| Per-contributor page | One page per contributor: their repositories, commits and activity over time, on the web and in the TUI. **Released in v0.5.0**: web page `/contributors/{id}` (header with name, login, avatar and the number of known Git identities, with no address of any kind; totals of commits, `+/-` lines, repositories, first and last commit; commits per day over 90 days; commits per week and per repository over 12 weeks; their repositories with commits, lines, last commit and repository state; their paginated commits; their pull requests when their forge account is known), links to this page from every list where a contributor appears; a contributor's detail in the TUI (`enter` in the Contributors view). No Active / Slacking / Dead state for a person: those states remain repository rules; the page shows the age of the last commit and the state of each of their repositories |
| Link to the forge profile | Wherever the web interface shows a contributor whose forge account is known, a secondary "GitHub" link opens their profile (address provided by the forge, stored in `contributors.profile_url`; for a contributor known before 0.5.0, filled in from their login at the next sync). No link for a contributor known only by their name and Git e-mail (local Git clones) |

## 0.6.0

The next release, decided by the maintainer: Watchoo announces its own new versions and knows how to
update itself.

| Feature | Detail |
| --- | --- |
| New-version notice | At **every launch** of a command, Watchoo checks, without blocking, whether a version newer than its own has been released, and announces it on the error output at the end of the command ("New Watchoo version available: v0.5.0 → v0.6.0. Update with "watchoo update"."). No notice with `--json`, outside a terminal (scripts, CI), for a development version, nor if the `WATCHOO_NO_UPDATE_CHECK` variable is set (Docker image). `watchoo serve` checks at startup then every 30 min, and the web interface then shows a discreet banner. **Released in v0.6.0** |
| One-command update | `watchoo update` shows the installed and latest versions, asks for confirmation, downloads the release archive for the system, verifies its SHA-256 checksum and replaces the binary in one piece; `watchoo update --check` only checks. Refused with an explanation in the Docker image (change the tag), for a version built from source (`go install …@latest` or `git pull && make build`) and when the binary's directory is not writable. Only the public pages of Watchoo's own releases are read: no token, no GitHub API, no quota. **Released in v0.6.0** |

## 0.7.0

| Feature | Detail |
| --- | --- |
| Bilingual interface | CLI, TUI, web interface and install script in **English by default**, also available in **French** (`WATCHOO_LANG`, the system locale, or the workspace's `lang` setting for the web). `--json` keys and state values do not change. **Released in v0.7.0** (see [Languages and translations](03-architecture.md#languages-and-translations)) |

## 1.0.0

No new feature: the public surface (commands, flags, exit codes, `--json` keys, settings and
`WATCHOO_*` variables, database schema, web routes) is reviewed and declared **stable**; from now on,
an incompatible change requires a major version. **Released in v1.0.0**.

## 1.1.0

| Feature | Detail |
| --- | --- |
| Languages of a repository | Languages of the default branch with their percentage (share of the bytes), computed at each sync from the local Git clone, with or without `gh` and without any quota: `watchoo languages`, "Languages" card of a repository's page, repository detail in the TUI. The figures are close to those of GitHub, not identical (see [Architecture](03-architecture.md#languages-of-a-repository-110)). **Released in v1.1.0** |
| Share of the contributors | Share of the commits of each contributor as a percentage, in `watchoo contributors` (`share` key in JSON), on a repository's page and in the repository detail of the TUI. **Released in v1.1.0** |

## 1.2.0

| Feature | Detail |
| --- | --- |
| Languages of a contributor | Lines added per language by the commits they authored, with their percentage, read from the local Git clone: "Languages" card on a contributor's page (and main language of each of their repositories), `watchoo contributors` (LANGUAGE column, `languages` key in JSON), contributor detail of the TUI. See [Architecture](03-architecture.md#languages-of-a-contributor-120). **Released in v1.2.0** |
| AI signatures | Tools declared in the commits (co-author, footer, agent account), per commit and per contributor, in `log`, `contributors`, the TUI and the web interface. See [AI signatures](#ai-signatures). **Released in v1.2.0** |

## V3

| Feature | Detail |
| --- | --- |
| Tracking a whole organization | Import all repositories of an account or an organization |
| Alerts | Discord / Slack / e-mail webhook, or desktop notification |
| Incoming GitHub webhooks | Real-time updates instead of polling |
| Other forges | GitLab, Gitea |
| Export | CSV / JSON for reports |

## Functional rules

- **Activity states**: computed from the age of the last commit, in whole days. The interface in
  French displays them as Actif / Flemme / Dead; the Slacking threshold keeps the name `flemme` in
  the options, settings and JSON (`--flemme`, `activity.flemme_after_days`).

  | State | Age of the last commit (default) | Web / TUI | CLI (`repo list`) |
  | --- | --- | --- | --- |
  | **Active** | 0 to 5 days | green | `-` |
  | **Slacking** | 6 to 15 days | orange | `~ 9 d` |
  | **Dead** | more than 15 days | red | `⚠ 42 d` |

  - "Last commit" = the most recent **across all tracked branches** of the repository, not only the
    default branch.
  - The two thresholds (5 and 15 days) are set **globally** in the config (user or workspace), and can
    be overridden **per repository** (a stable library that rarely moves, a project handed in at the
    end of the semester…): `watchoo repo add owner/repo --flemme N --dead N`, the `flemme` and `dead`
    columns of the import file, configuration sharing, or the "Activity thresholds of this
    repository" form of a repository's page in the web interface. Each threshold is independent: a
    repository may set only its Dead threshold and keep the workspace's Slacking threshold. `0` (or an
    empty field on the web) falls back to the workspace threshold.
  - A repository-specific threshold ranges from 1 to 3,650 days; when both are given together, Dead
    must be strictly greater than Slacking (otherwise refused, before any GitHub call). If mixing a
    repository threshold and a workspace threshold leaves Dead less than or equal to Slacking, Dead is
    raised to Slacking + 1 day for the state computation.
- **Non-activity states**: displayed in **gray**, they take priority over the activity states.
  - **Never synced**: the repository was just added.
  - **Error**: the last sync failed (repository deleted, access lost…).
  - **Archived**: the repository is archived on GitHub, it will not change anymore; it is not
    reported as Dead.
  - **Stopped**: tracking is stopped (`repo remove --keep-history`); the history remains viewable.
  - **Empty**: the repository has been synced but has no commit (a student repository that was just
    created).
  - Priority among gray states: Stopped, Error, Never synced, Archived, Empty.
- **Days**: counted in calendar days in the display time zone (`timezone` setting, otherwise the
  system's): a commit from yesterday at 11 p.m. is 1 day old, whatever the time.
- **Incremental**: a commit already in the database is never downloaded again.
- **Identity**: a contributor may use several e-mail addresses; they are attached to their GitHub
  login when it is known.
- **Read-only**: Watchoo never writes to GitHub.

## AI signatures

Since 1.2.0, Watchoo shows which AI tools are **declared** in the commits. It answers "is a tool
signed on this commit?", never "was this code written by AI?".

- **What a signature says**: a tool is declared on the commit. It may have written only the message, it
  can be added or removed by hand, and a squash may attach it to the person who merged. **Its absence
  proves nothing**: most tools let you turn it off. So the interface never says "human", gives no
  score nor percentage, and a commit without a signature shows nothing.
- **Three kinds of signature** (`kind`): `coauthor_trailer` (a `Co-authored-by` line of the message whose
  address belongs to a tool), `footer` (a whole line written by a tool), `agent_account` (the author of
  the commit is the account or the address of an agent).
- **Avoiding false positives**: addresses and accounts are compared exactly, **never Git names** (a
  person can be called Claude); the subject and indented lines are not read (quotes, examples); free text
  mentioning a tool does not count; automation without AI (Dependabot, Renovate) is not in the catalog.
- **Catalog**: only signatures observed on real public commits. Tools without a verified signature are
  left out rather than guessed.

| Tool (`tool`) | Recognized signatures |
| --- | --- |
| `aider` | Co-author `aider@aider.chat` or `noreply@aider.chat` |
| `claude` | Co-author `noreply@anthropic.com`; footer `Generated with [Claude Code]` |
| `codex` | Co-author `noreply@openai.com` or `codex@openai.com` |
| `copilot` | Co-author `copilot@github.com`; co-author or author: the `Copilot` or `copilot-swe-agent[bot]` account |
| `cursor` | Co-author or author `cursoragent@cursor.com` |

- **Not read**: the content of the code, the style of the messages, the pace of the commits. These
  clues have innocent explanations (grouped push, rebase, a student who has just learned Conventional
  Commits) and no measured reliability; this feature does not read them. The
  [diagnosis](12-supervision-diagnostic.md) (phase 7), off by default and for the trainer only, turns them
  into reflection leads, never verdicts, each learner being compared with their own history.
- **Purpose**: being able to know which tools are declared, by whom and where, not sanctioning. To know
  more, ask the contributors to keep the attribution of their tools enabled: Watchoo only sees what the
  commits declare.

## Project conventions

Phase 8.1 (not released): Watchoo measures the compliance of commit messages with a Conventional Commits
charter; the details are in [Project conventions](13-conventions.md).

- **Off by default** (`conventions.enabled`) and **for the trainer only**: these are objective findings,
  separate from the [diagnosis](12-supervision-diagnostic.md), **with no grade nor overall score**.
- **CC1**: compliant commit rate (commits without a blocking issue / evaluated commits), explained as `n/m`
  per repository; **CC2**: issues by pattern, blocking or warnings; **CC3**: rate per contributor;
  **CC4**: exclusions counted separately (merges, bots, root commit "Initial commit" created by GitHub,
  accounts of `conventions.excluded_accounts`).
- **Scope**: all the tracked branches, since the `conventions.since` date (can be overridden per repository).
- **Output**: `watchoo conventions` command, section of a repository's page and contributor's page; never
  the commit message, only the short SHA, the author and the codes.
- The charter (`since`, `commit.*`) is shared through `watchoo config export` / `import`.
