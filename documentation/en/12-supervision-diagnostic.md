# 12. Supervision and diagnosis

[← Contents](README.md)

**Status**: maintainer decision. Phases **7.1 to 7.5 done** (foundations, declarative signals, the
`watchoo diagnostic` command, rhythm signals, history backfill, human review on the web, form signals F1 to
F5; unreleased, sections 9.1 to 9.5); phase 7.6 to come in a later release, then F6 (content classifier, postponed), each after
its plan is approved (see [Roadmap](07-roadmap.md)). Source of the signal part: the specification
"Detection of AI-assisted code production signals", **version 0.2** (November 3, 2026, references verified),
written in French.

## 1. Decision

Watchoo moves from an **activity tracking** tool to a **supervision and diagnosis** tool for repositories
and their contributors. It no longer only answers *what moved, when and who did it*: it flags what deserves
a human look and explains why.

The first diagnosis component is the detection of signals suggesting AI-assisted code production
(completion, chat, coding agents). For the scope of this document, it replaces the rule "code content
analysis is out of scope" in [the vision](01-vision.md#out-of-scope-for-now).

### Context of use (maintainer decision, November 4, 2026)

- **Educational context**: the people tracked are **learners**, in a learning environment. The employment
  context (Labour Code, works council) is not Watchoo's.
