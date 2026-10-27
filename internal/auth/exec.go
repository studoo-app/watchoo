package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Limites appliquées à l'exécution de gh.
const (
	// maxStdout borne la sortie standard lue (la réponse de « auth status » fait quelques centaines
	// d'octets) ; au-delà, la réponse est jugée inattendue.
	maxStdout = 1 << 20
	// maxStderr borne la sortie d'erreur conservée ; le surplus est ignoré.
	maxStderr = 64 << 10
	// waitDelay est le temps laissé aux tubes pour se fermer après la fin de gh ou l'expiration du
	// délai, avant que Wait ne les ferme de force.
	waitDelay = time.Second
	// maxSummaryRunes est la longueur maximale de l'extrait de stderr repris dans une erreur.
	maxSummaryRunes = 200
)

// ghEnv est ajouté à l'environnement hérité : gh ne doit jamais attendre de saisie ni chercher de
// mise à jour. En cas de doublon, os/exec retient la dernière valeur, donc celles-ci.
var ghEnv = []string{"GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1"}

// tokenPattern reconnaît les tokens GitHub (classiques gho_, ghp_, ghu_, ghs_, ghr_ et fins
// github_pat_). Ils sont masqués dans tout extrait de stderr repris dans une erreur.
var tokenPattern = regexp.MustCompile(`(?:gh[opusr]_|github_pat_)[A-Za-z0-9_]+`)

// redacted remplace un token reconnu par tokenPattern.
const redacted = "[masqué]"

// result est le résultat d'un appel à gh qui a pu aller à son terme.
type result struct {
	stdout   []byte // sortie standard (tronquée à maxStdout) ; ne jamais la journaliser
	overflow bool   // la sortie standard dépassait maxStdout
	stderr   string // sortie d'erreur (tronquée à maxStderr)
	exitCode int    // code de retour ; -1 si gh a été arrêté par un signal
}

// run lance « <Path> args… » et attend sa fin, dans la limite du délai maximum. name (ex.
// « auth token ») sert aux messages d'erreur et aux logs.
//
// Un code de retour non nul n'est pas une erreur de run : il est renvoyé dans result.exitCode, à
// interpréter par l'appelant. run renvoie une erreur quand gh n'a pas pu être lancé (ErrGHNotFound…),
// quand le délai a expiré (ErrTimeout), quand le contexte de l'appelant est terminé (ctx.Err()) ou
// quand la sortie n'a pas pu être lue jusqu'au bout. Aucune erreur ne contient la sortie de gh.
func (g GH) run(ctx context.Context, name string, args ...string) (result, error) {
	path := g.path()
	timeout := g.timeout()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout := &cappedBuffer{max: maxStdout}
	stderr := &cappedBuffer{max: maxStderr}
	cmd := exec.CommandContext(runCtx, path, args...) // Cancel par défaut : Process.Kill
	cmd.Env = append(os.Environ(), ghEnv...)
	cmd.Stdin = nil // /dev/null : gh ne peut rien demander
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay

	start := time.Now()
	runErr := cmd.Run()
	res, err := interpret(ctx, runCtx, runErr, path, name, timeout)
	if err == nil {
		res.stdout = stdout.buf.Bytes()
		res.overflow = stdout.overflow
		res.stderr = stderr.buf.String()
	}

	// Log de diagnostic : jamais de sortie de gh, seulement la commande et son issue.
	attrs := []any{
		slog.String("commande", name),
		slog.String("hote", g.host()),
		slog.Duration("duree", time.Since(start).Round(time.Millisecond)),
	}
	if err != nil {
		attrs = append(attrs, slog.String("erreur", err.Error()))
	} else {
		attrs = append(attrs, slog.Int("code", res.exitCode))
	}
	slog.DebugContext(ctx, "appel de gh", attrs...)

	return res, err
}

// interpret traduit l'erreur de cmd.Run en résultat ou en erreur du package. L'ordre des cas compte :
// l'annulation par l'appelant l'emporte sur le délai, qui l'emporte sur le code de retour (un gh tué
// au délai sort avec un code non nul).
func interpret(ctx, runCtx context.Context, runErr error, path, name string, timeout time.Duration) (result, error) {
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
		return result{}, nil
	case ctx.Err() != nil:
		return result{}, fmt.Errorf("« gh %s » interrompu : %w", name, ctx.Err())
	case runCtx.Err() != nil:
		return result{}, fmt.Errorf("« gh %s » : %w (%s)", name, ErrTimeout, timeout)
	case errors.Is(runErr, exec.ErrNotFound), errors.Is(runErr, fs.ErrNotExist):
		return result{}, fmt.Errorf("lancement de %q : %w", path, ErrGHNotFound)
	case errors.As(runErr, &exitErr):
		code := exitErr.ExitCode()
		if code == 0 {
			code = -1 // ne devrait pas arriver : une ExitError signale un échec
		}
		return result{exitCode: code}, nil
	case errors.Is(runErr, exec.ErrWaitDelay):
		// gh a fini mais un processus enfant garde ses tubes ouverts : sortie peut-être incomplète.
		return result{}, fmt.Errorf("« gh %s » : sortie non terminée : %w", name, ErrUnexpectedOutput)
	default:
		// Erreur de démarrage (droits, format d'exécutable…) : elle ne cite que le chemin.
		return result{}, fmt.Errorf("lancement de %q impossible : %w", path, runErr)
	}
}

// cappedBuffer est un io.Writer qui garde au plus max octets et note s'il en a reçu davantage. Il
// accepte toujours l'écriture complète, pour que gh ne soit jamais bloqué sur un tube plein.
type cappedBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

// Write implémente io.Writer.
func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.max - c.buf.Len()
	if len(p) > room {
		c.overflow = true
		if room > 0 {
			c.buf.Write(p[:room])
		}
		return len(p), nil
	}
	c.buf.Write(p)
	return len(p), nil
}

// looksNotLoggedIn indique si la sortie d'erreur de gh signale une absence de connexion, par exemple
// « You are not logged into any GitHub hosts. To log in, run: gh auth login » ou
// « no oauth token found for github.com ».
func looksNotLoggedIn(stderr string) bool {
	s := strings.ToLower(stderr)
	for _, marker := range []string{"not logged in", "gh auth login", "no oauth token"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// summarize renvoie la première ligne non vide de stderr, sur une ligne, débarrassée des caractères
// de contrôle, avec les tokens GitHub masqués et raccourcie à maxSummaryRunes caractères. Chaîne
// vide si stderr ne contient rien d'affichable.
func summarize(stderr string) string {
	var line string
	for l := range strings.Lines(stderr) {
		if l = strings.TrimSpace(l); l != "" {
			line = l
			break
		}
	}
	if line == "" {
		return ""
	}
	line = strings.ToValidUTF8(line, "?")
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, line)
	line = tokenPattern.ReplaceAllString(line, redacted)
	if utf8.RuneCountInString(line) > maxSummaryRunes {
		line = string([]rune(line)[:maxSummaryRunes]) + "…"
	}
	return line
}
