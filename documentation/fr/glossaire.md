# Glossaire

[← Sommaire](README.md)

| Terme | Définition |
| --- | --- |
| **Espace de supervision** | Répertoire initialisé par `watchoo init` (dossier `.watchoo/`), avec sa config et sa base : un groupe supervisé, par exemple une promotion d'étudiants |
| **Dépôt suivi** | Dépôt GitHub enregistré dans Watchoo et synchronisé régulièrement |
| **Branche suivie** | Branche d'un dépôt suivi dont les commits sont récupérés |
| **Provider** | Interface d'accès à une forge (GitHub au MVP ; GitLab, Gitea, Git local ensuite) |
| **Forge** | Service d'hébergement de dépôts Git (GitHub, GitLab, Gitea…) |
| **Synchro incrémentale** | Récupération des seuls commits nouveaux depuis le dernier SHA connu |
| **SHA** | Identifiant unique d'un commit Git |
| **ETag** | En-tête HTTP permettant une requête conditionnelle ; une réponse 304 ne consomme pas de quota GitHub |
| **Rate limit / quota** | Limite de requêtes à l'API GitHub (5 000 / heure avec token) |
| **Contributeur** | Personne identifiée, regroupant éventuellement plusieurs e-mails Git et un login GitHub |
| **Auteur / committer** | L'auteur a écrit le changement ; le committer l'a enregistré dans l'historique |
| **Inactivité** | Durée écoulée depuis le dernier commit d'un dépôt, toutes branches suivies confondues |
| **Actif / Flemme / Dead** | États d'activité d'un dépôt : 0 à 5 j, 6 à 15 j, plus de 15 j depuis le dernier commit (seuils réglables) |
| **Sync run** | Une exécution de synchronisation d'un dépôt, journalisée dans `sync_runs` |
| **Skill** | Savoir-faire réutilisable pour un agent de code (instructions, références), installé depuis skills.sh |
| **HTMX** | Bibliothèque permettant de rafraîchir des fragments HTML sans JavaScript applicatif |
