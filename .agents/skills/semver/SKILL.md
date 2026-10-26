---
name: semver
description: Apply Semantic Versioning 2.0.0 (semver.org) to decide version bumps, validate version strings, and compare version precedence. Use whenever a task involves choosing the next version number, judging whether a change is a MAJOR/MINOR/PATCH bump, checking that a string is a well-formed version, ordering versions by precedence, or building a pre-release or build-metadata version. Often used as a building block by release, changelog, and publishing workflows — trigger whenever version numbers are being reasoned about, even if "semver" is never said.
---

# Semantic Versioning (SemVer 2.0.0)

Decision logic and reference for [SemVer 2.0.0](https://semver.org/). Often consumed by other workflows (the `keep-a-changelog` and `conventional-commits` skills) that just need the bump and the reason, so keep answers terse.

A version is a compatibility promise about a **declared public API**: MAJOR = breaks you, MINOR = new but compatible, PATCH = compatible fixes. Without a declared public API "breaking" is undefined — establish it or ask before deciding.

## Format

```text
MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]
```

- `MAJOR.MINOR.PATCH`: non-negative integers, no leading zeroes (`1.9.0`).
- Pre-release after `-` (`1.0.0-alpha.1`); build metadata after `+` (`1.0.0+20130313144700`).
- Pre-release/build identifiers: `[0-9A-Za-z-]`, dot-separated, non-empty, no leading zeroes on numeric pre-release identifiers.

## Deciding the bump

Don't pattern-match "new thing = MINOR." Reason in three steps:

1. **Name the public API.** A version is only meaningful against a declared surface consumers depend on — a library's exports, a CLI's flags, an HTTP contract, the set of installable skills in a repo. No declared surface → establish it or ask; without it "breaking" is undefined.
2. **Classify each change by its effect on that surface**, not by the size of the diff: does it _break_ existing use, _add_ compatible capability, or _fix_ behaviour inside the existing contract? A wholly new, opt-in unit — a new skill, a new subcommand — is additive, not breaking: nobody's existing use changes.
3. **Map to a slot — and mind the major-zero shift**, because what each number promises depends on whether you've cut `1.0.0`.

**At `1.0.0` and above** — the public API is a promise. First match wins:

1. Any backward-incompatible change → **MAJOR** (reset MINOR, PATCH to 0).
2. New backward-compatible functionality, or a deprecation → **MINOR** (reset PATCH to 0).
3. Only backward-compatible fixes → **PATCH**.

**Under `0.y.z`** — initial development, `1.0.0` uncut, so there's no promise; the numbers signal _relative_ risk instead. The scale slides down one slot, matching how `^` ranges resolve (`^0.1.0` means `>=0.1.0 <0.2.0`, so MINOR is the breaking position):

1. Backward-incompatible change → **MINOR** (`0.3.2` → `0.4.0`, reset PATCH to 0).
2. Anything compatible — feature _or_ fix → **PATCH** (`0.3.2` → `0.3.3`).

So adding a new skill to a `0.1.0` repo is a PATCH → `0.1.1`, not a MINOR. Always flag that a 0.x version carries no stability guarantee, and cut `1.0.0` once the API is stable and in production.

Mixed set: the highest-impact change decides the release — a single breaking change sets the level.

## Validating a version string

Official regex:

```text
^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$
```

Groups: 1=major, 2=minor, 3=patch, 4=pre-release, 5=build. Flag: leading zeroes (`01.2.3`); a leading `v` (`v1.2.3` — a tag prefix, strip before validating); fewer than three components (`1.2`); an empty pre-release/build identifier (`1.0.0-`).

## Comparing precedence

1. Compare MAJOR, then MINOR, then PATCH numerically; first difference decides.
2. Equal core: a version **with** a pre-release is lower than one without (`1.0.0-alpha` < `1.0.0`).
3. Both pre-release: compare identifiers left to right — both numeric → numerically; else ASCII; numeric < alphanumeric; if all shared are equal, more identifiers wins.
4. Build metadata never affects precedence.

Low to high: `1.0.0-alpha < 1.0.0-alpha.1 < 1.0.0-alpha.beta < 1.0.0-beta < 1.0.0-beta.2 < 1.0.0-beta.11 < 1.0.0-rc.1 < 1.0.0`

## Edge cases

- **Breaking change shipped as minor/patch:** don't mutate the published version — release a compatibility-restoring patch, then break properly in a MAJOR.
- **Dependency bump only:** PATCH if fixing a bug, MINOR if it enables new functionality you expose.
- **Deprecating:** bump MINOR, document it, keep it for at least one more minor release before removing it in a MAJOR.
- **Released versions are immutable** — corrections ship as a new version.
- **`0.0.z`:** the major-zero shift leaves no compatible slot (`^0.0.3` permits nothing) — treat every change as potentially breaking, bump PATCH, and keep the instability warning.

## Compact output for another workflow

```text
current: 0.3.2
bump: patch
next: 0.3.3
reason: added an opt-in --dry-run flag; no existing behaviour changes
warning: pre-1.0 (0.y.z) — no compatibility promise; minor is the breaking slot
```

Add the `warning:` line only if the input version was invalid or you're under `0.y.z`. For anything not covered here (BNF grammar, full FAQ), read [semver.org](https://semver.org/).
