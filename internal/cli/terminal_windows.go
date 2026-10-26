//go:build windows

package cli

import (
	"github.com/mattn/go-isatty"
	"golang.org/x/sys/windows"
)

// systemWidth renvoie la largeur, en colonnes, de la fenêtre de la console ouverte sur fd, ou 0 si elle
// est inconnue (terminal Cygwin ou MSYS2, par exemple, qui n'est pas une console).
func systemWidth(fd uintptr) int {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(fd), &info); err != nil {
		return 0
	}
	return int(info.Window.Right-info.Window.Left) + 1
}

// enableColor indique si le terminal ouvert sur fd interprète les séquences ANSI. Un terminal Cygwin ou
// MSYS2 les interprète. Une console ne les interprète qu'en mode « traitement des séquences » (activé
// par Windows Terminal) : s'il ne l'est pas, enableColor l'active, comme le fait gh ; si la console le
// refuse (Windows antérieur à Windows 10), les couleurs sont désactivées.
func enableColor(fd uintptr) bool {
	if isatty.IsCygwinTerminal(fd) {
		return true
	}
	h := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
