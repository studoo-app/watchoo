# 13. Project conventions

[← Contents](README.md)

**Status**: **maintainer decision** (phase 8, November 8, 2026); the choices are recorded in
[section 11](#11-maintainer-decisions). Phase 8.1 done, not released; the next phases start after their
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
| Local clone, **through git objects** | `git rev-list` (first parent, ranges between tags), `git for-each-ref refs/remotes/origin/` (all the remote's branches), `git ls-tree` and `git cat-file blob` on `origin/<default>` or on a tag, `git merge-base --is-ancestor` | both (the clone exists with or without `gh`) |
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
| CC6 | Footers and issues | Share of commits with a valid footer and share of those citing an issue (`(Closes\|Fixes\|Resolves\|Refs) #\d+`); informative | repository, contributor | database | both | H | P3 |
| CC7 | Message language | FR / EN mix within one repository, if the charter imposes a language; informative | repository | database | both | M | P3 |

### 5.2 Branches (BR)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| BR1 | Compliant branch name | Remote branches (`refs/remotes/origin/` of the clone) and `pull_requests.head_branch` of PRs not coming from a fork, excluding the default branch and exclusions (`gh-pages`, `dependabot/*`, `renovate/*`, setting). Compliant if `^(<types>)/[a-z0-9]+(-[a-z0-9]+)+$`; a single segment after `/`: "to review". Pattern given: unknown type, uppercase, `_`, space, accent, missing `/` | branch, repository; contributor through the PR author | clone + database | both | H | P1 |
| BR2 | Reused branch | A `head_branch` (not from a fork) with more than one PR merged into the default branch | branch, repository | database | API | M | P2 |
| BR3 | Protected default branch | **Postponed** (maintainer decision: not implemented in phase 8). Planned measure: `protected` from `GET /branches`: yes, no, unknown (clone), not applicable (for example a private repository on GitHub Free). **Snapshot** without history | repository | API, 0 requests | API | M | — |
| BR4 | Rewritten default branch | At sync time, the old head is no longer an ancestor of the new one (`merge-base --is-ancestor`); dated event (old and new SHA). Old object missing from the clone: "unknown", never "rewritten". In API mode, the sync's `ErrUnknownBase` error is also used as a signal | event, repository | clone | both | H | P2 |
| BR5 | Allowed merge methods | `allow_squash_merge`, `allow_merge_commit`, `allow_rebase_merge` of the repository (returned to an account with push rights) | repository | API, 0 requests | API | M | P3 |

### 5.3 Pull requests (PR)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| PR1 | Conventional PR title | Shared parser on the title of non-draft, non-bot PRs; open and merged PRs counted separately | PR, repository, author | database | API | H | P1 |
| PR2 | Forbidden or default title | Normalised title in the forbidden list, or equal (after normalisation: case, `-` and `_` as spaces) to the head branch name or to the subject of the PR's only commit (titles suggested by GitHub) | PR | database | API | H | P1 |
| PR3 | Commits that reached the default branch outside a PR | Walk of `git rev-list --first-parent origin/<default>` since `since`, without the root commit. **Through a PR**: SHA equal to the `merge_commit_sha` of a merged PR; sequence rebased by GitHub, recognised by a heuristic with no request (consecutive commits whose committer is GitHub and whose `committed_at` equals the `merged_at` of the PR that ends it; maintainer decision); merge whose committer is GitHub. **Outside a PR**: human committer, local merge pushed, web edit without a PR. Otherwise: "undetermined". With a clone only, fallback on `^Merge pull request #\d+` and `\(#\d+\)$` with the GitHub committer recognised by its `noreply@github.com` e-mail (in memory) | commit; repository; "author of the commit that arrived outside a PR" (never "pushed") | clone + database | both (degraded with a clone) | H with the API, M with a clone | P1 |
| PR4 | Reviewed by another member | New Provider method `PullRequestReviews` (login, identifier, state, date; never the body). Reviewed if, for at least one reviewer with an identifier other than the author's and not excluded (trainer, bots), the last state among `APPROVED`, `CHANGES_REQUESTED` and `DISMISSED` submitted no later than `merged_at` is `APPROVED`. Another account is enough; the `pr.reviewer_must_contribute` setting also requires a reviewer who authored at least one commit of the repository. Only counters are stored, not the reviewers. Derived view: "merged without review" | PR, repository; reviews received by the PR author | API, 1 request per PR | API | H | P1 |
| PR5 | Structured body and `Closes #n` | Body read in memory (as for D2): section headings, or bold text, among versioned FR and EN synonyms (context, what changes, how to test, screenshots); an empty section (or one reduced to an HTML comment) does not count. Closing keyword searched separately. Only a bit mask is stored (NULL while unknown). Known false negative: issue linked through the "Development" sidebar | PR, repository | API, 0 requests | API | M | P2 |
| PR6 | Squash merge | Commit of `merge_commit_sha` with one parent and the "(#n)" suffix: squash; two parents: merge commit. Compliant if squash **and** message compliant with the parser | PR | database | API | M | P2 |
| PR7 | PR template | See TL3 | repository | clone | both | H | P2 |

### 5.4 Tags and SemVer (SV)

| id | Indicator | Rule | Granularity | Source | Mode | Rel. | Prio |
| --- | --- | --- | --- | --- | --- | --- | --- |
| SV1 | SemVer tag name | Lowercase `v` then a SemVer 2.0.0 version (comparator extracted from `internal/selfupdate` into `internal/semver`). Patterns: no `v`, `V`, missing component, leading zero, empty pre-release; `+build` as a warning | tag, repository | database | both | H | P1 |
| SV2 | Milestones present | For each configured milestone: present, present under a non-compliant name, absent; a pre-release does not count as a milestone. With a deadline: compared to the first observation of the tag; "undetermined" if no successful sync took place between the date of the target commit and the deadline (display "set between X and Y") | milestone × repository | database | both | H (presence), M (deadline) | P1 |
| SV3 | Tag rewritten or deleted | `tag_observations` table, fed only if reading the tags succeeded and was not truncated: same name with another SHA (rewritten), gone (deleted). Label: "no rewrite observed since …", never "never rewritten"; no attribution to a person | tag, repository | database | both | M | P1 |
| SV4 | Pre-releases | Form `-rc.N`, `-beta.N`, `-alpha.N`; free forms (`-rc1`, `-RC.1`) give a warning, not an issue (maintainer decision); also a warning if an rc is set after the final version or if precedence contradicts ancestry | tag | database + clone | both | M | P2 |
| SV5 | Bump consistent with the commits | For each final tag F, base B = highest compliant final tag lower than F that is its ancestor; commits of `B..F` excluding merges and bots. Breaking change: MAJOR expected (MINOR before 1.0.0); `feat`: at least MINOR. **Only under-versioning is flagged**; "undetermined" if less than 80% of the range's commits are compliant; first tag not evaluated | pair of tags | clone + database | both | M | P2 |
| SV6 | Associated release | Excluding drafts: milestone without a release, release without a tag, inconsistent `prerelease` flag. **Information only**: the charter only mentions tags, SV6 does not count towards compliance | tag, release | database | API | H | P2 |
| SV7 | Precedence and history | Higher version set on an ancestor of a lower version; increment skip (v1 → v3) as information | pair of tags | database + clone | both | M | P3 |

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
| DOC8 | Archives | `.zip`, `.rar`, `.7z`, `.tar(.gz)` **in the whole repository** (maintainer decision); `.docx` and `.pdf` files under `documentation/` are flagged as information | repository, tag | clone | both | H | P1 |
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

- **Per repository**: CC1, BR1, PR1, PR3, PR4, SV1, SV2, then the CHANGELOG (CL1 to CL9), README (RD1 to
  RD3), documentation (DOC1 to DOC5, DOC8) and tooling grids.
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

### 7.2 Interfaces

- **Web**: `/conventions` route (overview and milestone review), navigation entry only if
  `conventions.enabled`. "Conventions" section on the repository page (badges per PR, dot per tag,
  "Tooling" block); "Compliant commits 18/25 (72%)" line on the contributor's page; compact dot on the
  dashboard, without sorting by this value. Icons always come with a text.
- **CLI**: a single command, with no subcommand (a repository named like a subcommand would be ambiguous):
  `watchoo conventions [repo] [--commits] [--failing] [--contributor <x>] [--milestone vX.Y.Z] [--json]`.
  `--commits --failing` lists the short SHA, the author and the issue codes, never the message. No
  `--strict` option (exit code depending on compliance) for now: it can be added later in a MINOR.
- **JSON**: keys in English (`commits.rate`, `commits.issues[{code,count,shas}]`, `semver_checks[]`,
  `changelog.rules[]`…), frozen by `internal/cli/json_schema_test.go`. No key added to `prs` or
  `releases` at first.
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
| `commit.language` | `any` | `fr`, `en` or `any` |
| `branch.pattern`, `branch.exclude` | `<type>/<scope>-<subject>`, `gh-pages`, `dependabot/*`, `renovate/*` | Naming (RE2 validated at load time) |
| `pr.body_sections`, `pr.require_closes` | charter sections, `true` | PR body |
| `pr.min_reviews`, `pr.reviewer_must_contribute` | 1, `false` | Reviews; another account is enough, unless the reviewer must be a contributor |
| `tags.milestones` | `v1.0.0`, `v2.0.0`, `v3.0.0` (optional deadline) | Milestones |
| `tags.prerelease_forms` | `rc`, `beta`, `alpha` | Compliant pre-releases; the other forms give a warning |
| `changelog.categories_fr` | `true` | Allows French labels (categories, H1) |
| `readme.sections` | charter sections | Synonyms of the expected sections |
| `docs.dir`, `docs.media_dir`, `docs.archives_scope` | `documentation`, `Media`, `repository` | Tree; archives forbidden in the whole repository |
| `excluded_accounts` | empty | Excluded accounts (trainer, bots): author's login, name or e-mail, case ignored; specific to the conventions, separate from `diagnostic.exclude` |

The block is shared through [`configshare`](02-features.md) (maintainer decision), except `enabled`,
`excluded_accounts` (which contains logins): sharing a charter is the intended use. A repository's own date
is exported with it, like its activity thresholds. The keys are then frozen by SemVer.

**Phase 8.1**: only `enabled`, `since`, `commit.*` and `excluded_accounts` exist; the other keys come with
their phase. Environment variables: `WATCHOO_CONVENTIONS_ENABLED`, `WATCHOO_CONVENTIONS_SINCE`,
`WATCHOO_CONVENTIONS_COMMIT_TYPES`… (comma-separated lists). Rules checked at load time: `since` empty or of
the form `YYYY-MM-DD` (an unquoted YAML date, `since: 2026-09-01`, is accepted; a date with a time is
refused, except an unquoted YAML timestamp at exactly midnight UTC, `2026-09-01T00:00:00Z`, which the YAML
decoder cannot tell from the date alone and which is read as `2026-09-01`), read at midnight in the workspace time zone; at least one type, each in lowercase letters; each
scope of the form `[a-z0-9][a-z0-9._/-]*`; `commit.max_subject_soft` at least 1 and
`commit.max_subject_hard` strictly greater; no empty entry in `commit.forbidden` nor `excluded_accounts`;
`commit.language` among `fr`, `en`, `any` (no effect on the analysis in 8.1). In the sharing file, the
block is written `settings.conventions` (`since`, `commit`): see
[Configuration sharing](05-cli.md#configuration-sharing-watchoo-config).

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
| 8.2 | On the fly: BR1, PR1, PR2, SV1, SV2 (presence), SV6; first milestone review; CC5 to CC7; CC1 per range between tags, CC3 trend | 0 quota | 1.8.0 |
| 8.3 | Clone tree: `repository_conventions`, CL1 to CL9, RD1 to RD4, DOC1 to DOC8, TL1 to TL3; milestone review on the tags' tree | 0 quota, 1 migration | 1.9.0 |
| 8.4 | History: `tag_observations` (SV3, SV2 deadlines), `branch_rewrites` (BR4), SV4, SV5, SV7, CL10 | 0 quota, 2 migrations | 1.10.0 |
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
