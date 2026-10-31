# 10. Workspaces

[← Contents](README.md)

A **workspace** is a directory that gathers everything related to **a group of tracked repositories**:
its configuration, its SQLite database, and therefore all of its data. You can have as many as you
want, fully independent from one another. It is the same principle as `git init`, which turns a
directory into a Git repository.

## Why `watchoo init`

Watchoo exists to monitor **distinct groups** that you do not want to mix. Example: an instructor
tracking the projects of several classes of students.

```
~/supervision/
  licence-projet/        ← watchoo init : les projets des étudiants de licence
    .watchoo/
  master-projet/         ← watchoo init : les projets des étudiants de master
    .watchoo/
  perso/                 ← watchoo init : mes propres dépôts
    .watchoo/
```

Each workspace has its own repositories, contributors, history, and settings. Deleting the directory
deletes all the data of that group, and only those.

## Scenario: monitoring the projects of undergraduate students

```bash
mkdir licence-projet
cd licence-projet
watchoo init                          # crée l'espace : .watchoo/ avec config et base SQLite
watchoo repo import promo-licence.csv  # ajoute d'un coup les dépôts de toute la promotion
watchoo sync                          # première récupération des commits
watchoo repo list                     # qui travaille, qui est en Flemme, qui est Dead
watchoo contributors --since 7d       # activité de chaque étudiant sur la semaine
watchoo serve                         # tableau de bord web de la promotion
```

All commands run inside `licence-projet/` (or one of its subdirectories) work on **that workspace
only**.

The `promo-licence.csv` file lists one repository per student or per group, with the display label
(detailed format in [CLI](05-cli.md#bulk-import-watchoo-repo-import)):

```csv
repo,label
alice/projet-licence,Alice Martin
bob/projet-licence,Bob Durand
```

The import summary flags repositories that are still inaccessible (students who have not granted
access). You rerun the same import later, without creating duplicates.

## Inside a workspace

```
licence-projet/
  .watchoo/
    config.yaml      réglages de l'espace (nom, intervalle, seuils, branches exclues…)
    watchoo.db       base SQLite : dépôts, commits, contributeurs, journal des synchros
    .gitignore       contient « * » : le contenu n'est jamais commité par erreur
  repos/             clones Git des dépôts suivis, avec ou sans gh (V2), créé au premier clone
    projet-licence/      alice/projet-licence, avec sa copie de travail
    bob-projet-licence/  bob/projet-licence : homonyme ajouté après
```

- The `.watchoo/` directory is created with **`0700`** permissions, and its files with **`0600`**:
  only the user can read them. The data comes from private repositories and contains students' names
  and e-mail addresses.
- The internal `.gitignore` prevents pushing the database to GitHub if the workspace lives inside a
  Git repository. It only covers `.watchoo/`: if the workspace root is a Git repository, **`repos/`
  shows up in it**, to add to your own `.gitignore` (Watchoo writes nothing outside `.watchoo/` and
  `repos/`).
- `repos/` (V2) sits at the **workspace root**, next to `.watchoo/`, created in **`0700`** by Watchoo
  at the first clone. Since 0.5.0, **every workspace has its own `repos/`**, with or without `gh`:
  every tracked repository is cloned there, to read the line statistics of all its commits (with
  `gh`) or its whole history (without `gh`), see [Local Git repositories](03-architecture.md#local-git-repositories-v2). It takes up
  roughly the size of the tracked repositories, and a repository's first sync is slower (full
  clone). The clones' content keeps the permissions set by Git. Each repository has its clone **with
  working copy** there, `repos/<name>` in lower case; two repositories with the same name (two
  students): the first one added keeps `repos/<name>`, the following ones `repos/<owner>-<name>`
  (full rule in the architecture doc).
- **These clones belong to Watchoo**: you can read the students' code there, but at every sync the
  working copy is reset to the repository's default branch (`git checkout --force` then
  `git reset --hard`): **any local change to a tracked file is overwritten**, another checked-out
  branch is left; untracked files are kept. `repo remove` deletes the clone,
  `repo remove --keep-history` keeps it; with or without `gh`, these are the same clones, kept up to
  date. A directory in `repos/` that Watchoo did not create for this repository is never modified
  nor deleted.
- `config.yaml` starts with the Watchoo header card, as YAML comments: the owl in small format,
  "Watchoo" and "Réglages de l'espace de supervision" ("Workspace settings"), in a frame (the
  workspace name does not appear there). The settings follow, commented in French; only the workspace
  name is active. The [`config.example.yaml`](../../config.example.yaml) file shows this content:

  ```yaml
  # ╭─────────────────────────────────────────────────╮
  # │  █▄    ▄█  Watchoo                              │
  # │  █ ●██● █  Réglages de l'espace de supervision  │
  # │  ▀██▙▟██▀                                       │
  # ╰─────────────────────────────────────────────────╯
  #
  # Réglages de l'espace de supervision Watchoo.
  ```
- **Workspace name**: the directory name by default (`licence-projet`), changeable in `config.yaml`.
  It is displayed in the web and TUI titles and in `watchoo status`.

## How Watchoo finds the workspace

Like Git, Watchoo looks for `.watchoo/` in the **current directory, then in its parents**. The first
one found is used.

| Case | Behavior |
| --- | --- |
| Command run inside the workspace or a subdirectory | Uses that workspace |
| Option `-C <directory>` (or `--workspace <directory>`) | Uses the workspace of that directory, wherever the command is run from |
| `WATCHOO_DIR` variable | Same effect as `-C`, handy for Docker and scripts |
| No workspace found | Clear error: "aucun espace Watchoo ici, lancez `watchoo init`" ("no Watchoo workspace here, run `watchoo init`") |

### Only the user's workspaces are accepted

An instructor often clones the repositories they supervise, sometimes **inside** their workspace
(`~/supervision/licence-projet/alice-projet/`); Watchoo does it itself in `repos/` (V2, with or
without `gh` since 0.5.0). A student may have committed a `.watchoo/` there (`git add -f` bypasses
the `.gitignore`): run from that clone, a command would find it before the
real workspace and read settings provided by the student (database, `web.*`, `github.gh_path`…).
Like Git's `safe.directory`, Watchoo therefore only trusts the user's workspaces. On Linux and macOS,
before reading a workspace's settings, it **refuses**:

- a `.watchoo/` not owned by the current user, or whose permissions exceed `0700` (readable or
  writable by group or others);
- a `.watchoo/config.yaml` in the same situation (different owner, permissions beyond `0600`).

`watchoo init` always sets these permissions; Git never does (a clone creates directories in `0755`
and files in `0644`), so a `.watchoo/` coming from a repository is refused. The message quotes the
path and the remedy: `chmod 700 …` (or `chmod 600 …`) if it is indeed your workspace, otherwise run
the command with `-C <your workspace>` (or `WATCHOO_DIR`). On Windows, this check is not performed.

### The `repos/` clones are never workspaces

Watchoo's clones (V2) have a working copy, **inside** the workspace: with a `077` umask, Git creates
directories in `0700` and files in `0600` there, so a `.watchoo/` committed by a student would pass
the permission check. The search therefore rules it out up front: a `.watchoo/` found **under a
workspace's `repos/` directory** (`licence-projet/repos/projet-groupe-3/.watchoo/`, at any
depth; `repos` name compared case-insensitively) is **ignored**, and the search restarts at
the workspace containing that `repos/`. Run from `repos/projet-groupe-3/src/`, a command
therefore works on `licence-projet`, as expected. Likewise, `watchoo init` refuses to create a
workspace under a workspace's `repos/`. This rule also applies to `-C` and `WATCHOO_DIR`.

