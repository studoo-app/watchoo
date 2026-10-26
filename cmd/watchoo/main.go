// Commande watchoo : point d'entrée du binaire.
package main

import (
	"os"

	"github.com/studoo-app/watchoo/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
