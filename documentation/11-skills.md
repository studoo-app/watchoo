# 11. Skills des agents de code

[← Sommaire](README.md)

Le projet est construit avec l'aide d'agents de code (Claude Code, Mistral Vibe…). Les **skills** leur
donnent des savoir-faire réutilisables : une procédure, des règles, des références. Watchoo utilise
l'annuaire ouvert **[skills.sh](https://www.skills.sh/)** et sa CLI `skills` pour les installer et les
gérer.

## Décision

- **Source unique** : les skills du projet viennent de [skills.sh](https://www.skills.sh/) (dépôts GitHub
  publics référencés par l'annuaire). Pas de skill copié à la main depuis une autre source.
- **Installation au niveau du projet**, pour que tous les contributeurs et tous les agents utilisent les
  mêmes skills.
- **Commités** dans le dépôt, avec le fichier de verrouillage `skills-lock.json`.

## Installer et mettre à jour

```bash
npx skills add <owner>/<repo>
```

```bash
npx skills update
```

- La CLI s'exécute avec `npx` : **Node est nécessaire pour cet outillage de développement uniquement**.
  Le principe « pas de Node » vaut pour le build et le binaire Watchoo, qui n'en dépendent toujours pas.
- La CLI envoie par défaut une télémétrie anonyme (nom et fichiers du skill). Pour la couper :
  `DISABLE_TELEMETRY=1`.

## Organisation dans le dépôt

```
watchoo/
  skills-lock.json         source, chemin et empreinte de chaque skill installé
  .agents/skills/          contenu des skills (référence unique)
    goland/
    semver/
    semver-changelog/
  .claude/skills/          liens symboliques vers .agents/skills/ (Claude Code)
  .vibe/skills/            liens symboliques vers .agents/skills/ (Mistral Vibe)
```

- `.agents/skills/` contient le seul exemplaire de chaque skill ; les répertoires propres à chaque agent
  n'en contiennent que des **liens symboliques**.
- `skills-lock.json` enregistre pour chaque skill son dépôt d'origine et une empreinte (`computedHash`) :
  une modification du contenu d'un skill se voit à la relecture.
- `.gitignore` : seul `.claude/worktrees` est ignoré, pour que `.claude/skills/` soit commité.

## Skills installés

| Skill | Source | Usage dans Watchoo |
| --- | --- | --- |
| `goland` | `g1joshi/agent-skills` | Développement Go : outils, débogueur delve, profilage, refactoring, lancement des tests |
| `semver` | `lelborn/skills` | Choisir le prochain numéro de version (MAJOR / MINOR / PATCH) selon SemVer 2.0.0 |
| `semver-changelog` | `prulloac/agent-skills` | Créer et tenir `CHANGELOG.md` au format « Keep a Changelog », à partir des commits depuis le dernier tag |

`semver` et `semver-changelog` servent dès la **phase 5** (GoReleaser, premières versions publiées) :
chaque release a un numéro SemVer et une entrée dans `CHANGELOG.md`.

## Versions et `CHANGELOG.md`

- **Surface publique** sur laquelle se décide le numéro (skill `semver`) : commandes, options et codes de
  sortie de la CLI, clés de la sortie `--json` ([CLI](05-cli.md#sortie-json)), réglages de
  `config.yaml` et variables `WATCHOO_*`, schéma de la base `.watchoo/watchoo.db` (migrations), routes de
  l'interface web.
- **Première version : `v0.1.0`** (MVP, phases 1 à 5). Tant que Watchoo est en `0.y.z`, rien n'est
  garanti stable ; selon le skill `semver`, un changement incompatible (clé `--json` renommée ou
  retirée, option supprimée…) fait monter **MINOR** (`0.1.3` → `0.2.0`), et tout changement compatible,
  fonctionnalité ou correction, fait monter **PATCH** (`0.1.0` → `0.1.1`). À partir de `1.0.0`, la règle
  habituelle s'applique (incompatible → MAJOR).
- **`CHANGELOG.md`** (racine) suit [Keep a Changelog 1.1.0](https://keepachangelog.com/fr/1.1.0/), en
  français : rubriques Ajouté, Modifié, Obsolète, Retiré, Corrigé, Sécurité ; section `[Unreleased]`
  tenue au fil des commits ; liens de comparaison GitHub en bas du fichier. Le skill `semver-changelog`
  part du dernier tag SemVer (`git tag -l --sort=-v:refname`) et des commits depuis ce tag.
- **Release** : on renomme `[Unreleased]` en `[X.Y.Z] - AAAA-MM-JJ`, on ajoute une section `[Unreleased]`
  vide et les liens, on commite, puis on pousse le tag `vX.Y.Z` ; le workflow de release
  ([Distribution](03-architecture.md#distribution)) fait le reste, et la section du `CHANGELOG.md` est
  recopiée dans la release GitHub en brouillon avant sa publication.

## Règles

- **Relire avant d'installer** : un skill est un ensemble d'instructions (et parfois de scripts) que
  l'agent va suivre. On lit son `SKILL.md` et ses fichiers avant de l'ajouter, et on vérifie sa page sur
  skills.sh (audits de sécurité, popularité, dépôt d'origine).
- **Un skill = un besoin du projet** : on n'installe que ce qui sert à Watchoo.
- **Mises à jour relues** comme du code : la différence du skill et de `skills-lock.json` passe en revue
  dans le commit.
- Les instructions du projet (documentation, prompt de l'agent) priment sur celles d'un skill en cas de
  contradiction.