In addition, `github.gh_path` must be a **name** looked up in the `PATH` (`gh`) or an **absolute
path**: a relative path with a separator (`./outils/gh`, `.watchoo/gh`) is refused, because it would
depend on the directory the command is run from.

## Behavior of `watchoo init`

1. Checks `gh` (`gh auth status`, see
   [Architecture](03-architecture.md#authentication-gh-optional)):
   - `gh` installed and logged in: the account and its scopes appear in the report; repositories
     will be tracked through the GitHub API;
   - `gh` **absent or logged in to no account** (V2): `init` **continues**, if `git` is installed, and
     warns (on the error output, and in `warnings` in JSON) that repositories will be tracked through
     **local Git clones** (`repos/` at the workspace root), with the machine's SSH key or Git
     credentials, without pull requests, releases or stars; it reminds you of `gh auth login` (and of installing `gh`
     if missing) to go through the GitHub API. The report says "source : clones Git
     locaux" (`source: "git"` in JSON). Without `git` either, `init` **stops** with a message
     saying to install `gh` or `git`, and creates nothing (see
     [Local Git repositories](03-architecture.md#local-git-repositories-v2));
   - `init` always stops, creating nothing, for what can be fixed beforehand: a token rejected by
     GitHub (account known to `gh`, revoked token: `gh auth login`, because `gh` would keep that token and
     syncs would go through the API), a `github.user` that `gh` does not know or cannot read, a
     `gh` that could not reach GitHub (network), did not answer or whose answer is unreadable.
2. Creates `.watchoo/` in the **current directory** (or the one given by `-C`), with `config.yaml`,
   the SQLite database with the right schema (goose migrations) and the `.gitignore`.
3. **Refuses** if the directory is already a workspace, so data is never overwritten.
4. **Warns** if a parent workspace already exists (e.g. `init` run in
   `~/supervision/licence-projet/groupe-a` while `licence-projet` is a workspace): you then create a
   nested workspace, independent from the parent.

On a terminal, the `init` report starts with the [header card](05-cli.md#header-card); redirected or
with `--json`, it stays plain text or JSON.

The token is **not** specific to a workspace: all workspaces use the user's `gh` account (or the one
defined by `github.user` in the workspace config). The source (GitHub API or local Git
clones) is not a workspace setting: it is chosen at every sync based on `gh` only.
Only the clones' protocol is configurable, per workspace or for the user: `git.protocol` (`ssh` by
default, `git@github.com:owner/repo.git`; or `https`, `https://github.com/owner/repo.git`, with
Git's credential helper).

## Settings: user and workspace

| Level | File | Content |
| --- | --- | --- |
| User (optional) | `$XDG_CONFIG_HOME/watchoo/config.yaml`, else `~/.config/watchoo/config.yaml` | Default values common to all workspaces (time zone, thresholds, interval…) |
| Workspace | `.watchoo/config.yaml` | Settings specific to the group; they override the user's |
| Command line / environment | options and `WATCHOO_*` variables | Override everything else, for one run |

### Sharing settings with other supervisors

`watchoo config export` and `watchoo config import` (CLI) or the Settings page (web) share the
tracked repositories and the **common settings** of a workspace: `sync.auto`, `sync.interval`,
`sync.parallelism`, `activity.flemme_after_days`, `activity.dead_after_days`, `branches.exclude` and
`timezone`. Settings specific to the machine or the person are **never** shared: `name`, `github.*`
(`gh` account, path to `gh`: a file coming from elsewhere must never choose the program being run),
`git.*` (clone protocol, tied to the machine's credentials) and `web.*`.
Details: [Configuration sharing](05-cli.md#configuration-sharing-watchoo-config).

When an import changes at least one common setting, `.watchoo/config.yaml` is **rewritten** from the
`watchoo init` template (same comments), with the imported settings uncommented:

- `name`, `github.*`, `git.*` and `web.*` already present in the file are kept as is, as well as the common
  settings the import does not provide;
- personal comments are lost; the old file is copied to `.watchoo/config.yaml.bak` (`0600`, replaced
  at each import);
- the new file is written alongside in `0600`, read back, validated and compared with the expected
  values before replacing the old one (atomic rename); in case of mismatch or error, `config.yaml` is
  not modified;
- values are written as escaped YAML values (quoted strings): a time zone or a pattern containing
  `"`, `#`, `:` or a line break can never create another key;
- if the imported settings are already the ones in effect, the file is not touched; a setting shadowed
  by a `WATCHOO_*` variable is compared with its value in the files, without the variable (the shared
  value is written even if the variable already has the same value, and a second import rewrites
  nothing);
- a defined `WATCHOO_*` variable keeps priority over the written value; the preview flags it for each
  imported setting it shadows, and refuses the import if, with the variables reapplied, the settings
  become invalid;
- the user file (`~/.config/watchoo/config.yaml`) is never modified.

## Several workspaces at the same time

- Each workspace is independent: you can run `watchoo serve` in `licence-projet` and in
  `master-projet` at the same time.
- Two `serve` instances cannot listen on the same port. The port is set per workspace (`web.addr` in
  `config.yaml`); if the port is already taken, `serve` exits with a message saying so.
- Web settings of the workspace (`web.*` keys): `web.addr` (listen address), `web.auth.username`
  (basic authentication; the password is never in a file: `WATCHOO_WEB_PASSWORD` variable),
  `web.trusted_proxy` (declared authenticating reverse proxy) and `web.allowed_hosts` (public names
  of the proxy accepted in the `Host` header). Two `serve` instances on the same machine each keep
  their own CSRF token (cookie named with the port).
- The GitHub quota is **shared** across all workspaces, since they use the same `gh` account. Each
  workspace displays the actual remaining quota (read from GitHub's responses). With or without `gh`, each
  workspace has its own clones (`repos/` at its root), which consume no quota.

## Students' personal data

A teaching workspace contains personal data (names, e-mail addresses, activity of each student).

- The data stays **on the instructor's machine**; Watchoo sends nothing anywhere except to the GitHub
  API (or, without `gh`, only reads the repositories with `git`).
- The local Git clones (V2) contain all the code and history of the tracked repositories: they live in
  the workspace (`0700`), are deleted by `repo remove` (kept with `--keep-history`).
- **Deleting** a workspace (the directory) deletes all of its data.
- `watchoo repo remove` also deletes the history of the removed repository and its local Git clone, after
  confirmation (`--keep-history` to keep them).
- _To be specified:_ retention period (e.g. end of the academic year) and informing students,
  according to the institution's rules (GDPR).
