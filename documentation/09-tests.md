# 9. Stratégie de tests

[← Sommaire](README.md)

Principe : **la CI ne dépend d'aucun service extérieur.** Pas de vrai `gh`, pas d'appel à l'API GitHub,
pas de token. Tout ce qui sort du binaire est simulé.

## Vue d'ensemble

| Ce qui est testé | Comment | Où |
| --- | --- | --- |
| Commande `gh` (token, état de connexion) | **Mock de `gh`** : interface + faux binaire (voir ci-dessous) | `internal/auth/` |
| API GitHub | Faux serveur GitHub programmable (`githubtest`, sur `httptest`) | `internal/provider/github/` |
| Espaces de supervision | Répertoires temporaires : `init`, refus si déjà initialisé, recherche dans les parents, `-C`, `WATCHOO_DIR`, espace imbriqué, droits `0700`/`0600` | `internal/workspace/` |
| Base SQLite et requêtes sqlc | Base SQLite temporaire par test, migrations goose appliquées | `internal/store/` |
| Import groupé | Fichiers CSV et texte de test : ligne mal formée, doublons, dépôt inaccessible (`404`), relance idempotente, `--dry-run` sans écriture | `internal/cli/` + `internal/sync/` |
| Moteur de synchro | Faux Provider en mémoire (`providertest`) + base temporaire | `internal/sync/` |
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

Deux faux, écrits en phase 2 et réutilisés par les phases suivantes :

| Faux | Package | Sert à tester |
| --- | --- | --- |
| Serveur GitHub programmable | `internal/provider/github/githubtest` | Le vrai provider GitHub (transport, cache ETag, quota, erreurs) ; en phase 3, le moteur de synchro de bout en bout |
| `Provider` en mémoire | `internal/provider/providertest` | Le moteur de synchro, la CLI, le web et le TUI, sans HTTP |

- **`githubtest`** : serveur `httptest` sur `127.0.0.1` qui simule les routes utilisées (dépôt, branches,
  commits, commit avec statistiques, compare, `/rate_limit`) à partir d'un état préparé par le test :
  dépôts, graphe de commits (merges compris), branches, force-push, renommage ou transfert (`301`), accès
  refusé (`404`) ou SSO (`403`), dépôt vide (`409`). Il reproduit le format JSON de GitHub, la pagination
  (en-tête `Link`), les ETag et les `304` (sans quota), le quota (en-têtes `X-RateLimit-*`, épuisement,
  remise à zéro), les limites secondaires (`Retry-After`), et tient un journal des requêtes reçues (sans
  la valeur de `Authorization`). Une réponse peut aussi être imposée pour un chemin donné.
- **`providertest`** : implémente `provider.Provider` avec la même sémantique que le provider GitHub
  (validation, ordre des commits, `Stop`, `Since`, `Limit`, dépôt vide, `ErrUnknownBase`, renommages),
  plus l'injection d'erreurs, le comptage des appels et un quota simulé.
- `internal/provider/github/testdata/` : quelques réponses JSON factices au format exact de GitHub, pour
  les tests de conversion des types.
- **Scénarios** (le vrai provider contre `githubtest`, `internal/provider/github/scenario_test.go`) :
  pagination des branches et des commits ; synchro incrémentale (premier import par la liste des
  commits, nouvelle branche par compare depuis la tête de la branche par défaut, y compris une branche
  qui a fusionné `main`, compare entre l'ancienne et la nouvelle tête, y compris un merge qui apporte des
  commits plus anciens que l'ancienne tête, et la perte de commits que provoquerait l'arrêt au premier
  SHA connu) ; force-push (`ErrUnknownBase`, puis repli) ; réponses `304` avec ETag (quota non décompté, cache
  partagé entre deux providers du même token et vidé par un autre token) ; quota épuisé ; limite
  secondaire ; `404` sur un dépôt privé sans accès ; `403` lié au SSO ; `401` ; dépôt renommé ou
  transféré ; dépôt vide ; dépôt archivé ; 4 dépôts synchronisés en parallèle (sous `-race`).
- **Test de lecture seule** : le serveur simulé fait échouer le test dès qu'il reçoit une requête
  `POST`, `PATCH`, `PUT` ou `DELETE`, et un test vérifie que le transport du provider les refuse avant
  l'envoi.
- **Test de fuite** : dans tous ces scénarios, les logs (capturés au niveau Debug) et les erreurs ne
  contiennent ni le faux token, ni en-tête `Authorization`, ni message de commit, nom ou e-mail.

## Logo et carte d'en-tête

- Les copies embarquées dans `internal/logo/art/` sont identiques octet pour octet à `assets/logo/*.txt`
  (sinon : `make logo`).
- Carte : toutes les lignes ont la même largeur affichée (accents, « », caractères de bloc), coupure de
  l'astuce, borne de 80 colonnes, pas de carte sous 40, repli 100 % ASCII, couleurs seulement si permises.
- CLI : carte affichée seulement sur un terminal simulé, jamais avec `--json`, sur stderr ni dans une sortie
  redirigée (la sortie de `watchoo version` hors terminal ne change pas).

## En CI

À chaque push et chaque pull request :

1. `go vet` et les tests (`go test ./...`, avec le détecteur de concurrence `-race`).
2. `sqlc generate` puis vérification qu'aucun fichier généré ne change (code commité à jour).
3. `make css` puis vérification que `app.css` commité est à jour.
4. Aucune variable `GH_TOKEN` / `GITHUB_TOKEN` n'est fournie aux tests : un test qui aurait besoin du vrai
   GitHub échoue au lieu de passer en silence.
