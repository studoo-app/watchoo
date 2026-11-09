# 13. Project conventions

[← Contents](README.md)

**Status**: **maintainer decision** (phase 8, November 8, 2026); the choices are recorded in
[section 11](#11-maintainer-decisions). Phases 8.1 to 8.4 done, not released; the next phases start after their
plan is approved. The scope exception is carried over into the [Vision](01-vision.md); the updates to
[Features](02-features.md), [Data model](04-data-model.md), [CLI](05-cli.md) and
[Web interface](06-web-interface.md) follow phase by phase.

## 1. Decision

The trainer imposes on the groups a **conventions charter** (commits, branches and pull requests,
versions, CHANGELOG, README, documentation) and checks it at a **milestone review**. Watchoo measures how
well this charter is followed, repository by repository, and derives a **"milestone review" view** from it.

These measures are **compliance indicators**: deterministic, verifiable, with no comparison to a personal
history and no hidden weight. They are not suspicions and stay **separate from the
[diagnosis](12-supervision-diagnostic.md)**: no data, no score and no page in common. Like the diagnosis,
their results are **reserved to the trainer**.

## 2. Positioning and guardrails

- **A finding, not a verdict**: each result shows its rule, its raw values and its exclusions. Watchoo
  does not grade: it computes no overall grade (no "/20") and ranks neither repositories nor people.
- **Unknown is not non-compliant**: a criterion that cannot be measured (clone unavailable, mode without
  the API, missing data) is shown as "unknown" and is left out of the total.
- **Reserved to the trainer**: the view and the `--json` output are intended for the trainer who
  supervises the workspace, like the diagnosis (maintainer decision). They contain nothing from the
  diagnosis; the students know the charter and its criteria, not the individual results computed by
  Watchoo.
- **Minimisation**: nothing of the content read is persisted (message, PR body, README, CHANGELOG, INDEX,
  file paths); only issue codes, counters, masks and version numbers are stored. The detailed lists
  (unreferenced documents, dead links) are **recomputed at display time** from the clone, never stored or
  logged.
- **Read-only and local**: unchanged. No write call to GitHub, no data sent elsewhere. No code is run (a
  "reproducible" installation is not checked by running the project).
- **Off by default**: everything goes through the `conventions.enabled` setting.
- **Scope exception**: reading README, CHANGELOG, INDEX, `documentation/` and the tooling files falls
  outside the "code content analysis" line of the [out of scope](01-vision.md#out-of-scope-for-now) list.
  The exception adopted: in-memory reading of these files only, without persisting the content; the style
  of the messages is judged against a charter known to the students, **never** to derive a suspicion of AI
  use.

## 3. Reference: the charter

Default values of the configuration ([section 8](#8-configuration)); each trainer can adapt them.

| | Requirement | Standards |
| --- | --- | --- |
| A | Commits `<type>(<scope>): <description>`; types `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`; lowercase, imperative, no final period, ~50 characters; body separated by a blank line; footers `Key: value`; breaking change via `!` or `BREAKING CHANGE:`. Forbidden: `update`, `maj`, `wip`, `test`, "Dépôt 1", empty message or no type, past tense, first person. Tooling: `commit-msg` hook or commitlint, PR template | [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/) |
| B | One branch per feature, `<type>/<scope>-<short-subject>`; never work directly on `main`, which is protected; conventional PR title; body: Context, What changes, How to test, Screenshots, `Closes #n`; one review by another member; squash merge | — |
| C | Tags `vX.Y.Z`, pre-release `v2.0.0-rc.1`, never rewritten; milestones `v1.0.0`, `v2.0.0`, `v3.0.0` | [SemVer 2.0.0](https://semver.org/) |
| D | `CHANGELOG.md` at the root: `## [Unreleased]` at the top, `## [X.Y.Z] - YYYY-MM-DD` from newest to oldest, categories Added, Changed, Deprecated, Removed, Fixed, Security in that order, SemVer statement, no empty entry | [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) |
| E | `README.md` at the root: presentation, group, stack and prerequisites, installation and launch, environment variables, tree, links to `/documentation/INDEX.md` and `CHANGELOG.md`, license or confidentiality | — |
| F | `documentation/INDEX.md` (table `Document \| Theme \| Version \| Last updated \| Owner`, one row per file); subfolders `NN-theme/`; file names in lowercase with hyphens, no space or accent; header on each document; embedded diagrams, `Media/`; no archive | — |
| G | Milestone review: 100% compliant commits; branches per feature, `main` protected, PRs reviewed; milestone tags never rewritten; CHANGELOG up to date; README with installation; exhaustive INDEX | — |

## 4. Data read

| Source | What is read | Mode |
| --- | --- | --- |
| Database (already synced) | `commits.message`, `commit_branches`, `branches`, `pull_requests` (title, branches, states, dates), `tags`, `releases` | both |
| Local clone, **through git objects** | `git rev-list` (first parent, ranges between tags), `git for-each-ref refs/remotes/origin/` (all the remote's branches), `git ls-tree -r -l -z` on the full SHA of the head of the default branch (branch list of the sync source) or of a tag's commit (read from the database); never a branch or tag name, then `git cat-file --batch` on object ids (see [section 9](#9-technical-impact)), `git merge-base --is-ancestor` | both (the clone exists with or without `gh`) |
| API, no extra request | `merge_commit_sha` (merged PRs only), PR body (in memory) and source repository in `GET /pulls`; `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` in `GET /repos` | API |
| API, extra request | `GET /repos/{o}/{r}/pulls/{n}/reviews` | API |

- The clone is **never** read through the file system of `repos/`: the working copy is reset at every sync
  and may have been modified by hand.
- No API fallback for the tree and the files (same decision as for languages): without a clone, the
  criteria concerned are "unknown".
- Tags and paths coming from the repository are passed after `--end-of-options` and are validated; a failing
  git command returns a typed error ("missing object"), without copying its stderr (which would contain the
  path or the tag name) into the logs.

## 5. Indicator catalogue

The prefixes CC, BR, PR, SV, CL, RD, DOC and TL do not overlap with those of the diagnosis (D, R, F, A, H).
Columns: **mode** "both" = computable without the API; "API" = "unknown" with a clone only. **Rel.**
(reliability): H high, M medium, L low. **Prio**: P1 to P3.

### 5.1 Commits (CC)

**Shared strict parser** (CC, PR1, SV5, CL10): the subject is the first non-empty line, without the squash
suffix "(#n)". It is compliant if it matches, case-sensitively,
`^(<types>)(\([a-z0-9][a-z0-9._/-]*\))?!?: \S.*$`. The permissive parser of the diagnosis's F4 signal
remains a separate "tolerant" mode.

**Scope** (maintainer decision): **all the tracked branches**, as they are today (`git rev-list` of the
tracked `origin/<branch>` in the clone, each commit counted once), since `conventions.since`.
`commit_branches` is only a fallback, with the note "may include rewritten commits" (the links between a
commit and a branch are never removed). The default branch alone forms a second, informative view.

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CC1 | Compliant commit rate | Commits without a blocking issue (CC2) / evaluated commits (excluding CC4) | repository, range between tags | database + clone | both | H | P1 |
| CC2 | Issues by pattern | **Blocking**: `empty`, `forbidden` (whole subject in the forbidden list, or prefix `wip`, `fixup!`, `squash!`, `amend!`), `github_default` (`^(Update\|Create\|Delete\|Rename) \S+$`, "Add files via upload"), `no_type`, `unknown_type`, `type_case`, `separator`, `scope` (malformed or empty scope, or missing from `commit.scopes`), `trailing_period`, `body_separator`, `length_hard` (> 72 characters). **Warnings**: `desc_capitalized` (except acronyms), `length_soft` (> 50), `footer_format`, `git_revert` (`Revert "…"` produced by Git, counted) | commit; pattern × repository, pattern × contributor | database | both | H | P1 |
| CC3 | Rate per contributor | CC1 on the commits they authored, with the trend since the last tag | contributor, contributor × repository | database | both | H | P1 |
| CC4 | Exclusions | Outside the denominator, counted separately: merges (`parent_count ≥ 2`), bots (`[bot]` suffix and catalogue accounts), root commit "Initial commit" created by GitHub or Classroom; unknown `parent_count` (NULL): fallback on the default merge messages and on the exact subject "Initial commit" with the GitHub committer. Web edits ("Update README.md") and AI agent accounts are still evaluated | commit | database | both | H | P1 |
| CC5 | Wording to review | Warning only: `past_tense` (closed lexicon of FR and EN participles), `progressive` (`-ing`), `first_person`. No rule on the "-é" ending alone | commit | database | both | M | P2 |
| CC6 | Footers and issues | Share of commits with a valid footer and share of those citing an issue (`(Closes\|Fixes\|Resolves\|Refs) #\d+`, keyword case ignored, optional `:` after it (`Refs: #12`, footer form), searched in the whole message, subject included); informative | repository, contributor | database | both | H | P3 |
| CC7 | Message language | FR / EN mix within one repository, if the charter imposes a language; informative | repository | database | both | M | P3 |

### 5.2 Branches (BR)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| BR1 | Compliant branch name | Remote branches (`refs/remotes/origin/` of the clone) and `pull_requests.head_branch` of PRs not coming from a fork (forks counted until phase 8.5, see the note below), excluding the default branch and exclusions (`gh-pages`, `dependabot/*`, `renovate/*`, setting). Compliant if `^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$`; a single segment after `/`: "to review". Codes (`--json`): blocking `unknown_type` (unknown type), `uppercase` (uppercase), `underscore` (`_`), `space` (space), `accent` (accent), `no_slash` (missing `/`), `pattern` (name that does not follow the rule with no other named issue, `feat/a.b`, or custom pattern not matched); warning `single_segment` ("to review") | branch, repository; contributor through the PR author | clone + database | both | H | P1 |
| BR2 | Reused branch | A `head_branch` (not from a fork) with more than one PR merged into the default branch | branch, repository | database | API | M | P2 |
| BR3 | Protected default branch | **Postponed** (maintainer decision: not implemented in phase 8). Planned measure: `protected` from `GET /branches`: yes, no, unknown (clone), not applicable (for example a private repository on GitHub Free). **Snapshot** without history | repository | API, 0 requests | API | M | — |
| BR4 | Rewritten default branch | At sync time, the old head is no longer an ancestor of the new one (`merge-base --is-ancestor`); dated event (old and new SHA). Old object missing from the clone: "unknown", never "rewritten". Only when the conventions are enabled, for the default branch tracked before and after the sync and under the same name (a change of default branch is not a rewrite); the ancestry is read in the clone updated by the sync, in both modes. In API mode only, when the clone does not decide (no clone, or commit missing from it), the sync's `ErrUnknownBase` error on the old head (commit gone from the repository, or no common ancestor) is also used as a signal. Limit: GitHub keeps orphaned commits for a long time, so after a force-push its comparison often succeeds (`diverged`, `behind`) and the API alone misses the rewrite; in clone-only mode `ErrUnknownBase` means a commit missing from the clone and is never a signal | event, repository | clone | both | H | P2 |
| BR5 | Allowed merge methods | `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` of the repository (returned to an account with push rights) | repository | API, 0 requests | API | M | P3 |

### 5.3 Pull requests (PR)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PR1 | Conventional PR title | Shared parser on the title of non-draft, non-bot PRs; open and merged PRs counted separately | PR, repository, author | database | API | H | P1 |
| PR2 | Forbidden or default title | Normalised title in the forbidden list, or equal (after normalisation: case, `-` and `_` as spaces) to the head branch name or to the subject of the PR's only commit (titles suggested by GitHub; comparison with the only commit not measured in 8.2, not before phase 8.5 and `merge_commit_sha`). Codes (`default_codes`): `forbidden`, `github_default`, `head_branch` | PR | database | API | H | P1 |
| PR3 | Commits that reached the default branch outside a PR | Walk of `git rev-list --first-parent origin/<default>` since `since`, without the root commit. **Through a PR**: SHA equal to the `merge_commit_sha` of a merged PR; sequence rebased by GitHub, recognised by a heuristic with no request (consecutive commits whose committer is GitHub and whose `committed_at` equals the `merged_at` of the PR that ends it; maintainer decision); merge whose committer is GitHub. **Outside a PR**: human committer, local merge pushed, web edit without a PR. Otherwise: "undetermined". With a clone only, fallback on `^Merge pull request #\d+` and `\(#\d+\)$` with the GitHub committer recognised by its `noreply@github.com` e-mail (in memory) | commit; repository; "author of the commit that arrived outside a PR" (never "pushed") | clone + database | both (degraded with a clone) | H with the API, M with a clone | P1 |
| PR4 | Reviewed by another member | New Provider method `PullRequestReviews` (login, identifier, state, date; never the body). Reviewed if, for at least one reviewer with an identifier other than the author's and not excluded (trainer, bots), the last state among `APPROVED`, `CHANGES_REQUESTED` and `DISMISSED` submitted no later than `merged_at` is `APPROVED`. Another account is enough; the `pr.reviewer_must_contribute` setting also requires a reviewer who authored at least one commit of the repository. Only counters are stored, not the reviewers. Derived view: "merged without review" | PR, repository; reviews received by the PR author | API, 1 request per PR | API | H | P1 |
| PR5 | Structured body and `Closes #n` | Body read in memory (as for D2): section headings, or bold text, among versioned FR and EN synonyms (context, what changes, how to test, screenshots); an empty section (or one reduced to an HTML comment) does not count. Closing keyword searched separately. Only a bit mask is stored (NULL while unknown). Known false negative: issue linked through the "Development" sidebar | PR, repository | API, 0 requests | API | M | P2 |
| PR6 | Squash merge | Commit of `merge_commit_sha` with one parent and the "(#n)" suffix: squash; two parents: merge commit. Compliant if squash **and** message compliant with the parser | PR | database | API | M | P2 |
| PR7 | PR template | See TL3 | repository | clone | both | H | P2 |

### 5.4 Tags and SemVer (SV)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SV1 | SemVer tag name | Lowercase `v` then a SemVer 2.0.0 version (comparator extracted from `internal/selfupdate` into `internal/semver`). Codes (`--json`): blocking `no_v` (no `v`), `upper_v` (`V`), `missing_component` (missing component), `leading_zero` (leading zero), `empty_prerelease` (empty pre-release), `invalid` (any other name); warning `build_metadata` (`+build`) | tag, repository | database | both | H | P1 |
| SV2 | Milestones present | For each configured milestone: present, present under a non-compliant name, absent; a pre-release does not count as a milestone. With a deadline: the tag with the exact name is "on time" if its first observation precedes the end of the deadline, "late" if a complete read of the tags between the date of the target commit and the end of the deadline did not see it, otherwise "undetermined" (display "set between X and Y"); proof of a read: see phase 8.4 below | milestone × repository | database | both | H (presence), M (deadline) | P1 |
| SV3 | Tag rewritten or deleted | `tag_observations` table, fed only when the conventions are enabled and reading the tags succeeded, was not truncated (fewer than 1,000 tags) and skipped no tag (a tag missing from an incomplete list would be closed wrongly): same name with another SHA (rewritten), gone (deleted); only a change of target commit is seen (both sources give the SHA of the target commit, not of the tag object: an annotated tag recreated on the same commit is invisible). Label: "no rewrite observed since …", never "never rewritten"; no attribution to a person | tag, repository | database | both | M | P1 |
| SV4 | Pre-releases | Form `-rc.N`, `-beta.N`, `-alpha.N`; free forms (`-rc1`, `-RC.1`) give a warning, not an issue (maintainer decision); also a warning if an rc is set after the final version or if precedence contradicts ancestry | tag | database + clone | both | M | P2 |
| SV5 | Bump consistent with the commits | For each final tag F, base B = highest compliant final tag lower than F that is its ancestor; commits of `B..F` excluding merges and bots. Breaking change: MAJOR expected (MINOR before 1.0.0); `feat`: at least MINOR. **Only under-versioning is flagged**; "undetermined" if less than 80% of the range's commits are compliant; first tag not evaluated | pair of tags | clone + database | both | M | P2 |
| SV6 | Associated release | Excluding drafts: milestone without a release (`milestone_without_release`), release without a tag (`release_without_tag`), inconsistent `prerelease` flag (`prerelease_mismatch`). **Information only**: the charter only mentions tags, SV6 does not count towards compliance | tag, release | database | API | H | P2 |
| SV7 | Precedence and history | Higher version set on an ancestor of a lower version; increment skip (v1 → v3) as information | pair of tags | database + clone | both | M | P3 |

**Phase 8.2: measured on the fly, and limits.** BR1, PR1, PR2, SV1, SV2 (presence), SV6, CC5 to CC7, CC1
per interval between tags and the CC3 trend are computed when read, from the database and the clone, with no
migration and no extra request. Approximations:

- **Forks**: the database does not keep the source repository of a PR, so the head branches of PRs from a
  fork are counted in BR1, and those PRs in PR1, until phase 8.5 (`from_fork`).
- **API mode**: it is not stored. PR1, PR2 and SV6 are "unknown" for a repository the API has never seen
  (no forge identifier) and for a workspace synced in clone-only mode; otherwise the PRs and releases in the
  database are those of the last sync through the API. PR head branches are only added to BR1 in API mode.
- **PR1 and PR2**: PRs other than drafts, bots and PRs closed without merge, opened since the start date. A
  PR is linked to a contributor by its forge account. Comparing the title with the subject of the PR's only
  commit is **not measured**: the database does not link commits to PRs.
- **BR1**: remote branches read in the clone (`refs/remotes/origin/`, without update) and PR head branches
  (without bots), each name once. The built-in rule also gives `pattern` to a name that does not follow it
  with no other named issue (`feat/a.b`, `feat/-x`). Without a clone, or with an unreadable clone, the fallback is the
  **tracked** branches in the database: the list is partial (a branch excluded by `branches.exclude` or by
  the repository patterns never appears there). A branch is attributed to a person only through the author
  of the PR whose head branch it is.
- **Tags**: read in the database (the sync writes them in both modes, with the SHA of the target commit). A
  tag has no date: a due date bounds the commits by their **authored date** (`authored_at`).
- **CC1 per interval**: `git rev-list <tag> ^<previous final tag>` in the clone, without update; only
  conforming final tags bound an interval. Commits of the interval that are not in the database (untracked
  branches) have no message: they are counted apart ("not in the database") and, in the milestone review,
  make the criterion "unknown" when there is no other issue. A tag commit missing from the clone makes the
  interval "unknown", never wrongly bounded. Without a clone: "unknown". Only final tags bound an interval:
  conforming, without pre-release **nor build metadata** (`v1.0.0+build`, conforming with a warning, bounds
  none). Interval of a milestone bounded by its due date: a commit not in the database has no date, it is
  counted among the missing ones even if it is later than the due date (a cautious choice: "unknown" rather
  than an unverified 100%); if none of the tracked branches is in the clone, the interval is unreadable
  (`clone_unreadable`), never empty.
- **Report restricted to contributors** (`--contributor`): intervals not computed (a repository
  indicator); PR1, PR2 and BR1 only cover the PRs of these contributors and their head branches,
  unattributed branches are left out: no other person appears.
- **CC3 trend**: two rates per contributor, on the commits reachable from the last final tag of their
  repository and on those that arrived since; a commit of a repository without a readable clone or a final
  tag is not placed (counted apart).
- **CC7**: a heuristic without dependency (closed lists of function words, accented letters); many short
  subjects stay "unknown".

**Phase 8.4: history and versions, implementation and limits.** BR4, SV3 and the SV2 deadlines are read
from the `branch_rewrites` and `tag_observations` tables, fed by the sync **only while the conventions are
enabled** (data minimization); SV4, SV5, SV7 and CL10 are computed at read time, from the database and the
existing clone (no fetch), and are not stored. None of these indicators is attributed to a person.
Approximations kept:

- **SV2 deadlines**: only the tag with the **exact name** is judged (a non-compliant name stays
  "nonconforming name"). End of the deadline: midnight the next day in the time zone of the workspace,
  bound excluded. No existing data proves that a sync read the tags at a given moment (`sync_runs` says
  neither which list was read, nor whether it was truncated, nor whether the conventions were enabled):
  **the proof of a complete read is a date of `tag_observations`** of the repository (first observation,
  last observation or end of any row), each being the moment of a successful, untruncated read made with the
  conventions enabled. "Late" requires such a date in `[date of the target commit, end of the deadline)`,
  earlier than the first observation of the tag. Otherwise "undetermined", shown as "set between X and Y"
  (X: date of the target commit, when the commit is in the database; Y: first observation). Limits: a
  repository without any other tag, or whose reads left no date in the window (the last observation of a
  row is rewritten at every read), gives "undetermined"; so does a target commit later than the deadline or
  of unknown date, and a tag never observed.
- **SV3**: four states per tag, `stable` (no rewrite observed), `restored` (deleted then set again on the
  same commit, without rewrite: counted with the tags without observed rewrite), `rewritten` (the name
  designated another commit) and `deleted` (gone and not back, prevails over an earlier rewrite). One row of
  `tag_observations` per period: a (name, SHA) pair that comes back after a deletion opens a new row. Label:
  "no rewrite observed since <first observation>". Tag never seen by a complete read (conventions enabled
  after the last sync, truncated reads): "unknown" (`not_observed`).
- **BR4**: shown per repository, number of events, date of each, old and new short SHAs; never "pushed".
- **SV4**: codes `free_form` (neither `-<form>.N` nor a form of `tags.prerelease_forms`), `after_final`
  (pre-release set after the final version with the same MAJOR.MINOR.PATCH: the final version is a strict
  ancestor of the pre-release in the clone; when ancestry does not decide, same commit, unrelated commits or
  no clone, the pre-release was first observed after the final version) and `order` (of two pre-releases
  of the same version successive by precedence, the higher one is set on a strict ancestor of the lower one;
  clone only). All are **warnings**, never issues. Only the pre-releases with a compliant name (SV1) are
  evaluated.
- **SV5**: bases searched by decreasing precedence, stopping at the first unreadable ancestry
  (`clone_unreadable`); no ancestor: `no_base`. Commits of `B..F` read from the clone then from the database;
  merges (parent count, or default merge message when it is unknown) and bots are excluded, **not** the
  accounts of `excluded_accounts` nor the commits before the start date. Only the commits compliant with
  the strict parser declare a type or a breaking change. The "MINOR is enough for a breaking change" rule
  applies when the base is `0.y.z`. "Undetermined" when commits of the range are missing from the database
  (`partial`) or when less than 80% of the evaluated commits are compliant (`low_conform`); a range without
  evaluated commit is "consistent". Over-versioning is never flagged. Without a clone: "unknown"
  (`no_clone`).
- **SV7**: only **successive** compliant final versions by precedence are compared (an inconsistency
  between non-successive versions, both unrelated to the version between them, is not seen);
  `precedence_ancestry` (higher version set on a strict ancestor of the lower version) is the issue,
  `increment_skip` is information, also computed without a clone and extended to PATCH and MINOR skips
  (`v1.0.0` → `v1.0.2`). Two tags on the same commit are consistent.
- **CL10**: last commit of the history of the known head of the default branch that touched `CHANGELOG.md` at
  the root (`git --literal-pathspecs log -1 --format=%H --end-of-options <head> -- CHANGELOG.md`), then the
  commits of the head not reachable from it; same exclusions as SV5. The count is a minimum when commits are
  missing from the database (`missing`). **Grayed out** (information) when CL9 of the last milestone set
  (highest version among the milestones that have a tag) is compliant in the cache of the sync. Unknown
  without a clone (`no_clone`), without a commit that touched the CHANGELOG (`no_file`) or when the head is
  not known in the database (`clone_unreadable`). Computed at read time: `FileRulesVersion` does not change.
- **Reading the clone**: through an optional interface (`HistoryReader`: `IsAncestor`, `LastTouch`),
  without update; each ancestry test is memoized for one computation. An error (commit missing from the
  clone, unreadable clone) means "unknown", never "no".
- **Report restricted to contributors** (`--contributor`): SV4, SV5, SV7 and CL10 are not computed
  (`filtered`); BR4, SV3 and the deadlines, read from the database, stay in the JSON; the CLI text and the
  page of a contributor do not show them.

### 5.5 CHANGELOG (CL)

The `CHANGELOG.md` blob is read in memory from the clone (at most 1 MiB); code blocks are ignored.

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| CL1 | Present at the root | Exact `CHANGELOG.md`; otherwise misnamed (`(?i)^change[-_]?log(\.md)?$`), elsewhere, absent, or not measurable (empty repository) | repository, tag | clone | both | H | P1 |
| CL2 | Header and statements | H1 `Changelog` (or `Journal des modifications`), mention of Keep a Changelog and of SemVer before the first H2 | repository | clone | both | H | P1 |
| CL3 | `[Unreleased]` at the top | First H2 = `## [Unreleased]`; issues: absent, misplaced, duplicated, without brackets | repository | clone | both | H | P1 |
| CL4 | Version headings and dates | `## [X.Y.Z(-pre)] - YYYY-MM-DD`, `[YANKED]` allowed; real date, not in the future. Issues: `v` in the heading (warning), missing brackets, date in FR format, absent, invalid, in the future | version | clone | both | H | P1 |
| CL5 | Descending order | Strictly descending SemVer precedence, non-increasing dates | repository | clone | both | H | P2 |
| CL6 | Categories | H3 in {Added, Changed, Deprecated, Removed, Fixed, Security}, or their French labels (Ajouté, Modifié, Obsolète, Supprimé, Corrigé, Sécurité), allowed by default; no duplicate, in that order, no bullet outside a category | version | clone | both | H | P1 |
| CL7 | Empty entries | Released version without a bullet, category without a bullet, empty or "TODO" bullet; an empty `[Unreleased]` is not an issue | repository | clone | both | H | P2 |
| CL8 | Consistent tags and sections | Tag `vX.Y.Z` without a section, dated section without a tag (`[YANKED]` exempt); date gap shown without being counted | tag | database + clone | both | H | P1 |
| CL9 | Up to date at the tag | In `<tag>:CHANGELOG.md`, the `## [X.Y.Z]` section exists; result cached per (tag, SHA) | milestone | clone | both | H | P1 |
| CL10 | Freshness | Number of `feat` and `fix` on the default branch since the last commit that touched the CHANGELOG; grayed out if CL9 is compliant | repository | clone + database | both | M | P2 |
| CL11 | Comparison links | `[X]: url` definition for each heading; outside the total | repository | clone | both | M | P3 |
| CL12 | Copy of `git log` | At least 10 bullets, of which at least 60% are identical to a commit subject, or at least 20% SHAs or "Merge"; only the percentages are stored; "to reread" | repository | clone + database | both | L | P3 |

### 5.6 README (RD)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| RD1 | Present at the root | Exact `README.md`; `README.<language>.md` does not count and is not a fault | repository, tag | clone | both | H | P1 |
| RD2 | Expected sections | Headings outside code, normalised and compared to FR and EN synonyms: presentation, group, stack and prerequisites, installation, launch, environment variables, tree, license or confidentiality (a `LICENSE` file is noted). Issue: "not found by its heading". Only a mask is stored | repository, tag | clone | both | M | P1 |
| RD3 | Links to INDEX and CHANGELOG | Relative link, absolute from the root (`/documentation/INDEX.md`) or URL of the same repository; states: valid, dead, absent, to the folder only | repository, tag | clone | both | H | P1 |
| RD4 | Size | Words outside code, configurable threshold (150); raw value shown | repository | clone | both | M | P2 |
| RD5 | Installation | Command block in the installation or launch section; `.env.example` present if variables are announced; lead only | repository | clone | both | L | P3 |

### 5.7 Documentation (DOC)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| DOC1 | INDEX present | Exact `documentation/INDEX.md`; variants flagged (`docs/`, `index.md`) | repository, tag | clone | both | H | P1 |
| DOC2 | INDEX table | GFM table with the 5 expected columns (synonyms allowed); number of rows. The cells are never stored (the Owner column contains names) | repository, tag | clone | both | H | P1 |
| DOC3 | Exhaustive INDEX | Number of `*.md` files in `documentation/` (excluding INDEX and `Media/`) that no INDEX link targets; duplicate INDEX rows | repository, tag | clone | both | H | P1 |
| DOC4 | Dead links | Relative links of the README, the INDEX and the documents whose target does not exist; a case difference is flagged as such | repository | clone | both | H | P1 |
| DOC5 | Naming | Folders `^\d{2}-[a-z0-9]+(-[a-z0-9]+)*$`; files in lowercase with hyphens, no space or accent (exceptions: `INDEX.md`, `README.md`, and `Media/` with its capital letter, allowed by the charter) | repository, tag | clone | both | H | P1 |
| DOC6 | Document headers | Title, purpose, scope, author, date, version: front matter or "Key: value" lines in the first 40 lines; counters per field | repository | clone | both | M | P2 |
| DOC7 | Diagrams and images | Relative images present or broken, external images (badges excluded), `mermaid` blocks, `Media/` folder | repository | clone | both | H | P2 |
| DOC8 | Archives | `.zip`, `.rar`, `.7z`, `.tar`, `.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz` (case ignored) **in the whole repository** (maintainer decision); `.docx` and `.pdf` files under `documentation/` are flagged as information | repository, tag | clone | both | H | P1 |
| DOC9 | Freshness | Dates of the last commit on `documentation/`, the INDEX, the CHANGELOG and the code; commits outside the documentation since; lead | repository | clone | both | M | P2 |
| DOC10 | Declared dates | Date in the INDEX column compared to the document's last commit | document | clone | both | L | P3 |

The lists of DOC3, DOC4 and DOC5 (paths) are recomputed at display time; only the counters are stored.

### 5.8 Tooling (TL)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TL1 | commitlint configured | Configuration file (depth ≤ 2) or `commitlint` / `@commitlint/cli` key in `package.json`; sub-state "extends config-conventional"; date of introduction | repository | clone | both | H | P2 |
| TL2 | Versioned `commit-msg` hook | `.husky/commit-msg` (with `prepare: husky`), lefthook, pre-commit as `commit-msg`, cz, cog. Status "configured", **never "active"** | repository | clone | both | M | P2 |
| TL3 | PR template | Locations recognised by GitHub, case ignored; number of charter sections present. Absent: "not found in the repository" (an organisation template is invisible) | repository | clone | both | H | P2 |
| TL4 | CI checking the conventions | A workflow cites a known action (commitlint, semantic-pull-request…); only the pattern identifier is kept | repository | clone | both | M | P3 |
| TL5 | Effect of the tooling | CC1 before and after the earliest introduction among TL1, TL2, TL4; "n/a" if a sample is smaller than 10 | repository | database + clone | both | M | P3 |

**Phase 8.3: details of the file criteria.** The pure functions `CheckChangelog`, `CheckReadme`, `CheckIndex`,
`CheckDocsTree` and `DetectTooling` read the list of paths of the tree and the content of the files they ask
for only (CHANGELOG, README, INDEX, `.md` documents of the documentation, at most 300 for DOC4, tooling files
and PR templates). A result keeps only identifiers, states, codes and counters; the path lists (DOC3, DOC4,
DOC5, DOC8) are returned apart. Their rules version, `FileRulesVersion`, is separate from the one of the
commits: a change of the commit rules does not invalidate the cache of the files. Detailed choices:

- **Missing file**: the presence criterion (CL1, RD1, DOC1) is nonconforming and the criteria that read the
  file are "unknown" (`no_file`), so that the absence counts once. A misnamed or misplaced file (`misnamed`,
  `elsewhere`) is still analyzed. Tree truncated at the cap or empty repository: all the criteria are
  "unknown"; a file over the read cap or not read: "unknown" too.
- **Markdown analysis**: ATX headings only (no underlined headings), outside fenced code blocks and HTML
  comments; links `[x](y)`, definitions `[x]: y` and GFM tables (delimiter row with as many columns as the
  header). Headings are compared after normalization (case, accents, punctuation, leading numbering) with
  French and English synonyms versioned with `FileRulesVersion`.
- **CL6**: the order of the categories is a **warning** (`category_order`), not an issue: Watchoo's own
  CHANGELOG, the conforming case of the tests, has an old version with its categories in another order, and
  Keep a Changelog does not require it. Unknown categories, duplicates and bullets outside a category stay
  issues. French labels (categories, H1, `[Non publié]`) give the `french_label` warning when
  `changelog.categories_fr` is true, an issue otherwise; `[v1.2.0]` gives the `v_prefix` warning.
- **CL7**: `[Unreleased]` is not evaluated at all (empty category or "TODO" included).
- **CL8**: only the names of the tags are read (a tag that designates a final version, even under a
  nonconforming name); the date gap between a tag and its section is not shown yet. As the tags change
  without the head changing, their names enter the fingerprint of the settings of the cache. On the tree of a
  milestone, only the tags and sections of lower or equal precedence count.
- **CL4**: a future date is future relative to the day of the computation; the cached result is reviewed
  only at the next computation.
- **CL9** is measured on the tree of the tag of a milestone only (the section is searched by its version,
  whatever the form of the heading, judged by CL4).
- **RD2**: a licence file at the root counts as the license section (`license_file`): `LICENSE`, `LICENCE` or
  `COPYING`, case ignored, without extension or with the `.md` or `.txt` extension. Only a
  mask of the sections found and numbers are kept.
- **RD3**: a link to an `index.md` at the root or in a `doc`, `docs` or `documentation` folder counts as a
  link to the INDEX; a link that matches the target only when ignoring case is dead (GitHub is case
  sensitive). A URL of the same repository is `https://github.com/<owner>/<name>` (repository page) or
  `…/blob|tree|raw/<ref>/<path>` (single-segment reference), compared with the current name of the
  repository, case ignored: documented exception to the rule "nothing specific to GitHub outside
  `internal/provider/`" (text comparison, no access to the forge), like the catalog of `internal/aisignature`.
- **DOC3**: duplicate rows are counted in the complete tables only (the five columns of DOC2). Without an
  INDEX at the expected place, the evaluated documentation is the folder of the INDEX found.
- **DOC4**: images (`![x](y)`) belong to DOC7, not DOC4. No dead link among the files read, but files not
  read: "unknown" (`unread_documents`).
- **DOC5**: every subfolder of the documentation follows `NN-topic`, except the media folder (case ignored)
  and its content; the files of the media folder follow the rule of the files. Hidden files are exempt.
- **TL1**: depth at most 2 folders, outside `node_modules/` and `vendor/`; the introduction date (which needs
  the history) is not measured yet. **TL2**: `.husky/commit-msg` without a `prepare` script installing husky
  gives the `husky_no_prepare` warning. **TL3**: only `.md` files are recognized.

**Codes, counters and reasons of the file criteria (`--json`).** Full list, fixed by `FileRulesVersion`; a
code keeps the same meaning from one criterion to another. A counter whose key is a code counts the elements
(versions, categories, bullets, links, paths) that carry this code.

| Criterion | Blocking codes | Warnings | Counters |
| --- | --- | --- | --- |
| CL1 | `absent`, `misnamed` (other case or extension at the root), `elsewhere` (elsewhere in the repository) | — | — |
| CL2 | `no_h1`, `h1_title` (level-1 heading other than "Changelog"), `no_keepachangelog`, `no_semver` | `french_label` | — |
| CL3 | `unreleased_absent`, `unreleased_misplaced`, `unreleased_duplicate`, `unreleased_no_brackets` | `french_label` | — |
| CL4 | `no_brackets`, `date_fr`, `no_date`, `invalid_date`, `future_date`, `invalid_version` | `v_prefix` | `versions`; one counter per code |
| CL5 | `version_order`, `date_order` | — | `versions`, `version_order`, `date_order` |
| CL6 | `unknown_category`, `duplicate_category`, `bullet_outside_category` | `french_label`, `category_order` | `sections`; one counter per code |
| CL7 | `empty_version`, `empty_category`, `empty_bullet` | — | `versions`; one counter per code |
| CL8 | `tag_without_section`, `section_without_tag` | — | `tags`, `tag_without_section`, `section_without_tag` |
| CL9 | `version_missing` | — | — |
| RD1 | `absent`, `misnamed`, `elsewhere` | `translation_only` | `translations` |
| RD2 | `missing_section` | — | `expected`, `found`, `expected_mask`, `found_mask`, `license_file` (1 if a licence file is at the root), `missing_section` |
| RD3 | `index_link_absent`, `index_link_dead`, `index_link_folder`, `changelog_link_absent`, `changelog_link_dead` | — | — |
| RD4 | `too_short` | — | `words`, `min_words` |
| DOC1 | `absent`, `misnamed`, `elsewhere` | — | — |
| DOC2 | `no_table`, `missing_column`, `no_rows` | — | `tables`, `rows`, `columns`, `missing_column` |
| DOC3 | `unreferenced`, `duplicate_row` | — | `documents`, `unreferenced`, `duplicate_row` |
| DOC4 | `dead_link`, `case_mismatch` | — | `links`, `sources`, `unread`, `dead_link`, `case_mismatch` |
| DOC5 | `dir_name`, `file_name` | — | `dirs`, `files`, `dir_name`, `file_name` |
| DOC8 | `archive` | `office_document` | `archive`, `office_document` |
| TL1 | `not_found` | — | `configs`, `extends_conventional` (1 if a configuration extends config-conventional), `unread` |
| TL2 | `not_found` | `husky_no_prepare` | `tools_mask`, `unread` |
| TL3 | `not_found` | — | `templates`, `expected`, `sections` (the largest number of charter sections found in one template), `unread` |

- **Masks**: bit `i` (value `1 << i`) for the `i`-th element, from 0. `expected_mask` and `found_mask` follow
  the order of the README sections: `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`,
  `license`. `tools_mask` follows the order of the TL2 tools: `husky`, `lefthook`, `pre-commit`,
  `commitizen`, `cocogitto`.
- **Reasons of a family** (`reason` of `changelog`, `readme`, `docs`, `tooling`, repeated by all its
  criteria): `not_computed` (not computed yet: run `watchoo sync`), `stale` (computed for another revision,
  another version of the rules or other settings: to recompute), `no_clone`, `filtered` (report restricted to
  contributors).
- **Reasons of a criterion** in a computed family: `empty_repository` (empty repository), `tree_truncated`
  (list of paths truncated at the cap), `no_file` (the file read by the criterion is absent, its absence is
  counted by CL1, RD1 or DOC1), `blob_too_large` (file beyond 1 MiB), `blob_unreadable` (file listed but not
  read), `no_docs_dir` (documentation folder absent), `unread_documents` (DOC4: no dead link among the files
  read, others not read).
- **Reasons of the lists** (`docs.lists.reason`, detailed view of a repository): `not_computed`, `stale`,
  `no_clone`, `clone_unreadable` (tree unreadable in the clone), `tree_truncated`.
- **Milestone review** (`milestone.repositories[].files.reason` and `reason` of its file criteria): the SV1
  reasons when the milestone tag is absent (`no_tag`, `not_due`, `tag_absent`), then `not_computed`, `stale`,
  `no_clone`, and in a computed revision the reasons of a criterion above.

## 6. What cannot be measured

| Requirement | Why | What Watchoo shows |
| --- | --- | --- |
| Meaning of the message, right type, CHANGELOG "for humans" | Human judgment | Syntax (CC2), obvious wording (CC5), probable copy (CL12) |
| Imperative in French | "ajoute" is both imperative and present; the infinitive is common | Obvious cases only: past participle, first person |
| Hook installed and active | `.git/hooks` is not versioned; `--no-verify`; web edits | "Configured" (TL2), measured effect (TL5) |
| One branch, one PR = one subject | Semantics | Reused branch (BR2), PR size in context |
| Quality of the review, real identity of the reviewer | Human; multiple accounts | Approval, delay, excluded accounts |
| Protection of the default branch | Admin rights required for the details; concept absent from git; depends on the GitHub plan; BR3 postponed | Observed effects: commits outside a PR (PR3), rewrites (BR4) |
| Tags "never rewritten", date a tag was set | Snapshots only; no tag date | "No rewrite observed since …", first observation (SV3) |
| Semantic correctness of the version bump | No declared public API | Consistency with the declared commits (SV5) |
| Reproducible installation | The code would have to be run | Lead RD5 |
| Accuracy of the README, the INDEX, the headers | Personal data would have to be cross-checked | Presence and form |
| Branches created and deleted between two syncs | Invisible (`fetch --prune`) | Nothing |

## 7. Output

**No overall grade.** Each family shows an explained `n/m` rate (denominator and exclusions). A checklist
counts **criteria** ("7/9 criteria compliant, 1 unknown"); it always opens the details and neither sorts
nor ranks anything.

- **Per repository**: CC1, BR1, PR1, PR3, PR4, SV1, SV2, then the CHANGELOG (CL1 to CL8; CL9 on the tag of
  a milestone), README (RD1 to RD4), documentation (DOC1 to DOC5, DOC8) and tooling (TL1 to TL3) grids.
- **Per contributor**: CC3 and its main patterns, PR titles (PR1), commits that arrived outside a PR that
  they authored (PR3), reviews received on their PRs (PR4; the reviews given are not counted, since the
  reviewers are not stored). No repository indicator (CL, RD, DOC, SV) is attributed to a person.

### 7.1 Milestone review (RJ)

Criterion × milestone table (`v1.0.0`, `v2.0.0`, `v3.0.0`). File criteria (RD, DOC, CL) are evaluated
**on the tag's tree** (cached per tag-SHA pair); commits and PRs **on the range between the previous tag
and the milestone tag**. Missing tag: range bounded by the configured deadline, otherwise "not
evaluable". `conventions.since` is mandatory for the RJ.

| Criterion G | Indicators | Target |
| --- | --- | --- |
| 100% compliant commits | CC1 on the range, all tracked branches | 100% |
| Branches per feature, `main` protected, PRs reviewed | BR1, PR3 = 0, PR4; protection "not evaluated" (BR3 postponed) | compliant, or "unknown" with a clone only |
| Milestone tags never rewritten | SV1, SV2, SV3 | present, no rewrite observed |
| CHANGELOG up to date at the milestone | CL9, CL8 | section present at the tag |
| README with installation | RD1, RD3, installation section of RD2 (RD5 as a lead) | present; the other RD2 sections as information |
| Exhaustive INDEX | DOC1, DOC3 | 0 unreferenced documents |

**Phase 8.2: first milestone review.** For one configured milestone (`tags.milestones`), each repository
gets a list of criteria, `conform`, `nonconform` or `unknown` (conform, not conform, unknown), with no overall score and no sorting ("k/n
criteria conform, u unknown"); they are repository indicators, attributed to nobody. Only the criteria
measured in 8.2 appear:

- **CC1** on the milestone interval: from the previous final tag (SemVer precedence) to the tag that
  designates the milestone (exact or nonconforming name); missing tag: commits of the tracked branches since
  the previous final tag authored before the end of the due date, "unknown" before that end (`not_due`),
  "unknown" (`no_tag`) without a due date. Conform when every evaluated commit conforms; "unknown" without a clone,
  without an evaluated commit, or when commits of the interval are not in the database;
- **BR1**, current state: conform when no branch has a blocking issue; "unknown" without a branch to
  evaluate;
- **SV1** of the tag that designates the milestone; missing tag: "unknown" (`no_tag` without a due date,
  `not_due` before the end of the due date, `tag_absent` after it: no name to judge, the absence is counted
  by SV2);
- **SV2**: conform when the tag with the exact name exists; tag missing before the end of the due date:
  "unknown".

Without a start date (`conventions.since` of the workspace or of the repository), the repository is "not
evaluable: missing start date". PR3 and PR4 are added with phases 8.5 and 8.6.

**Phase 8.4: SV2 deadline and SV3 in the review.** With a due date, **SV2** also judges when the tag with the
exact name was set: "on time" conform, "late" not conform, "undetermined" unknown (`deadline_undetermined`).
**SV3** follows SV2 (the "milestone tags never rewritten" criterion combines SV1, SV2 and SV3): conform when no
rewrite of the tag that designates the milestone was observed (stable, or deleted then set again on the same
commit), not conform when it was rewritten or deleted; missing tag: "unknown" with the reason of SV1; tag
never observed: "unknown" (`not_observed`). The review therefore counts twelve criteria.

**Phase 8.3: file criteria of the review.** Seven criteria follow SV2, read from the cache of the sync for
the **tree of the tag** that designates the milestone (same rule as SV1: exact name, else the first
nonconforming name): **CL9** and **CL8** ("CHANGELOG up to date at the milestone"), **RD1**, **RD2** and
**RD3** ("README with installation"; in the review, RD2 judges **only the installation section**: conform
when its heading is found; the other sections found are given for information, as `readme_found`), **DOC1**
and **DOC3** ("INDEX exhaustive"). Tag absent: the seven criteria are "unknown" with the reason of SV1
(`no_tag`, `not_due`, `tag_absent`). No result for the tag: "unknown" (`not_computed`: not computed yet,
run `watchoo sync`; `no_clone` without a local clone); a result of another commit (tag moved), of another
`FileRulesVersion` or of another fingerprint of the settings: "unknown" (`stale`, to recompute). A file
absent from the tree counts once (RD1 or DOC1 not conform), the criteria that read it being "unknown"
(`no_file`).

### 7.2 Interfaces

- **Web**: `/conventions` route (overview and milestone review), navigation entry only if
  `conventions.enabled`. "Conventions" section on the repository page (badges per PR, dot per tag,
  "Tooling" block); "Compliant commits 18/25 (72%)" line on the contributor's page; compact dot on the
  dashboard, without sorting by this value. Icons always come with a text.
  Phase 8.2: `/conventions` shows the overview by repository (order of the workspace) and by contributor
  (by name), never sorted by conformity, and the milestone review of the milestone chosen with
  `?milestone=vX.Y.Z` (first configured milestone by default, 400 for a milestone that is not configured);
  disabled, it explains how to enable the conventions, without data. The repository section adds the
  non-conforming branches, a badge per PR (number and codes, never the title), a dot per tag, the
  milestones, the releases (SV6, information) and CC1 per interval between tags; the contributor's page
  adds the conforming PR titles and the CC3 trend (two rates). The compact dot on the dashboard is not
  there yet. Detail in [06-web-interface.md](06-web-interface.md#7-conventions-180-phase-82).
- **CLI**: a single command, with no subcommand (a repository named like a subcommand would be ambiguous):
  `watchoo conventions [repo] [--commits] [--failing] [--contributor <x>] [--milestone vX.Y.Z] [--json]`.
  `--commits --failing` lists the short SHA, the author and the issue codes, never the message. No
  `--strict` option (exit code depending on compliance) for now: it can be added later in a MINOR.
  Phase 8.2: the pull requests are shown by **number, author, head branch and codes**, never by their
  title; `--milestone vX.Y.Z` shows the milestone review instead of the overview (a milestone that is not
  configured is a usage error); details in [05-cli.md](05-cli.md#commands).
- **JSON**: keys in English (`commits.rate`, `commits.issues[{code,count,shas}]`, `semver_checks[]`,
  `changelog.rules[]`…), frozen by `internal/cli/json_schema_test.go`. No key added to `prs` or
  `releases` at first. Phase 8.2 adds keys only (`traits`, `language`, `branches`, `pull_requests`, `tags`,
  `intervals` per repository; `traits`, `trend`, `pull_requests`, `branches` per contributor; top-level
  `milestone`, `null` without `--milestone`), the same top-level keys whatever the options.
  Phase 8.3 adds keys only: `changelog`, `readme`, `docs` and `tooling` per repository (`state`, `reason`,
  `sha`, `computed_at`, `conform`, `unknown`, `total`, `criteria[{id,status,reason,codes,counts}]`; `docs`
  adds `lists`, filled only in the detailed view of a repository and empty otherwise, so that the keys stay
  the same) and `files` per repository of the milestone review.
- **JSON, phase 8.4**: only adds keys: `rewrites`, `tag_history`, `prereleases`, `bumps`, `precedence` and
  `changelog_freshness` per repository, `deadline` per milestone of `tags.milestones`, `deadline` and
  `tag_history` per repository of the milestone review; details in [05-cli.md](05-cli.md#json-output).
- **Phase 8.3: reading of the file criteria.** `Build` and `ReviewMilestone` only read the cache of the
  sync (`repository_conventions`), never the clone, except for the path lists. The head of the default
  branch is compared with the last known commit of the branch in the database, a milestone tag with its
  commit in the database; a result is used only for the same commit, the same `FileRulesVersion` and the
  same `config_hash`, otherwise "to recompute" (`stale`); no row: "not computed yet, run watchoo sync"
  (`not_computed`), or "no local clone" (`no_clone`). There is no score: one grid per family, "k/n criteria
  conform, u unknown", credited to nobody (with `--contributor`, they are "unknown" with `filtered`; a
  contributor's page does not show them). The path lists (DOC3 unreferenced documents and duplicate rows, DOC4 dead
  links, DOC5 badly named paths, DOC8 archives and office documents) are recomputed from the clone, at the
  commit of the cached result, only in the detailed view of a repository (`watchoo conventions owner/repo`,
  repository page); they are neither stored nor logged. CLI and web display the same codes, with a label
  shared by both (`conv.*` keys of the catalog). The overview of `/conventions` gives one summary badge per
  family; the repository page gives the grids and, folded, the path lists.
- All texts go through `internal/i18n`, in English and in French.

## 8. Configuration

`conventions:` block of the settings, separate from `diagnostic:`; the default values are those of the
charter in [section 3](#3-reference-the-charter).

| Setting | Default | Role |
| --- | --- | --- |
| `enabled` | `false` | Turns the conventions on |
| `since` | empty | Start date of the evaluation (mandatory for the milestone review); can be overridden per repository (`watchoo repo add --conventions-since YYYY-MM-DD`, like `--flemme` and `--dead`; `repositories.conventions_since` column) |
| `commit.types`, `commit.scopes` | charter types, all scopes | Allowed types and scopes |
| `commit.max_subject_soft`, `commit.max_subject_hard` | 50, 72 | Subject length (warning, blocking) |
| `commit.forbidden` | charter list | Forbidden subjects |
| `commit.language` | `any` | `fr`, `en` or `any`; information only (CC7), no effect on compliance |
| `branch.pattern` | empty (built-in rule `^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$`, built from `commit.types`) | Branch naming (BR1): an RE2 expression, validated at load time, that replaces the built-in rule |
| `branch.exclude` | `gh-pages`, `dependabot/*`, `renovate/*` | Branches left out (BR1): glob patterns (`*`, `?`, `[...]`, `path.Match` syntax), each one designating the name itself or one of its path prefixes (`dependabot/*` excludes `dependabot/npm_and_yarn/lodash-4.17.21`); the default branch is always left out |
| `pr.body_sections`, `pr.require_closes` | charter sections, `true` | PR body |
| `pr.min_reviews`, `pr.reviewer_must_contribute` | 1, `false` | Reviews; another account is enough, unless the reviewer must be a contributor |
| `tags.milestones` | `v1.0.0`, `v2.0.0`, `v3.0.0` (no deadline) | Milestones (SV2, milestone review): each entry `vX.Y.Z`, or `vX.Y.Z@YYYY-MM-DD` for a deadline |
| `tags.prerelease_forms` | `rc`, `beta`, `alpha` | Compliant pre-release forms `-<form>.N` (SV4, phase 8.4); the other forms give a warning, never a deviation |
| `changelog.categories_fr` | `true` | Allows French labels (categories, H1, `[Non publié]`) with a warning; `false`: they are deviations (CL2, CL3, CL5) |
| `readme.sections` | the eight charter sections | Expected README sections (RD2): keys among `presentation`, `group`, `stack`, `install`, `run`, `env`, `tree`, `license`; their FR and EN synonyms are built in and versioned |
| `readme.min_words` | 150 | Number of words below which the README is too short (RD4), at least 1 |
| `docs.dir`, `docs.media_dir`, `docs.archives_scope` | `documentation`, `Media`, `repository` | Documentation folder (relative to the root), media folder (a single segment under `docs.dir`); archives forbidden in the whole repository (`repository`) or under `docs.dir` only (`documentation`) |
| `excluded_accounts` | empty | Excluded accounts (trainer, bots): author's login, name or e-mail, case ignored; specific to the conventions, separate from `diagnostic.exclude` |

The block is shared through [`configshare`](02-features.md) (maintainer decision), except `enabled`,
`excluded_accounts` (which contains logins): sharing a charter is the intended use. A repository's own date
is exported with it, like its activity thresholds. The keys are then frozen by SemVer.

**Phase 8.1**: only `enabled`, `since`, `commit.*` and `excluded_accounts` exist; the other keys come with
their phase (see **Phase 8.2** below). Environment variables: `WATCHOO_CONVENTIONS_ENABLED`, `WATCHOO_CONVENTIONS_SINCE`,
`WATCHOO_CONVENTIONS_COMMIT_TYPES`… (comma-separated lists). Rules checked at load time: `since` empty or of
the form `YYYY-MM-DD` (an unquoted YAML date, `since: 2026-09-01`, is accepted; a date with a time is
refused, except an unquoted YAML timestamp at exactly midnight UTC, `2026-09-01T00:00:00Z`, which the YAML
decoder cannot tell from the date alone and which is read as `2026-09-01`), read at midnight in the workspace time zone; at least one type, each in lowercase letters; each
scope of the form `[a-z0-9][a-z0-9._/-]*`; `commit.max_subject_soft` at least 1 and
`commit.max_subject_hard` strictly greater; no empty entry in `commit.forbidden` nor `excluded_accounts`;
`commit.language` among `fr`, `en`, `any` (no effect on the analysis in 8.1; information of CC7 since 8.2). In the sharing file, the
block is written `settings.conventions` (`since`, `commit`): see
[Configuration sharing](05-cli.md#configuration-sharing-watchoo-config).

**Phase 8.2**: `branch.pattern`, `branch.exclude` and `tags.milestones` are added (1.8.0), shared like
`commit.*`. Environment variables: `WATCHOO_CONVENTIONS_BRANCH_PATTERN`,
`WATCHOO_CONVENTIONS_BRANCH_EXCLUDE` and `WATCHOO_CONVENTIONS_TAGS_MILESTONES` (comma-separated lists for the
last two; the pattern is never split: `{2,40}` stays whole). Rules checked at load time:

- `branch.pattern` empty (built-in rule) or a valid **RE2** regular expression (Go syntax: no lookbehind
  nor lookahead, `(?!…)` is refused); a branch name that does not match the given pattern gets the single
  code `pattern`. The pattern is applied as is, **without anchoring**: a match anywhere in the name is
  enough (`(feat|fix)/[a-z-]+` accepts `WIP_Feat/feat/x`); add `^…$` to require the whole name to follow
  it, like the built-in rule;
- `branch.exclude`: each pattern non-empty and valid for `path.Match`. A pattern designates the name itself
  or one of its **path prefixes**, like `.gitignore`: `path.Match` alone does not cross `/`, and
  `dependabot/*` would not have matched the real Dependabot branches (`dependabot/<ecosystem>/<dependency>`).
  This reading is specific to `conventions.branch.exclude`: `branches.exclude` (branches tracked by the
  sync) keeps the strict `path.Match` reading;
- `tags.milestones`: each entry is a compliant tag name (SV1) of a **final version**, lowercase `v`
  followed by `MAJOR.MINOR.PATCH` without pre-release nor build metadata (`v1.0.0`; `1.0.0`, `V1.0.0`,
  `v1.0`, `v1.0.0-rc.1` and `v1.0.0+build` are refused), followed if needed by `@YYYY-MM-DD`, a valid
  calendar date (`v1.0.0@2026-12-18`; the deadline runs until the end of the day, in the workspace time
  zone); a version appears only once; an empty list declares no milestone.

`tags.prerelease_forms` did not exist in 8.2: it comes with SV4 (phase 8.4, see below). SV1 still accepts
any valid SemVer pre-release, whatever its form (`-rc1` as well as `-rc.1`); SV4 is the one that warns about
a free form.

In the sharing file, these keys are written `settings.conventions.branch` (`pattern`, `exclude`) and
`settings.conventions.tags` (`milestones`); a file that contains them is refused by the versions before
1.8.0, which accept no unknown key.

**Phase 8.3**: `changelog.categories_fr`, `readme.sections`, `readme.min_words`, `docs.dir`,
`docs.media_dir` and `docs.archives_scope` are added (1.9.0), shared like `commit.*`. Exact syntax:

```yaml
conventions:
  changelog:
    categories_fr: true          # boolean
  readme:
    sections: ["presentation", "group", "stack", "install", "run", "env", "tree", "license"]
    min_words: 150               # integer, at least 1
  docs:
    dir: "documentation"         # path relative to the repository root
    media_dir: "Media"           # a single folder name, under docs.dir
    archives_scope: "repository" # "repository" or "documentation"
```

Environment variables: `WATCHOO_CONVENTIONS_CHANGELOG_CATEGORIES_FR`, `WATCHOO_CONVENTIONS_README_SECTIONS`
(comma-separated list), `WATCHOO_CONVENTIONS_README_MIN_WORDS`, `WATCHOO_CONVENTIONS_DOCS_DIR`,
`WATCHOO_CONVENTIONS_DOCS_MEDIA_DIR` and `WATCHOO_CONVENTIONS_DOCS_ARCHIVES_SCOPE`. Rules checked at load
time:

- `readme.sections`: each key among the eight above (lowercase, as is), only once; an empty list expects
  no section (RD2 is then compliant by default). The synonyms recognized for each key ("Installation",
  "Getting started"…) cannot be set: they are built into the binary and versioned with the rules;
- `readme.min_words`: an integer of at least 1;
- `docs.dir`: a path relative to the root, non-empty segments separated by `/`, without a leading or
  trailing `/`, without a `.` or `..` segment, without a space at the start or end of a segment, without `\` or a
  control character, at most 255 bytes
  (`documentation`, `docs/project`);
- `docs.media_dir`: a single segment of the same form (`Media`, `images`), looked for under `docs.dir`;
- `docs.archives_scope`: `repository` (archives forbidden in the whole repository, default) or
  `documentation` (under `docs.dir` only).

These settings are part of the fingerprint of the cached results (`config_hash`): changing one of them
recomputes the file criteria at the next sync. In the sharing file, they are written
`settings.conventions.changelog` (`categories_fr`), `settings.conventions.readme` (`sections`, `min_words`)
and `settings.conventions.docs` (`dir`, `media_dir`, `archives_scope`); a file that contains them is refused
by the versions before 1.9.0, which accept no unknown key (as is a `config.yaml` that contains them).

**Phase 8.4**: `tags.prerelease_forms` is added (1.10.0), shared like `commit.*`. Exact syntax:

```yaml
conventions:
  tags:
    prerelease_forms: ["rc", "beta", "alpha"] # compliant forms "-<form>.N"
```

Environment variable: `WATCHOO_CONVENTIONS_TAGS_PRERELEASE_FORMS` (comma-separated list, `rc,preview`).
Rules checked at load time: **at least one form**; each form is a word of 1 to 32 lowercase ASCII letters
(`rc`, `preview`; `RC`, `rc1`, `pre-release` are refused); a form appears only once. A pre-release has a
compliant form if it is written exactly `-<form>.N`, the form in the list and `N` an integer without a
leading zero (`v1.0.0-rc.1`); any other form (`-rc1`, `-RC.1`, `-pre`) gets a SV4 warning, never a
deviation. In the sharing file, the key is written `settings.conventions.tags.prerelease_forms`; a file that
contains it is refused by the versions before 1.10.0 (as is a `config.yaml` that contains it).

## 9. Technical impact

- **Packages**:
  - `internal/conventions`: the parser and the aggregation are pure functions, modelled on
    `internal/aisignature`, that return stable codes and read neither the database nor the forge; `Build`
    reads the database and the clone only through the `Reader` and `CloneReader` interfaces, implemented by
    `internal/store` and `internal/provider/git`, never the forge (`ParseCommit`, `CheckBranch`, `CheckPRTitle`,
    `CheckPRBody`, `CheckTag`, `ExpectedBump`, `CheckChangelog`, `CheckReadme`, `CheckIndex`,
    `CheckDocsTree`, `DetectTooling`);
  - `internal/semver`: comparator extracted from `internal/selfupdate`, with no change to the surface;
  - bot recognition moves from `internal/diagnostic` to `internal/aisignature` (account catalogue).
- **Sync**: `internal/sync/conventions.go`, called when `conventions.enabled`. The clone is read through
  optional interfaces of the git provider (`TreePaths`, `ReadBlobs`, `FirstParent`, `IsAncestor`,
  `RevList`), with no addition to the `Provider` interface. On the `Provider` side: `PullRequest.MergeCommitSHA`
  (filled only if the PR is merged), `PullRequest.FromFork` and
  `PullRequestReviews` (GET; `ErrUnsupported` with a clone). The computation is only rerun if the head of
  the default branch, `RulesVersion` or the configuration fingerprint has changed. Failures logged at
  Debug level, without message or path.
- **Sync, done in phase 8.3** (file criteria):
  - the computation only runs if `conventions.enabled` and if the clone was updated during this sync
    (otherwise the previous result stays); no API request, no extra fetch;
  - measured revisions: the head of the default branch (SHA read in the branch list of the sync source)
    and, for every configured milestone, the tag that designates it (same rule as the milestone review:
    exact name, otherwise the first non-conforming name), whose SHA is read **in the database**; no tag
    name is passed to git. In API mode, if the tag list was not reread (low quota), the tags already in
    the database are used: an accepted approximation, consistent with the milestone review;
  - reading through the Git objects only: `git ls-tree -r -l -z --end-of-options <sha>` (at most 20,000
    files; beyond, the list is "truncated" and the criteria unknown), then `git cat-file --batch` on the
    **object identifiers** (never the paths), without filter or conversion: a file over 1 MiB is "too
    large" (unknown), at most 32 MiB read per revision (beyond, files "unreadable"). Symbolic links and
    submodules are ignored; an LFS file is read as its pointer; a path that is not valid UTF-8 or that
    contains a control character is ignored;
  - recomputed only if the measured commit, `FileRulesVersion` (version of the file rules, separate from
    the `RulesVersion` of the commit messages) or `config_hash` changed; `config_hash` covers the settings
    `conventions.changelog.*`, `conventions.readme.*`, `conventions.docs.*` **and** the names of the tags
    that designate a version (CL8 compares them to the sections): a new version tag therefore triggers a
    recomputation. The repository (`owner/name`, case ignored) is covered too: RD3, DOC3 and DOC4 recognize
    the URLs of the same repository, so a rename or a transfer triggers a recomputation;
  - rows of the tags that no longer designate a configured milestone (milestone removed, tag deleted from
    the repository) are deleted; the head row stays, unless the source no longer has a default branch
    (emptied repository, `HEAD` on a missing branch): it is then deleted too;
  - failure: logged at Debug level with the repository, the number of revisions and the reason (without
    path, tag name or content), the repository sync succeeds; a database access failure (reading or writing the cache, reading the
    tags) is logged at Debug level as well;
  - a "future" date (CL4) is judged on the day of the computation, in the time zone of the workspace.
- **Sync, done in phase 8.4** (history):
  - only when `conventions.enabled`; written in the transaction of the sync of the repository, at its date;
  - `tag_observations` is updated only when the read of the tags by the sync succeeded, is not truncated
    (fewer than 1,000 tags: a list of exactly 1,000 tags is taken as truncated) and skipped no tag (empty
    name, invalid SHA); otherwise nothing is opened or closed. Same name with another SHA: the old row is
    closed (`rewritten`) and a new one opened; name gone: closed (`deleted`);
  - `branch_rewrites`: at most one event per sync, for the default branch tracked before and after, under
    the same name, two different full SHAs; ancestry read by `git merge-base --is-ancestor --end-of-options`
    in the clone updated by the sync (both modes); in API mode without a clone that decides,
    `ErrUnknownBase` on the old head counts as a rewrite;
  - no tag name, branch name nor SHA in the logs (Debug, repository name only); the sync of the repository
    never fails because of these computations.
- **Data** (migrations from `00016`, described first in [Data model](04-data-model.md)):
  - `commits.parent_count`, written at sync time from `Commit.Parents`; existing rows are backfilled by an
    `UPDATE` from `git rev-list --parents` (insertion never modifies a commit already in the database);
  - `repository_conventions(repo_id, rev_sha, rev_kind, tag_name, rules_version, config_hash, results,
    computed_at)`: codes and counters, per head and per tag;
  - `tag_observations(repo_id, name, sha, first_seen_at, last_seen_at, ended_at, end_reason)`;
  - `branch_rewrites(repo_id, branch, old_sha, new_sha, observed_at)`;
  - `repositories.conventions_since` (NULL = the workspace's date);
  - `pull_requests.merge_commit_sha`, `from_fork`, `body_sections` (NULL = unknown),
    `reviews_by_others`, `approved_before_merge`, `reviews_checked_at`: counters and flags, never the
    reviewers' logins (maintainer decision);
  - `branches.protected` is not added while BR3 is postponed;
  - that is 19 tables instead of 16.
  - CC, BR1, PR1, PR2, SV1, SV2 and SV6 are computed on the fly from the database: a charter change takes
    effect immediately.
- **Quota**:
  - phases 8.1 to 8.4: no request;
  - 8.5: no request in steady state; turning the conventions on (or raising `RulesVersion`) triggers
    **one** full reread of the PR list (at most 10 pages per repository) to fill `merge_commit_sha`,
    `from_fork` and `body_sections`;
  - 8.6: one request per merged PR, only once, plus the open PRs whose `updated_at` changed; for example
    30 repositories × 40 PRs = 1,200 requests on the first pass (24% of the hourly quota), capped per sync
    and cut below the quota reserve. The ETag cache is shared in `watchoo serve` but empty for each CLI
    process;
  - releases are already reread in full at every sync: cost unchanged.
- **Interaction with the diagnosis**: the squash merge imposed by the charter produces large single commits
  on the default branch, which feed A1 (massive commit) and A2 (repository delivered in one go) and erase
  the material of H4. When `conventions.enabled` is on, identified squash commits (`merge_commit_sha` of a
  PR, or "(#n)" with the GitHub committer) are excluded from A1 and A2, and the "novelty: Conventional
  Commits subject" criterion of F4 is neutralised after `conventions.since`. To be reported in
  [Supervision and diagnosis](12-supervision-diagnostic.md).
- **Public surface**: new command, route, settings, JSON keys and additive migrations: MINOR.
- **Tests**: case tables in `internal/conventions` (each CC2 code, charter examples, official regex from
  semver.org, Watchoo's CHANGELOG as a compliant case); `gittest` (tooling, CHANGELOG at each tag, tag
  rewritten and branch rewritten by `push --force`, direct push and merge); `githubtest`
  (`merge_commit_sha`, `/pulls/{n}/reviews`); CLI (`conventions --json`) and web e2e; EN and FR i18n
  catalogues.

## 10. Phase plan (proposal)

Each phase can be delivered alone; phases 8.1 to 8.4 are enough for a milestone review with a clone only,
8.5 and 8.6 complete the "unknown" criteria.

| Phase | Content | Cost | Version |
| --- | --- | --- | --- |
| 8.0 | This document approved (done, November 8, 2026) and exception carried over into 01; updates to 02, 04, 05, 06 and 12 with each phase | no code | — |
| 8.1 | `internal/conventions` (parser), `internal/semver`, `conventions.*` settings, CC1 to CC4, `commits.parent_count` and its backfill, `watchoo conventions --commits`, web section and contributor page, adjustments to A1, A2 and F4. **Done, not released** (`[Unreleased]` of the CHANGELOG; `scope` code added to the blocking codes of CC2; CC1 per range between tags and CC3 trend since the last tag postponed to 8.2, with the reading of the tags) | 0 quota, 1 migration | 1.7.0 |
| 8.2 | On the fly: BR1, PR1, PR2, SV1, SV2 (presence), SV6; first milestone review; CC5 to CC7; CC1 per range between tags, CC3 trend. **Done, not released** (`[Unreleased]` of the CHANGELOG; `branch.pattern`, `branch.exclude` and `tags.milestones` settings; `/conventions` route; approximations listed in [5.4](#54-tags-and-semver-sv): forks included until 8.5, PR2 comparison with the only commit not measured, API mode inferred; compact dot on the dashboard postponed) | 0 quota | 1.8.0 |
| 8.3 | Clone tree: `repository_conventions`, CL1 to CL9, RD1 to RD4, DOC1 to DOC8, TL1 to TL3; milestone review on the tags' tree. **Done, not released** (`[Unreleased]` of the CHANGELOG; migration `00017`; `changelog.categories_fr`, `readme.sections`, `readme.min_words`, `docs.dir`, `docs.media_dir`, `docs.archives_scope` settings; deviations: DOC6 and DOC7 postponed to 8.7, CL10 to 8.4; `FileRulesVersion` separate from `RulesVersion`; `config_hash` also covers the names of the version tags (CL8); category order (CL6) as a warning; date gap between tag and section (CL8) not measured; TL1 without introduction date; DOC4 reads at most 300 documents; path lists only on a repository's page and in `watchoo conventions owner/repo`, not in `/conventions`; in API mode, milestone tags read from the database, see [section 9](#9-technical-impact)) | 0 quota, 1 migration | 1.9.0 |
| 8.4 | History: `tag_observations` (SV3, SV2 deadlines), `branch_rewrites` (BR4), SV4, SV5, SV7, CL10. **Done, not released** (`[Unreleased]` of the CHANGELOG; migrations `00018` and `00019`; setting `tags.prerelease_forms`; SV3 in the milestone review; approximations listed in [5.4](#54-tags-and-semver-sv): proof of a read of the tags by the dates of `tag_observations`, rewrite of an annotated tag on the same commit invisible, BR4 partial in API mode alone; SV4, SV5, SV7 and CL10 computed at read time, not stored) | 0 quota, 2 migrations | 1.10.0 |
| 8.5 | API with no extra request: `merge_commit_sha`, `from_fork`, `body_sections`, merge methods; full PR3, PR5, PR6, BR2, BR5 | one reread of the PR lists | 1.11.0 |
| 8.6 | Reviews: PR4, "merged without review", reviews received | 1 request per merged PR | 1.12.0 |
| 8.7 | Optional leads: CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4, TL5 | 0 quota | MINOR |

## 11. Maintainer decisions

Decisions of November 8, 2026.

| # | Question | Decision |
| --- | --- | --- |
| 1 | Reading README, CHANGELOG, `documentation/` and the tooling, excluded by the [vision](01-vision.md) | **Yes**, in memory, without persisting the content; exception carried over into the vision |
| 2 | Output and visibility | `n/m` rates and criteria, **no overall grade**; results **reserved to the trainer**, like the diagnosis |
| 3 | Scope of the "100% compliant commits" | **All the tracked branches**; the default branch alone as information |
| 4 | `conventions.since` | Workspace date, **can be overridden per repository** |
| 5 | Storage of the reviews | **Counters only** (no `pull_request_reviews` table); reviews received only |
| 6 | Sharing the charter | **Yes** through `configshare`, except `enabled` and `excluded_accounts` |
| 7 | Default milestones | **`v1.0.0`, `v2.0.0`, `v3.0.0`** |
| 8 | GitHub release per milestone (SV6) | **Information only** |
| 9 | "Another member" reviewer | **Another account is enough**; `pr.reviewer_must_contribute` as an option |
| 10 | Excluded accounts | **Specific to the conventions**, separate from `diagnostic.exclude` |
| 11 | Rebase merge (PR3) | **Heuristic with no request**; "undetermined" when in doubt |
| 12 | `--strict` option | **Not for now** |
| 13 | Commit language | **Free** (`commit.language: any`); mix flagged as information (CC7) |
| 14 | Default tolerances (warning rather than issue) | `[v1.2.0]` in the CHANGELOG; categories and H1 in French; free-form pre-releases and `+build`; `Media/` with its capital letter |
| 15 | Archives | Forbidden **in the whole repository**; `.docx` and `.pdf` files under `documentation/` flagged as information |
| 16 | Branch protection (BR3) | **Postponed**: not implemented in phase 8 |

### 11.1 Points still to check

- **Thresholds** to validate on real repositories before display: 80% for SV5, 150 words for RD4, CL8 date
  gap, CL12 and DOC9 thresholds.
- **Before phase 8.6**: does a review update the PR's `updated_at` (rereading the open PRs)?
- **If BR3 is taken up again**: does `protected` reflect rulesets? Which GitHub plan are the student
  repositories on (protection of a private repository does not exist on GitHub Free)?

## 12. References

- Conventional Commits 1.0.0: <https://www.conventionalcommits.org/en/v1.0.0/>
- Semantic Versioning 2.0.0: <https://semver.org/>
- Keep a Changelog 1.1.0: <https://keepachangelog.com/en/1.1.0/>
- commitlint and `@commitlint/config-conventional`: <https://commitlint.js.org/>
- GitHub API, pull request reviews: <https://docs.github.com/en/rest/pulls/reviews>
- GitHub API, branches (`protected`): <https://docs.github.com/en/rest/branches/branches>
