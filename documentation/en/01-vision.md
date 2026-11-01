# 1. Vision and goals

[← Contents](README.md)

## The problem

When you track several GitHub repositories (personal projects, team repositories, dependencies,
organizations), there is no simple, local view to tell at a glance:

- which repositories have received new commits;
- when the last activity happened;
- who contributes, and how often;
- which repositories have been dormant for too long.

## The answer: Watchoo

A **single Go binary** that watches a list of GitHub repositories, stores their history in a local
SQLite database, and reports it:

- through a **command line** (readable tables, scriptable JSON output);
- in an **interactive mode** in the terminal (`watchoo tui`): repositories, commits and contributors
  from the keyboard;
- through a **local web interface** served by the same binary (`watchoo serve`).

## Goals

| Goal | Description | Version |
| --- | --- | --- |
| **Monitor** | Detect new commits on all tracked branches of each repository | MVP |
| **Attribute** | Identify the contributors (author, committer, GitHub account) and their activity | MVP |
| **Visualize** | Dashboard: state of each repository, latest commits, evolution over time | MVP / V2 |
| **Alert** | Notify when a repository is updated or stays inactive for too long | V3 |

## Scope

- **Initial**: GitHub only. **Tracked repositories are mainly private**; public repositories are also
  supported. The token must therefore grant read access to the relevant private repositories, and
  local data is treated as confidential (see
  [Architecture](03-architecture.md#private-repositories)).
- **Planned from the start**: later addition of a local Git repository (V2), then GitLab or Gitea (V3),
  thanks to a
  `Provider` interface. No GitHub-specific code may leak outside the GitHub provider.

## Workspaces

Watchoo monitors separate **groups** of repositories. Each group lives in its own directory,
initialized with `watchoo init`, which holds its configuration and its SQLite database. Example: a
`licence-projet` directory for the projects of undergraduate students, another for those of master's
students. Data is never mixed. See [Workspaces](10-workspaces.md).

## Out of scope (for now)

- Multi-user tool or SaaS: Watchoo is a **personal and local** tool.
- Code content analysis (quality, security, detailed diff).
- Writing to GitHub: Watchoo is **read-only**.

## Target users

- **Instructor who monitors their students' projects**: one workspace per class of students or per
  module (e.g. "licence-projet"), to see who works, when, and who is falling behind.
- A developer or maintainer who tracks several projects.
- A technical lead who wants an activity view of a team or an organization.
- Anyone wanting to detect abandoned or inactive repositories.