- **Purpose**: **support** learners, whether they work with or without AI. The diagnosis gives the trainer an
  overview and **reflection leads** on each project, to discuss with the learner ("explain this commit to
  me"). It is not used to grade, to sanction or to detect cheating.
- **Access to results**: reserved to the trainer who supervises the workspace. Learners have no access to
  Watchoo's signals or reflection leads (local tool, see [Workspaces](10-workspaces.md)). In this context, a
  "flag" is therefore a reflection lead for the trainer.

## 2. Purpose and guardrails (non-negotiable)

- **A flag is not a verdict.** It invites a look. No automatic decision: no grading, no sanction, no HR
  evaluation, no export to an evaluation system.
- **Explainability**: each flag shows the triggered signals, their raw values and the personal median.
  Never an opaque score alone.
- **Self-comparison**: rhythm signals compare a person to their own history.
- **Evidence hierarchy**: a declarative signal (trailer, signature, bot) outweighs several statistical ones.
- **Learning loop**: every human decision is recorded to measure signal precision and tune thresholds.
- **Transparency**: results stay reserved to the trainer, but learners are informed that their repositories
  are diagnosed, of its purpose (support) and of the criteria used (principle 1.2.5 of the specification;
  GDPR information duty, art. 13 and 14). Criteria are shown in the interface.
- **Data minimisation**: Watchoo already stores the commit message (`commits.message`, since V1); this document
  does not extend it. Signals persist only **recognised tool identifiers**, never the whole message. Logs never
  show messages, e-mail addresses or diff contents. Form signals are computed in memory and only their results
  are persisted. Decision to confirm: should `commits.message` eventually stop being kept?
- **Workspace boundary**: flags stay in their supervision workspace ([Workspaces](10-workspaces.md)) and are
  never mixed across workspaces.
- **Read-only and local**: unchanged. No write to GitHub, no data sent elsewhere.

**Out of scope**: plagiarism detection between people, analysis of external chat sessions (they leave no
trace in Git), and any automatic decision whatsoever.

## 3. Reference: production without AI

Rhythm signals assume a "normal" throughput without AI. Published orders of magnitude (Brooks 1975,
COCOMO, McConnell 2006, Jones, Cusumano et al. 2003, Prechelt 2000, Peng et al. 2023) serve as **context**,
never as thresholds. The operational threshold is always the person's own median over a sliding window.
Methodological points: denominator (coding is only part of the work), net lines (lines that survive),
skewed distributions (medians and robust statistics), language. Productivity figures often cited without a
primary source must be treated with caution (Bossavit 2014).

## 4. Data and preprocessing

| Data | Source |
| --- | --- |
| Additions, deletions, files per commit | `git log --numstat --no-merges -M` (API or local clone) |
| Author, dates | `%aI` (author date, not commit date, which is sensitive to rebases) |
| Trailers and signatures | full message, read in memory, reduced to the recognised tool |
| PR branch and description | API (pull requests already read in V2) |
| Diff contents (F signals) | read in memory, never persisted |

Preprocessing:

1. Identities: contributors already unified by Watchoo (`contributors`) are the reference.
2. Merge commits excluded.
3. Renames detected (`-M`).
4. Files excluded by configurable patterns: lock files (`package-lock.json`, `go.sum`…), vendored dependencies
   (`vendor/`, `node_modules/`), generated files (`*.pb.go`, `*.min.js`, `dist/`…), binary files.
5. Worked day: only days with at least one commit enter daily medians.

Known limits: squash merges (trailers removed), rebases (dates rewritten), people who commit rarely (large
commits are natural for them). Signal R2 must be read in light of the person's habits.

## 5. Signal catalogue

Each signal has an identifier, a definition, a default threshold and a reliability level.

### Family D: declarative (high reliability)

| ID | Signal | Default trigger |
| --- | --- | --- |
| D1 | AI co-author trailer (`Co-authored-by:`) | Match with the maintained list of tools |
| D2 | Signature in the message or PR ("Generated with…") | Match with the maintained list |
| D3 | Bot author (`*[bot]`, known accounts) | Known account |
| D4 | Agent configuration file (`AGENTS.md`, `CLAUDE.md`, `.cursorrules`, `.github/copilot-instructions.md`…) | Presence and addition date (context only) |
| D5 | Branch named by an agent (`codex/`, `copilot/`, `claude/`, `cursor/`…) | Known prefix |

Note: VS Code's `git.addAICoAuthor` setting (1.110, February 2026) adds a trailer automatically, with three
modes: `off`, `chatAndAgent` and `all` (default changed to `all` in April 2026). The trailer is added only for
commits made **from VS Code**. D1 therefore depends on the editor and its configuration. D4 shows that the tool
is used in the repository, not that a given commit is assisted.

### Family R: rhythm (medium reliability)

Computed per author, compared to their history over the **90 days** preceding the event.

Modified z-score: `z* = 0.6745 × (x − m) / MAD` (m: history median, MAD: median absolute deviation).
Default threshold: **z\* > 3.5** (Iglewicz & Hoaglin, 1993). If MAD = 0 or fewer than 20 observations, the
signal is marked "insufficient history" and does not trigger.

| ID | Signal | Default trigger |
| --- | --- | --- |
| R1 | Net lines per day | z\* > 3.5 |
| R2 | Commit size (lines added) | z\* > 3.5 **and** > 150 lines |
| R3 | Addition ratio (additions / total) | > 0.95 on a commit over 150 lines, if the personal median is < 0.80 |
| R4 | Apparent speed (lines / minutes since the same author's previous commit that day) | > 10 lines/min on a commit over 100 lines |
| R5 | Files per commit | z\* > 3.5 **and** ≥ 8 files |
| R6 | Regime break (R1 median over 14 days vs. previous 90 days) | Ratio > 3 |

R4 is the most fragile signal: a legitimate copy-paste can trigger it. It stays at "to review" until calibrated.

### Family F: form (low reliability)

Supporting hints, **never trigger a flag on their own**.

| ID | Signal | Default trigger |
| --- | --- | --- |
| F1 | Comment density (comment lines / lines added) | z\* > 3.5 |
| F2 | Systematic docstrings | > 90% while the personal median is < 30% |
| F3 | Lexical diversity of identifiers | z\* < −3.5 |
| F4 | Commit message style (length, bullets, conventional commits) | Abrupt change over the previous 20 messages |
| F5 | Tests delivered in the same commit as code | < 10% of the person's historical commits |
| F6 | Content classifier (optional, **disabled by default**) | Probability > 0.9 |

F6 has low weight: existing detectors generalise poorly (Suh et al., 2025). It is **postponed** to a later
step (section 9): it would need an external model, hence a dependency outside the documented stack. F1 to F5
are done in phase 7.5 (exact rules in section 9.5).

## 6. Levels, score and output

| Level | Default rule | Reading |
| --- | --- | --- |
| **Declared** | At least one signal D1, D2, D3 or D5 on the commit | Use attested by tool traces |
| **Probable** | ≥ 2 R signals **and** ≥ 1 F signal | Converging indicators |
| **To review** | 1 R signal, or ≥ 2 F signals | Isolated anomaly, often explainable |

D4 triggers no level on its own: it is shown as context. The weighted score (D = 10, R = 3, F = 1,
F6 = 0.5) is only used for sorting, and will be recalibrated from human reviews.

Proposed JSON output (`watchoo diagnostic --json`, name to be validated):

```json
{
  "commit": "a1b2c3d",
  "auteur": "e.martin",
  "date": "2026-10-14T16:42:00+02:00",
  "niveau": "probable",
  "score": 7,
  "signaux": [
    {"id": "R1", "valeur": 412, "mediane_perso": 35, "z_modifie": 6.1},
    {"id": "F2", "valeur": 0.94, "mediane_perso": 0.12}
  ],
  "contexte": {"agent_configure_depuis": null, "fichiers_exclus": ["package-lock.json"]},
  "revue": null
}
```

The JSON keys above are kept in French, as in the specification, until the schema is validated.

## 7. Human review

Review sheet (CLI and web interface): level, signals with raw values and personal median, diff of the commit
and its neighbours (± 3), the person's throughput chart over 90 days with the flagged point, context (period,
excluded files, agent configuration).

Recorded decision: `assisted` · `not assisted` · `undetermined`; cause if not assisted (`internal
copy-paste`, `code generated by a tool`, `existing code import`, `refactoring`, `other`); confidence (low,
medium, high); free note; reviewer and date.

Use: precision per signal, weight recalibration (logistic regression on `assisted` / `not assisted`
decisions, `undetermined` excluded), threshold tuning (precision / recall), inter-reviewer agreement (Cohen's
κ on a shared sample), feeding the file exclusion list.

## 8. Technical impact (to be reported when implementing)

- **Data**: new tables for per-commit statistics, flags and reviews. Tables and fields will be described in
  [Data model](04-data-model.md) before migration (goose, sqlc). Commit contents are not persisted.
- **Retention**: to be set per workspace, with purge. A default value to be proposed to the maintainer.
- **Provider**: diff reads go through the `Provider` interface (GitHub or local clone). They consume API quota
  and are handled like sync (ETags, rate limit, incremental).
- **Interface**: pages and commands in FR and EN; every text goes through `internal/i18n`.
- **Public surface**: new command, new `--json` keys and new settings. Adding them is MINOR, but any later
  incompatible change is MAJOR ([Skills](11-skills.md#versions-and-changelogmd)).

## 9. Phase plan (proposal)

Each phase follows the project rule: code, tests without external services (`gittest` repositories), `go vet`,
summary of what works. The plan and tree of each phase are proposed before the code.

- **7.1 Foundations** (done): detailed in section 9.1. Deliverable: `watchoo sync` fills the per-commit signal
  table, enabled per workspace, without changing existing displays or public outputs.
- **7.2 Declarative** (done): signals D1 to D5 (D2 for PR descriptions and D5 on top of 7.1), "Declared" level,
  `watchoo diagnostic` output. Deliverable: declarative flags visible and explained in the CLI.
- **7.3 Rhythm** (done): signals R1 to R6, 90-day window, modified z-score, "To review" level ("Probable"
  waits for phase 7.5), history backfill. Deliverable: statistical leads, insufficient history reported.
- **7.4 Review** (done): review sheet in CLI and on the web, decision recording, throughput chart. Deliverable: every
  flag can be labelled.
- **7.5 Form** (done): signals F1 to F5, "Probable" level. Deliverable: supporting hints, never alone.
  Detailed in section 9.5.
- **7.6 Calibration**: precision per signal, recalibration, κ. Deliverable: calibration report, documented
  thresholds. Starts once enough reviews exist; **later release** (maintainer decision, November 4, 2026):
  phases 7.1 to 7.5 ship without it, with the specification's thresholds.
- **Later step: F6, content classifier** (postponed by the maintainer on November 4, 2026): optional, off by
  default, low weight (0.5). It needs a detection model (GPTSniffer or DetectCodeGPT style, references 17 to
  19), hence a dependency outside the documented stack, subject to the maintainer's approval; it would run
  locally, without sending code anywhere. To be reconsidered after calibration (7.6), if the reviews show
  that F1 to F5 are not enough.

## 9.1 Phase 7.1: foundations (done)

**Goal**: collect, per commit, the data the signals need, with no display and no decision. Plan approved
by the maintainer on November 3, 2026: separate table, disabled by default, 365 days, `commits.message`
kept as is, D2 (PR description) deferred to 7.2.

**Starting point** (in the code): `commits` already stored `message`, `additions`, `deletions` and
`stats_known`; statistics come from the local clone (`CommitStatsBatch`, numstat in batches) or from the
API (`CommitStats`, as a fallback). The file detail, the lines after exclusion and the tool identifiers
were missing.

**What is delivered**:

```
internal/diagnostic/       business package, no logs, no dependency on cli, tui, web
  rules.go                 lists D1 (trailers), D2 (signatures), D3 (bots), D4 (agent files),
                           default exclusion patterns
  exclude.go               Excluder: "name/" patterns (directory), file name, full path
  compute.go               Compute: lines and files after exclusion, tools, bot, agent file
internal/provider/         CommitStats.Files (path, lines, binary), filled by the local clone only
internal/store/            migration 00007_commit_signals.sql, queries/diagnostic.sql, diagnostic.go
internal/sync/signals.go   computation after ApplyRepoSync, purge at the end of the sync
internal/workspace/        settings diagnostic.enabled, diagnostic.exclude, diagnostic.retention_days
```

**Rules**:
- **Disabled by default** (`diagnostic.enabled: false`): nothing is computed or purged.
- **New commits only**: commits synced after the feature is enabled, whose local Git clone gives the file
  detail. Earlier history is not measured; the API fallback without a clone gives no signal (the API
  response is read with `per_page=1` only, so as not to download the code of private repositories).
- **Merge commits excluded** (`--no-merges` in the specification, section 3.2).
- **Binary files always excluded**; default patterns: specification, section 3.2. Pattern syntax
  (`path.Match`): `name/` a directory anywhere (wildcards accepted, e.g. `*.egg-info/`), `a/b/` a
  subdirectory anywhere, `/name/` a directory at the root only, `pattern` a file name, `path/pattern` or
  `/file` a path from the root.
- **Renames detected** (`-M`, specification, section 3.2): a moved file counts only its changed lines (0 for a
  plain move), as in GitHub's statistics.
- **Claude D1 trailer**: recognised by the `noreply@anthropic.com` address only, never by the name (Claude is
  also a first name).
- **Measured signals**: D1 (`Co-authored-by:` trailers), D2 for the commit message part, D3 (bot author),
  D4 (the commit **changes** an agent configuration file; the addition date will come with the context
  display). D5 (PR branch name) and the PR description part of D2 come in 7.2.
- **Written once**: a row is never rewritten. Changing `diagnostic.exclude` only applies to later commits;
  a recompute command is deferred.
- **Purge** at the end of each sync: rows whose author date is older than `diagnostic.retention_days`.
- **Never shared**: `watchoo config export` / `import` ignore `diagnostic.*`, and the rewrite of
  `config.yaml` after an import keeps them.
- **Isolated failure**: a failure to write the signals becomes a warning of the repository's sync, without
  failing it.

**Tests**: see [Testing](09-testing.md#supervision-and-diagnosis-phase-7).

**Left for the display** (next phases): the `watchoo diagnostic` command, the levels, the human review. No
existing public output (`log`, `status`, `--json`) changed.

## 9.2 Phase 7.2: declarative signals and first output (done)

Plan approved by the maintainer on November 4, 2026: only detected tools are kept (never the text of a PR
description), `--json` keys in English like the rest of Watchoo, CLI output only (web and TUI in 7.4, with
the review sheet).

- **Signal made explicit**: each recognised tool is kept with the signal that found it (`D1:claude` for a
  trailer, `D2:unnamed` for a signature), so that each lead says where it comes from.
- **D2 on PRs**: a PR description is read in memory by the GitHub provider; only tool identifiers are kept
  (`pull_requests.tool_ids`, migration `00012`). For PRs read after the update, with the diagnosis enabled;
  local clones have no PRs.
- **D5**: prefix of a PR's branch (`codex/`, `copilot/`, `claude/`, `cursor/`), read from
  `pull_requests.head_branch`, already stored.
- **D4 as context**: date an agent configuration file was first added to the repository (`git log
  --diff-filter=A` on the clone), kept in `repositories.agent_config_since` (migration `00012`) and shown
  as context, without a level.
- **"Declared" level** (`declared`): at least one D1, D2, D3 or D5 signal; computed by `internal/diagnostic`,
  with the list of signals and their values.
- **`watchoo diagnostic [--repo X] [--author Y] [--since 30d]` command** (text and `--json`), for the
  trainer: reflection leads from newest to oldest (commits and PRs), signals and values, D4 context per
  repository, reminder "a reflection lead, not a verdict". Diagnosis disabled: the command says so, without
  an error.
- **Purge**: the tool identifiers of PRs also follow `diagnostic.retention_days` (opening date).

## 9.3 Phase 7.3: rhythm signals and history backfill (done)

Plan approved by the maintainer on November 4, 2026: thresholds of the specification without calibration until
phase 7.6; a command to backfill the history already cloned (the web button comes with the diagnosis web
pages, in 7.4); R4 disabled by default.

- **Signals R1 to R6** (section 5), computed when read by `internal/diagnostic` from `commit_signals` (no new
  table): modified z-score, 3.5 threshold, 90-day window. The history is the author's **in the repository**
  (one learner project per repository); a day is a day in the workspace's display time zone.
- **Insufficient history**: fewer than 20 observations, or zero MAD: the signal does not trigger, and the
  output says how many authors are in that case.
- **R6** compares the R1 median of the last 14 days (at least 3 worked days) with that of the previous 90
  days (at least 20 worked days, positive median); only the first day of a break is reported.
- **R4** (apparent speed) is computed only with the `diagnostic.speed_signal: true` setting (default
  `false`); a commit at the same minute as the previous one is not measured.
- **Levels**: "Declared" keeps priority; otherwise one or more R signals give "To review" (`to_review`).
  "Probable" requires a form signal (phase 7.5).
- **Leads**: per commit (R2 to R5) or per day (R1, R6: `kind` `day`); each signal gives its raw value, the
  personal median and the z-score.
- **History backfill**: `watchoo diagnostic backfill [--repo X]` measures, on the local clones, the commits
  already stored without signals and more recent than the retention period, merges excluded. An existing
  clone is read as is, without `git fetch` (the backfill can run during a `watchoo serve` sync); a missing
  clone is created first.
- **Author**: the commit's, read again at each computation (a merge of contributors keeps the learner's
  history).
- **Retention**: no lead is shown beyond `diagnostic.retention_days`, PR leads included (their branch, D5,
  stays stored with the PR). The `--since` filter keeps a day that straddles the boundary.

## 9.4 Phase 7.4: human review on the web (done)

Plan approved by the maintainer on November 4, 2026: reviews in a `diagnostic_reviews` table that follows the
signals' retention period; review on the web only (the CLI stays read-only); a single reviewer (single-user
tool): the web interface account if any, otherwise the workspace name.

- **"Diagnosis" page** (`/diagnostic`, in the navigation when the diagnosis is enabled): reminder "reflection
  leads, not verdicts", leads from newest to oldest with their level, signals and review status, filter by
  repository, insufficient histories, D4 context, and a **history backfill** button (run in the
  background, one at a time).
- **Review sheet** (`/diagnostic/lead?key=…`): level, signals with raw value, personal median and z-score;
  for a commit, its **diff read from the local clone** (never stored or logged, truncated beyond 200 KB) and
  the author's neighbouring measured commits (± 3); the author's **throughput chart** over 90 days (lines
  added per day) with the flagged day; D4 context; link to the forge.
- **Decision** (section 6.2): `assisted`, `not_assisted` or `undetermined`; cause if not assisted
  (`internal_copy`, `tool_generated`, `existing_import`, `refactoring`, `other`); confidence (`low`, `medium`,
  `high`); free note (2,000 characters at most); reviewer and date filled automatically. A lead has at most
  one review, replaced if done again; the signals present at review time are kept (calibration in phase 7.6).
- **Purge**: a review follows the retention period, from the date of its lead.

## 9.5 Phase 7.5: form signals (done)

Plan approved by the maintainer on November 4, 2026: only the **measures** are kept, never the code (to be
revisited only if calibration shows the need); F6 postponed (section 9).

- **Measurement**: at sync (and through the history backfill), the **added lines** of each measured commit
  are streamed from the local clone (`git diff-tree -p -U0 -M`, first parent, binary files and
  `diagnostic.exclude` patterns left out) and analysed **in memory** (`internal/diagnostic`, `FormMeter`),
  without parsing: line-by-line recognition of comments, function definitions and identifiers, for Go,
  JavaScript/TypeScript (Vue, Svelte), PHP, Java, Kotlin, C#, Scala, Swift, Rust, Dart, C/C++, Python, Ruby,
  shell scripts, SQL, HTML/Twig/XML and CSS. A line over 1,000 characters (minified code) is ignored. The
  commit message is measured in memory too.
- **Kept** (11 columns of `commit_signals`, migration `00014`): numbers only — source lines, comment lines,
  definitions and documented definitions, identifiers and their lexical diversity, test lines, message
  length, list lines, Conventional Commits subject. No line of code, identifier, path or message.
  `form_known` tells a measured form from a row written before phase 7.5 or a failed read; the history
  backfill completes those rows.
- **Signals** (computed when read, like R1 to R6; the author's history in the repository, 90 days, at least
  20 observations, commits with a measured form only):

| ID | Measure | Trigger |
| --- | --- | --- |
| F1 | Comment lines / source lines added (commits of at least 20 source lines) | z\* > 3.5 |
| F2 | Documented definitions / definitions (comment just before, docstring just after in Python) | ≥ 3 definitions, share > 90%, personal median < 30% |
| F3 | Lexical diversity of the identifiers (keywords and strings left out): MATTR, window of 50 identifiers (Covington and McFall, 2010) | z\* < −3.5 |
| F4 | Message length (trailers and signatures left out) against the 20 previous messages | z\* > 3.5 **and** a novelty absent from the 20 messages: a list of at least 2 lines, or a Conventional Commits subject |
| F5 | Test lines delivered with at least 20 lines of code (files `*_test.go`, `test_*.py`, `*.spec.ts`, `*Test.java`, directories `tests/`, `__tests__/`…) | ≥ 5 test lines, while fewer than 10% of the code commits of the history have some |

- **Implementation choices** (the specification leaves them open): MATTR rather than the plain ratio
  "distinct identifiers / identifiers", which mechanically drops with the size of the commit; F4 requires an
  unusual length **and** a novelty of form, so that a learner who adopts Conventional Commits after a lesson
  is not flagged for that change alone; F5 compares with a share (not a median); a zero MAD gives no score,
  as for R1 to R6.
- **Levels** (section 6): "Probable" (`probable`) for at least 2 R signals and 1 F signal; "To review" for 1
  R signal or at least 2 F signals; **an F signal alone never raises a lead**. F signals are about a commit:
  they combine with R2 to R5 of the same commit, not with R1 and R6 (day leads). "Declared" keeps priority;
  the F signals present are then shown with the others.
- **Display**: each F signal gives its raw value, the personal median (F5: the usual share of commits with
  tests) and, for F1, F3 and F4, the z-score; labels in English and French.

## 10. Open questions (for the maintainer to decide)

1. **Informing learners**: text and timing (before the first analysis: course introduction, course charter…).
   What is communicated: that the diagnosis exists, its purpose, its criteria; never individual results,
   which are reserved to the trainer.
2. **Default retention period**, and purge.
3. **Context of use**: **settled**, educational (section 1). GDPR requires validation by the institution's DPO
   (record of processing, retention period). For reference, a company use would fall under other rules (French
   Labour Code, art. L1222-4, L2312-37 and L2312-38; Cass. soc., 11 Dec. 2019, no. 18-11.792) and is not planned.
4. **Presenting it to learners**: **settled**, a support and dialogue tool ("explain this commit to me"), not a
   cheating detector.
5. **References**: those of specification v0.2, section 8, all verified on November 3, 2026. Secondary sources are
   flagged there; McConnell (2006) still has to be checked in the book.
6. **Code content**: phase 7.5 keeps measures only. Keeping excerpts (to review F1 to F3 without a clone) is
   not planned; to be reconsidered only if calibration (7.6) calls for it.

## 11. References

References of specification v0.2 (section 8), verified on November 3, 2026. The numbers are used as cross-references
from the other sections. Secondary sources are flagged.

### 11.1 Algorithms and methods

- **Modified z-score and 3.5 threshold** (R signals, section 5): Iglewicz, B., Hoaglin, D. C. (1993). *How to Detect
  and Handle Outliers*. ASQC Quality Press (vol. 16). NIST/SEMATECH, *e-Handbook of Statistical Methods*, 1.3.5.17.
  https://itl.nist.gov/div898/handbook/eda/section3/eda35h.htm
- **Logistic regression and precision / recall curves** (section 6.3): standard methods, no specific reference in
  the specification.
- **Cohen's κ** (inter-reviewer agreement, section 6.3): standard method, no specific reference in the specification.
- **MATTR, moving-average type-token ratio** (F3, section 9.5): Covington, M. A., McFall, J. D. (2010). "Cutting the
  Gordian Knot: The Moving-Average Type–Token Ratio (MATTR)". *Journal of Quantitative Linguistics*, 17(2), 94-100.
  DOI: 10.1080/09296171003643098. *(Added in phase 7.5: lexical diversity independent of length.)*
- **Conventional Commits 1.0.0** (F4): https://www.conventionalcommits.org/en/v1.0.0/; **Git trailers** (removed from the
  message length): `git interpret-trailers` documentation, https://git-scm.com/docs/git-interpret-trailers

### 11.2 Developer productivity (without AI): section 3

1. Brooks, F. P. (1975; 1995 ed.). *The Mythical Man-Month*. Addison-Wesley. ISBN 0-201-00650-2 and 0-201-83595-9.
2. Jones, C. (2008). *Applied Software Measurement* (3rd ed.). McGraw-Hill.
3. Jones, C., Bonsignour, O. (2011). *The Economics of Software Quality*. Addison-Wesley.
4. Jones, C. *Software Estimating Rules of Thumb* (1997-2007). Software Productivity Research.
   https://www.scribd.com/document/482442740/Capers-Rules-pdf
5. McConnell, S. (2006). *Software Estimation: Demystifying the Black Art*. Microsoft Press. *(To be checked in the book.)*
6. Cusumano, M., MacCormack, A., Kemerer, C. F., Crandall, B. (2003). "Software Development Worldwide: The State of
   the Practice". *IEEE Software*, 20(6), 28-34. DOI: 10.1109/MS.2003.1241363
   https://dblp.org/rec/journals/software/CusumanoMKC03
7. MacCormack, A., Kemerer, C. F., Cusumano, M., Crandall, B. (2003). "Trade-offs between Productivity and Quality in
   Selecting Software Development Practices". *IEEE Software*, 20(5), 78-85. DOI: 10.1109/MS.2003.1231158
   (Source of the median of 17.6 lines/day, 29 HP projects.)
8. Skeptics Stack Exchange, "Do professional software developers write an average of 10 lines of code per day?"
   (secondary source). https://skeptics.stackexchange.com/questions/17224/
9. Prechelt, L. (2000). "An Empirical Comparison of Seven Programming Languages". *IEEE Computer*, 33(10), 23-29.
   DOI: 10.1109/2.876288
10. Prechelt, L. (2000). *An empirical comparison of C, C++, Java, Perl, Python, Rexx, and Tcl for a search/string-processing
    program*. University of Karlsruhe. https://publikationen.bibliothek.kit.edu/372000
11. QSM, *Performance Benchmark Tables*. https://qsm.com/resources/qsm-benchmark-tables
12. Beller, M., Orgovan, V., Buja, S., Zimmermann, T. (2020). "Mind the Gap: On the Relationship Between Automatically
    Measured and Self-Reported Productivity". arXiv:2012.07428.
13. Forsgren, N., Storey, M.-A., Maddila, C., Zimmermann, T., Houck, B., Butler, J. (2021). "The SPACE of Developer
    Productivity". *ACM Queue*, 19(1), 20-48. DOI: 10.1145/3453928. https://queue.acm.org/detail.cfm?id=3454124
14. Sadowski, C., Zimmermann, T. (eds.) (2019). *Rethinking Productivity in Software Engineering*. Apress.
    DOI: 10.1007/978-1-4842-4221-6
15. Bossavit, L. (2012; print 2013). *The Leprechauns of Software Engineering*. Leanpub. https://leanpub.com/leprechauns

### 11.3 Effect of AI on productivity

16. Peng, S., Kalliamvakou, E., Cihon, P., Demirer, M. (2023). "The Impact of AI on Developer Productivity: Evidence
    from GitHub Copilot". arXiv:2302.06590.

### 11.4 Detection of AI-generated code (content): F signals

17. Suh, H., Tafreshipour, M., Li, J., Bhattiprolu, A., Ahmed, I. (2025). "An Empirical Study on Automatically Detecting
    AI-Generated Source Code: How Far Are We?". *ICSE 2025*, 859-871. DOI: 10.1109/ICSE55347.2025.00064.
    https://arxiv.org/abs/2411.04299 *(Basis for the low weight of F6.)*
18. Shi, Y., Zhang, H., Wan, C., Gu, X. (2025). "Between Lines of Code: Unraveling the Distinct Patterns of Machine and
    Human Programmers" (DetectCodeGPT). *ICSE 2025*. https://arxiv.org/abs/2401.06461
    Code: https://github.com/YerbaPage/DetectCodeGPT *(Basis for F3.)*
19. Nguyen, P. T., Di Rocco, J., Di Sipio, C., Rubei, R., Di Ruscio, D., Di Penta, M. (2024). "GPTSniffer: A
    CodeBERT-based classifier to detect source code written by ChatGPT". *Journal of Systems and Software*, 112059.
    DOI: 10.1016/j.jss.2024.112059. Preprint: arXiv:2307.09381.

### 11.5 Watermarking of generated code

20. Lee, T., Hong, S., Ahn, J., Hong, I., Lee, H., Yun, S., Shin, J., Kim, G. (2024). "Who Wrote this Code? Watermarking
    for Code Generation" (SWEET). *ACL 2024*, 4890-4911. DOI: 10.18653/v1/2024.acl-long.268.
    https://arxiv.org/abs/2305.15060
21. Kirchenbauer, J., Geiping, J., Wen, Y., Katz, J., Miers, I., Goldstein, T. (2023). "A Watermark for Large Language
    Models". *ICML 2023*, PMLR 202, 17061-17084. https://arxiv.org/abs/2301.10226
    *(Out of scope for 7.1: listed for reference, no signal relies on watermarking.)*

### 11.6 Agent traces in Git repositories: D and R signals

22. Robbes, R., Matricon, T., Degueule, T., Hora, A., Zacchiroli, S. (2026). "Agentic Much? Adoption of Coding Agents on
    GitHub". arXiv:2601.18341; to appear in *ACM TOSEM*. *(Heuristics based on files, trailers and authors; size of
    assisted commits: basis for D1, D4 and R2.)*
