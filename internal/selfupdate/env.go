package selfupdate

// Variables d'environnement de la mise à jour.

import (
	"os"
	"strings"
)

const (
	// EnvNoCheck coupe toute vérification de nouvelle version (commandes et « watchoo serve ») quand
	// elle vaut autre chose que vide, « 0 » ou « false ». L'image Docker la pose.
	EnvNoCheck = "WATCHOO_NO_UPDATE_CHECK"
	// EnvDocker signale l'image Docker (posée par le Dockerfile) : « watchoo update » y est refusé.
	EnvDocker = "WATCHOO_DOCKER"
)

// envTrue indique une valeur de variable « vraie » : non vide, et ni « 0 » ni « false » (casse
// ignorée).
func envTrue(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

// CheckDisabled indique si la vérification des nouvelles versions est coupée par EnvNoCheck, lue par
// getenv (os.Getenv en production).
func CheckDisabled(getenv func(string) string) bool {
	return envTrue(getenv(EnvNoCheck))
}

// dockerEnvFile est le fichier que Docker crée à la racine des conteneurs.
var dockerEnvFile = "/.dockerenv"

// InDocker indique si Watchoo tourne dans un conteneur Docker : EnvDocker posée (image officielle) ou
// fichier /.dockerenv présent.
func InDocker() bool {
	if envTrue(os.Getenv(EnvDocker)) {
		return true
	}
	_, err := os.Stat(dockerEnvFile)
	return err == nil
}
