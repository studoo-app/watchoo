# 2. Fonctionnalités

[← Sommaire](README.md)

Le MVP se limite à **suivre, synchroniser et afficher** ; tout le reste vient ensuite.

## MVP

| Fonctionnalité | Détail |
| --- | --- |
| Ajouter / retirer un dépôt suivi | Saisie `owner/repo` ou URL GitHub ; toutes les branches suivies par défaut, restreignables par liste ou motif. Ajout par `watchoo repo add`, l'import, la page Réglages du web ou la touche `a` du TUI (vue Dépôts : toutes les branches, synchro lancée tout de suite) |
| Import groupé de dépôts | Fichier CSV (dépôt, nom affiché, branches) : toute une promotion d'un coup, accès vérifié, bilan, relançable sans doublon |
| Synchronisation des commits | Récupération **incrémentale** depuis le dernier SHA connu |
| Liste des derniers commits | Message, auteur, date, SHA court, lien GitHub ; filtres par dépôt, auteur, période |
| Vue contributeurs | Nombre de commits par personne et par dépôt, date de dernière activité |
| Statut d'un dépôt | Dernier commit, état (Actif / Flemme / Dead), branche par défaut, étoiles, issues ouvertes |
| Synchro automatique | Tâche périodique configurable : 30 min par défaut, 15 min minimum (dans `serve` et `tui`), désactivable |
| Synchro manuelle | À tout moment, tous les dépôts ou un seul : `watchoo sync`, bouton dans le web, touche dans le TUI |
| Graphiques d'activité (web) | Commits par jour sur 90 jours pour l'espace et pour chaque dépôt ; commits par semaine sur 12 semaines des 8 contributeurs les plus actifs |

## V2

