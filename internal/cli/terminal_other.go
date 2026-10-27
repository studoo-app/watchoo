//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package cli

// systemWidth renvoie 0 : sur ce système, la largeur du terminal n'est connue que par COLUMNS.
func systemWidth(uintptr) int {
	return 0
}

// enableColor indique si le terminal ouvert sur fd interprète les séquences ANSI : supposé vrai.
func enableColor(uintptr) bool {
	return true
}