23. Ghaleb, T. A. (2026). "Fingerprinting AI Coding Agents on GitHub". *MSR 2026*. arXiv:2601.17406. *(Per-agent
    signatures: basis for D1 and F4.)*
24. Ghaleb, T. A. (2026). "AgenTag: Attribution of AI Coding Agents from Behavioral Fingerprints". arXiv:2608.00966.
25. Li, H., Zhang, H., Hassan, A. E. (2026). "AIDev: Studying AI Coding Agents on GitHub". *(Dataset; arXiv identifier
    to be taken from the paper's page.)*
26. Stein, M. (2026). "How are AI agents used? Evidence from 177,000 MCP tools". arXiv:2603.23802. *(Appendix A.3:
    limits of detection through Git metadata; basis for false negatives, section 7.2.)*
27. coderbuds/ai-detector. https://github.com/coderbuds/ai-detector *(Tool: detection rules in PRs and commits.)*
28. satinath-nit/ai-usage-measurement-framework. https://github.com/satinath-nit/ai-usage-measurement-framework
29. VS Code `git.addAICoAuthor` setting: VS Code 1.110 release notes (February 2026); PR microsoft/vscode#310226
    (April 16, 2026). Secondary summary: https://www.penligent.ai/hackinglabs/fr/vs-code-copilot-co-author-when-ai-attribution-becomes-a-supply-chain-trust-problem/
30. Vaughan, D. (2026). "The Invisible Agent Problem". *(Blog, secondary source.)*
    https://codex.danielvaughan.com/2026/06/27/detecting-ai-coding-agents-open-source-180-million-repo-census-codex-cli-attribution-supply-chain/

### 11.7 Legal texts (France)

31. French Labour Code, art. L1222-4 (prior information of employees).
32. French Labour Code, art. L2312-37 and L2312-38 (works council consultation). https://code.travail.gouv.fr/code-du-travail/l2312-38
33. French Court of Cassation, social chamber, December 11, 2019, no. 18-11.792.