| Fonctionnalité | Détail |
| --- | --- |
| Activité par contributeur dans un dépôt | Graphique des commits de chaque contributeur à l'intérieur d'un même dépôt. **Publié en v0.4.0** : sur la page d'un dépôt (web), commits par semaine sur 12 semaines des 8 contributeurs les plus actifs du dépôt, comme la page des contributeurs ; total et courbe par semaine des principaux contributeurs dans le détail d'un dépôt du TUI ; `watchoo contributors [--repo X] --weeks N` (commits semaine par semaine, clé `weekly` en JSON). Toutes les branches suivies comptent, chaque commit une fois |
| Seuils d'activité par dépôt | Régler les seuils Flemme / Dead d'un dépôt (option de `repo add`, colonne de l'import, réglage web). **Publié en v0.4.0** : `repo add --flemme N --dead N`, colonnes `flemme` et `dead` de l'import, partage de la configuration, formulaire de la page d'un dépôt |
| Pull requests et releases | PR ouvertes / mergées, tags, releases. **Publié en v0.4.0** : lues à chaque synchro (PR de façon incrémentale, depuis la dernière modification connue ; releases et tags en entier, gratuits quand rien n'a changé), `watchoo prs [--repo X] [--state open\|merged\|closed\|all]` et `watchoo releases [--repo X]` (texte et `--json`), cartes « Pull requests » et « Releases et tags » sur la page d'un dépôt (web), PR ouvertes sur le tableau de bord, PR ouvertes et dernière release dans le détail du TUI. Un échec sur ces listes n'empêche pas la synchro des commits : il devient un avertissement |
| Dépôts Git locaux | Suivre un dépôt depuis un clone local (`git clone`, puis `git fetch` périodique) : commits, branches et tags sans appel à l'API GitHub. Utilisé quand `gh` n'est pas installé ou connecté. **Publié en v0.4.0** : choix refait à chaque synchro selon `gh` seulement ; clones avec copie de travail dans `repos/<name>` à la racine de l'espace (homonymes : `repos/<owner>-<name>` ; modifications locales des fichiers suivis écrasées à chaque synchro) (SSH par défaut, `git.protocol: https` possible), avec les identifiants Git de la machine, sans invite ; `watchoo init` sans `gh` prévient au lieu de s'arrêter ; source affichée par `watchoo status`, le bilan de `watchoo sync` et la page Réglages ; étoiles, issues, pull requests et releases masquées pour un dépôt jamais lu par l'API ; `repo remove` supprime le clone (`--keep-history` le garde) ; image Docker avec `git` |

## 0.5.0

Version suivante de la V2, décidée par le mainteneur : elle complète la vue contributeurs.

| Fonctionnalité | Détail |
| --- | --- |
| Page par contributeur | Une page par contributeur : ses dépôts, ses commits et son activité dans le temps, sur le web et dans le TUI. **Publié en v0.5.0** : page web `/contributors/{id}` (en-tête avec nom, login, avatar et nombre d'identités Git connues, sans aucune adresse ; totaux de commits, lignes `+/-`, dépôts, premier et dernier commit ; commits par jour sur 90 jours ; commits par semaine et par dépôt sur 12 semaines ; ses dépôts avec commits, lignes, dernier commit et état du dépôt ; ses commits paginés ; ses pull requests quand son compte de forge est connu), liens vers cette page depuis toutes les listes où un contributeur apparaît ; détail d'un contributeur dans le TUI (`entrée` dans la vue Contributeurs). Pas d'état Actif / Flemme / Dead pour une personne : ces états restent des règles de dépôt ; la page donne l'ancienneté du dernier commit et l'état de chacun de ses dépôts |
| Lien vers le profil de forge | Partout où l'interface web montre un contributeur dont le compte de forge est connu, un lien secondaire « GitHub » ouvre son profil (adresse fournie par la forge, enregistrée dans `contributors.profile_url`). Aucun lien pour un contributeur connu seulement par son nom et son e-mail Git (clones Git locaux) |

## 0.6.0

Version suivante, décidée par le mainteneur : Watchoo signale lui-même ses nouvelles versions et sait se
mettre à jour.

| Fonctionnalité | Détail |
| --- | --- |
| Avis de nouvelle version | À **chaque lancement** d'une commande, Watchoo vérifie, sans bloquer, si une version plus récente que la sienne est publiée, et l'annonce sur la sortie d'erreur à la fin de la commande (« Nouvelle version de Watchoo disponible : v0.5.0 → v0.6.0. Mettez-la à jour avec « watchoo update ». »). Aucun avis avec `--json`, hors terminal (scripts, CI), pour une version de développement, ni si la variable `WATCHOO_NO_UPDATE_CHECK` est posée (image Docker). `watchoo serve` vérifie au démarrage puis toutes les 24 h, et l'interface web montre alors un bandeau discret. **Publié en v0.6.0** |
| Mise à jour en une commande | `watchoo update` affiche la version installée et la dernière, demande confirmation, télécharge l'archive de la release pour le système, vérifie sa somme SHA-256 et remplace le binaire d'un bloc ; `watchoo update --check` ne fait que vérifier. Refus expliqué dans l'image Docker (changer le tag), pour une version compilée depuis les sources (`go install …@latest` ou `git pull && make build`) et si le répertoire du binaire n'est pas accessible en écriture. Seules les pages publiques des releases de Watchoo sont lues : ni token, ni API GitHub, ni quota. **Publié en v0.6.0** |

## V3

| Fonctionnalité | Détail |
| --- | --- |
| Suivi d'une organisation entière | Importer tous les dépôts d'un compte ou d'une organisation |
| Alertes | Webhook Discord / Slack / e-mail, ou notification desktop |
| Webhooks GitHub entrants | Mise à jour en temps réel au lieu du polling |
| Autres forges | GitLab, Gitea |
| Export | CSV / JSON pour rapports |

## Règles fonctionnelles

- **États d'activité** : calculés à partir de l'âge du dernier commit, en jours entiers.

  | État | Âge du dernier commit (défaut) | Web / TUI | CLI (`repo list`) |
  | --- | --- | --- | --- |
  | **Actif** | 0 à 5 jours | vert | `-` |
  | **Flemme** | 6 à 15 jours | orange | `~ 9 j` |
  | **Dead** | plus de 15 jours | rouge | `⚠ 42 j` |

  - « Dernier commit » = le plus récent **sur toutes les branches suivies** du dépôt, pas seulement la
    branche par défaut.
  - Les deux seuils (5 et 15 jours) se règlent **globalement** dans la config (utilisateur ou espace),
    et peuvent être remplacés **pour un dépôt** (une bibliothèque stable qui bouge rarement, un projet
    rendu en fin de semestre…) : `watchoo repo add owner/repo --flemme N --dead N`, colonnes `flemme` et
    `dead` du fichier d'import, partage de la configuration, ou formulaire « Seuils d'activité » de la
    page d'un dépôt dans l'interface web. Chaque seuil est indépendant : un dépôt peut ne fixer que son
    seuil Dead et garder le seuil Flemme de l'espace. `0` (ou un champ vide dans le web) revient au
    seuil de l'espace.
  - Un seuil propre vaut de 1 à 3 650 jours ; quand les deux sont donnés ensemble, Dead doit être
    strictement supérieur à Flemme (sinon refus, avant tout appel à GitHub). Si le mélange d'un seuil
    propre et d'un seuil de l'espace donne Dead inférieur ou égal à Flemme, Dead est ramené à Flemme + 1
    jour pour le calcul de l'état.
- **États hors activité** : affichés en **gris**, ils sont prioritaires sur les états d'activité.
  - **Jamais synchronisé** : le dépôt vient d'être ajouté.
  - **En erreur** : la dernière synchro a échoué (dépôt supprimé, accès perdu…).
  - **Archivé** : le dépôt est archivé sur GitHub, il ne bougera plus ; il n'est pas signalé en Dead.
  - **Arrêté** : le suivi est arrêté (`repo remove --keep-history`) ; l'historique reste consultable.
  - **Vide** : le dépôt a été synchronisé mais n'a aucun commit (dépôt d'étudiant tout juste créé).
  - Priorité entre états gris : Arrêté, En erreur, Jamais synchronisé, Archivé, Vide.
- **Jours** : comptés en jours calendaires dans le fuseau d'affichage (réglage `timezone`, sinon celui du
  système) : un commit d'hier à 23 h a 1 jour, quelle que soit l'heure.
- **Incrémental** : un commit déjà présent en base n'est jamais re-téléchargé.
- **Identité** : un contributeur peut utiliser plusieurs e-mails ; ils sont rattachés à son login GitHub
  quand celui-ci est connu.
- **Lecture seule** : Watchoo n'écrit jamais sur GitHub.
