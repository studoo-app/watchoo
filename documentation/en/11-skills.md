# 11. Coding agent skills

[← Contents](README.md)

The project is built with the help of coding agents (Claude Code, Mistral Vibe…). **Skills** give
them reusable know-how: a procedure, rules, references. Watchoo uses the open directory
**[skills.sh](https://www.skills.sh/)** and its `skills` CLI to install and manage them.

## Decision

- **Single source**: the project's skills come from [skills.sh](https://www.skills.sh/) (public
  GitHub repositories referenced by the directory). No skill copied by hand from another source.
- **Installed at the project level**, so that every contributor and every agent uses the same
  skills.
- **Committed** to the repository, with the `skills-lock.json` lock file.

## Install and update

```bash
npx skills add <owner>/<repo>
```

```bash
npx skills update
```

- The CLI runs with `npx`: **Node is needed for this development tooling only**. The "no Node"
  rule still applies to the build and the Watchoo binary, which never depend on it.
- The CLI sends anonymous telemetry by default (skill name and files). To turn it off:
  `DISABLE_TELEMETRY=1`.

## Layout in the repository

```
watchoo/
  skills-lock.json         source, path and checksum of every installed skill
  .agents/skills/          skill contents (single reference)
    goland/
    semver/
    semver-changelog/
  .claude/skills/          symbolic links to .agents/skills/ (Claude Code)
  .vibe/skills/            symbolic links to .agents/skills/ (Mistral Vibe)
```

- `.agents/skills/` holds the only copy of each skill; the per-agent directories only contain
  **symbolic links**.
- `skills-lock.json` records for each skill its source repository and a checksum (`computedHash`):
  a change to a skill's contents shows up on review.
- `.gitignore`: only `.claude/worktrees` is ignored, so that `.claude/skills/` is committed.

## Installed skills

| Skill | Source | Use in Watchoo |
| --- | --- | --- |
| `goland` | `g1joshi/agent-skills` | Go development: tools, delve debugger, profiling, refactoring, running tests |
| `semver` | `lelborn/skills` | Pick the next version number (MAJOR / MINOR / PATCH) per SemVer 2.0.0 |
| `semver-changelog` | `prulloac/agent-skills` | Create and maintain `CHANGELOG.md` in the "Keep a Changelog" format, from the commits since the last tag |

`semver` and `semver-changelog` are used from **phase 5** (GoReleaser, first published releases):
every release gets a SemVer number and a `CHANGELOG.md` entry.

## Versions and `CHANGELOG.md`

- **Public surface** the version number is decided on (skill `semver`): CLI commands, flags and
  exit codes, keys of the `--json` output ([CLI](05-cli.md#json-output)), `config.yaml` settings and
  `WATCHOO_*` variables, schema of the `.watchoo/watchoo.db` database (migrations), routes of the
  web interface.
- **First release: `v0.1.0`** (MVP, phases 1 to 5). While Watchoo is `0.y.z`, nothing is stable;
  per the `semver` skill, an incompatible change (a renamed or removed `--json` key, a dropped
  option…) bumps **MINOR** (`0.1.3` → `0.2.0`), and any compatible change, feature or fix, bumps
  **PATCH** (`0.1.0` → `0.1.1`). From `1.0.0` on, the usual rule applies (incompatible → MAJOR).
- **`CHANGELOG.md`** (root) follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/):
  Added, Changed, Deprecated, Removed, Fixed, Security sections; a `[Unreleased]` section kept as
  commits land; GitHub compare links at the bottom of the file. The `semver-changelog` skill starts
  from the last SemVer tag (`git tag -l --sort=-v:refname`) and the commits since that tag.
- **Release**: rename `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD`, add an empty `[Unreleased]` section
  and the links, commit, then push the `vX.Y.Z` tag; the release workflow
  ([Distribution](03-architecture.md#distribution)) does the rest, and the `CHANGELOG.md` section
  is copied into the draft GitHub release before it is published.

## Rules

- **Review before installing**: a skill is a set of instructions (and sometimes scripts) the agent
  will follow. Read its `SKILL.md` and its files before adding it, and check its page on
  skills.sh (security audits, popularity, source repository).
- **One skill = one project need**: only install what serves Watchoo.
- **Updates reviewed** like code: the skill and `skills-lock.json` diff goes through review in the
  commit.
- The project's instructions (documentation, agent prompt) take precedence over a skill's on any
  contradiction.
