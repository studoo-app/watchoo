# Glossary

[← Contents](README.md)

| Term | Definition |
| --- | --- |
| **Workspace** | Directory initialized by `watchoo init` (`.watchoo/` folder), with its config and database: one monitored group, for example a class of students |
| **Tracked repository** | GitHub repository registered in Watchoo and synced regularly |
| **Tracked branch** | Branch of a tracked repository whose commits are fetched |
| **Provider** | Interface to a forge (GitHub at the MVP; GitLab, Gitea, plain Git later) |
| **Forge** | Git repository hosting service (GitHub, GitLab, Gitea…) |
| **Incremental sync** | Fetching only the new commits since the last known SHA |
| **SHA** | Unique identifier of a Git commit |
| **ETag** | HTTP header enabling a conditional request; a 304 response consumes no GitHub quota |
| **Rate limit / quota** | Limit of requests to the GitHub API (5,000 / hour with a token) |
| **Contributor** | Identified person, possibly grouping several Git emails and a GitHub login |
| **Author / committer** | The author wrote the change; the committer recorded it in the history |
| **Inactivity** | Time elapsed since the last commit of a repository, across all tracked branches |
| **Actif / Flemme / Dead** | Activity states of a repository: 0 to 5 days, 6 to 15 days, more than 15 days since the last commit (adjustable thresholds) |
| **Sync run** | One execution of a repository sync, logged in `sync_runs` |
| **Skill** | Reusable know-how for a coding agent (instructions, references), installed from skills.sh |
| **HTMX** | Library refreshing HTML fragments without any application JavaScript |
