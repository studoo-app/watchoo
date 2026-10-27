package cli

// Logs du processus (log/slog, texte, sur la sortie d'erreur).

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// EnvLogLevel est la variable qui règle le niveau des logs : debug, info, warn (défaut) ou error.
const EnvLogLevel = "WATCHOO_LOG_LEVEL"

// newLogger renvoie le logger du processus, écrit sur w (deps.Stderr) au niveau de EnvLogLevel
// (valeur inconnue : warn, avec un avertissement), ou Debug si verbose (option --verbose). Les
// logs ne contiennent jamais le token, ni message de commit, nom ou e-mail : seulement identifiants de
// dépôt, SHA courts et compteurs (règle des émetteurs : sync, provider, store).
func newLogger(w io.Writer, level string, verbose bool) *slog.Logger {
	lvl := slog.LevelWarn
	switch {
	case verbose:
		lvl = slog.LevelDebug
	default:
		parsed, ok := parseLogLevel(level)
		if !ok {
			warn(w, fmt.Sprintf("%s=%q inconnu : niveau warn utilisé (valeurs : debug, info, warn, error)",
				EnvLogLevel, level))
		}
		lvl = parsed
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl}))
}

// parseLogLevel analyse un niveau de log (casse et espaces ignorés) ; vide = warn. Une valeur inconnue
// donne warn et false.
func parseLogLevel(s string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return slog.LevelWarn, true
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelWarn, false
	}
}
