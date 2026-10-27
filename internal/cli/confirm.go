package cli

// Confirmation interactive (« watchoo repo remove »).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/studoo-app/watchoo/internal/textclean"
)

// errNeedYes est l'erreur d'une confirmation impossible : l'entrée standard n'est pas un terminal.
var errNeedYes = errors.New("confirmation impossible : l'entrée standard n'est pas un terminal, " +
	"relancez avec --yes pour confirmer")

// confirm pose la question prompt sur deps.Stderr et lit une ligne sur deps.Stdin : « o », « oui »,
// « y », « yes » (casse ignorée) = oui ; tout le reste, ou la fin de l'entrée, = non. Si l'entrée n'est
// pas un terminal (deps.StdinIsTerminal), elle ne lit rien et renvoie une erreur qui demande l'option
// --yes.
//
// Si ctx est annulé pendant l'attente (Ctrl+C : Execute capte SIGINT et annule le contexte au lieu de
// tuer le processus), confirm rend la main tout de suite avec « non », sans attendre Entrée : la
// lecture, qui ne connaît pas ctx, se fait dans une goroutine abandonnée à l'annulation (le processus
// se termine juste après).
func confirm(ctx context.Context, deps Deps, prompt string) (bool, error) {
	if !deps.StdinIsTerminal() {
		return false, errNeedYes
	}
	if _, err := fmt.Fprintf(deps.Stderr, "%s [o/N] ", textclean.Clean(prompt)); err != nil {
		return false, fmt.Errorf("affichage de la question : %w", err)
	}
	type answer struct {
		line string
		err  error
	}
	read := make(chan answer, 1)
	go func() {
		line, err := readLine(deps.Stdin)
		read <- answer{line, err}
	}()
	var line string
	var err error
	select {
	case <-ctx.Done():
		fmt.Fprintln(deps.Stderr) // la question n'a pas reçu de fin de ligne
		return false, nil
	case a := <-read:
		line, err = a.line, a.err
	}
	if errors.Is(err, io.EOF) {
		// Fin de l'entrée (ctrl+D) : le terminal n'a pas affiché de fin de ligne après la question.
		fmt.Fprintln(deps.Stderr)
	} else if err != nil {
		return false, fmt.Errorf("lecture de la réponse : %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "o", "oui", "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// readLine lit r octet par octet jusqu'à la première fin de ligne (non comprise) ou la fin de
// l'entrée : rien n'est consommé au-delà de la réponse. Un « \r » final (Windows) est retiré par
// l'appelant (TrimSpace).
func readLine(r io.Reader) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for b.Len() < 1024 {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				return b.String(), nil
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			return b.String(), err
		}
	}
	return b.String(), nil
}
