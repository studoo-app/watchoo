# 9. Stratégie de tests

[← Sommaire](README.md)

Principe : **la CI ne dépend d'aucun service extérieur.** Pas de vrai `gh`, pas d'appel à l'API GitHub,
pas de token. Tout ce qui sort du binaire est simulé.

## Vue d'ensemble

| Ce qui est testé | Comment | Où |
| --- | --- | --- |
| Commande `gh` (token, état de connexion) | **Mock de `gh`** : interface + faux binaire (voir ci-dessous) | `internal/auth/` |
| API GitHub | Serveur HTTP simulé avec `httptest` | `internal/provider/github/` |
| Espaces de supervision | Répertoires temporaires : `init`, refus si déjà initialisé, recherche dans les parents, `-C`, `WATCHOO_DIR`, espace imbriqué, droits `0700`/`0600` | `internal/workspace/` |
| Base SQLite et requêtes sqlc | Base SQLite temporaire par test, migrations goose appliquées | `internal/store/` |
| Import groupé | Fichiers CSV et texte de test : ligne mal formée, doublons, dépôt inaccessible (`404`), relance idempotente, `--dry-run` sans écriture | `internal/cli/` + `internal/sync/` |
| Moteur de synchro | Provider simulé + base temporaire | `internal/sync/` |
| CLI | Exécution des commandes cobra, vérification de la sortie texte et JSON | `internal/cli/` |
| Web | `httptest` sur le routeur chi, vérification des pages et fragments | `internal/web/` |
| TUI | Tests du modèle Bubble Tea : messages envoyés, état et rendu obtenus | `internal/tui/` |

## Mock de la commande `gh`

Watchoo dépend de `gh` pour obtenir le token (voir
[Architecture](03-architecture.md#authentification--gh-imposé)). Pour que la CI n'ait besoin ni de `gh`
ni d'un compte GitHub, `gh` est simulé à deux niveaux.

### Niveau 1 — interface Go (tests unitaires)

L'accès à `gh` passe par une petite interface dans `internal/auth/`, par exemple :

- obtenir le token ;
- obtenir l'état de connexion (compte, droits).

L'implémentation réelle exécute `gh` (`os/exec`). Les tests des autres packages (synchro, CLI, web, TUI)
utilisent une **fausse implémentation en mémoire** qui renvoie la réponse voulue. Aucun processus n'est
lancé.

### Niveau 2 — faux binaire `gh` (tests de l'implémentation réelle)

Pour tester le code qui lance vraiment `gh` (arguments, lecture de la sortie, codes de retour,
erreurs), on utilise un **faux exécutable `gh`** :

- un petit programme Go dans `internal/auth/testdata/fakegh/`, compilé au début des tests ;
- le chemin de `gh` est réglable dans la config (`github.gh_path`, `gh` par défaut), ce qui permet aux
  tests de pointer vers le faux binaire sans toucher au `PATH` ;
- son comportement est choisi par une variable d'environnement propre aux tests (par exemple
  `FAKEGH_SCENARIO`).

Scénarios couverts :

| Scénario | Comportement du faux `gh` | Ce que Watchoo doit faire |
| --- | --- | --- |
| Connecté | `auth status` réussit, `auth token` renvoie un faux token | Utiliser le token |
| Non connecté | `auth status` échoue avec le message de `gh` | `init` et `status` indiquent `gh auth login` |
| `gh` absent | Chemin inexistant | Message clair : installer `gh` |
| Plusieurs comptes | `auth token --user X` renvoie un token différent | Respecter `github.user` ; refuser un `github.user` écrit avec une autre casse (`--user` distingue les majuscules) |
| `GH_TOKEN` seul (Docker, CI) | `auth status` ne connaît le compte que par `GH_TOKEN` ; `auth token` sans `--user` renvoie ce token, `auth token --user X` échoue | Utiliser le token sans `--user` ; refuser un `github.user` que `gh` ne connaît que par `GH_TOKEN` |
| Token refusé | `auth status` donne l'état `error` avec `HTTP 401: Bad credentials` | `init` et `status` indiquent `gh auth login` |
| GitHub injoignable | `auth status` donne l'état `error` (erreur réseau) ou `timeout` | Erreur claire, sans conseiller `gh auth login` |
| Token changé entre deux synchros | Deux appels renvoient deux tokens différents | Utiliser le nouveau token sans redémarrer |
| Sortie inattendue / délai dépassé | Sortie vide, ou `gh` qui ne répond pas | Erreur claire, délai maximum appliqué |

Règles :

- le faux token est une valeur factice (ex. `gho_fake_for_tests`) ;
- un test vérifie que ce faux token **n'apparaît jamais dans les logs** ni dans les sorties CLI, web ou
  TUI.

## API GitHub simulée

- Serveur `httptest` qui rejoue des réponses JSON enregistrées dans `testdata/`.
- Cas couverts : pagination, réponses `304` avec ETag, quota épuisé (en-têtes de rate limit), `404` sur
  un dépôt privé sans accès, `403` lié au SSO, dépôt renommé ou transféré.
- **Test de lecture seule** : le serveur simulé fait échouer le test dès qu'il reçoit une requête
  `POST`, `PATCH`, `PUT` ou `DELETE`.

## En CI

À chaque push et chaque pull request :

1. `go vet` et les tests (`go test ./...`, avec le détecteur de concurrence `-race`).
2. `sqlc generate` puis vérification qu'aucun fichier généré ne change (code commité à jour).
3. `make css` puis vérification que `app.css` commité est à jour.
4. Aucune variable `GH_TOKEN` / `GITHUB_TOKEN` n'est fournie aux tests : un test qui aurait besoin du vrai
   GitHub échoue au lieu de passer en silence.
