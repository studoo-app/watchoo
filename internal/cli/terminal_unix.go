//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cli

import "golang.org/x/sys/unix"

// systemWidth renvoie la largeur, en colonnes, du terminal ouvert sur fd (ioctl TIOCGWINSZ), ou 0 si
// elle est inconnue.
func systemWidth(fd uintptr) int {
	ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
	if err != nil {
		return 0
	}
	return int(ws.Col)
}

// enableColor indique si le terminal ouvert sur fd interprète les séquences ANSI : toujours vrai sous
// Unix.
func enableColor(uintptr) bool {
	return true
}
