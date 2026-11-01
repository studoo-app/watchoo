//go:build !unix

package workspace

// checkTrusted ne contrôle rien hors Unix : les droits 0700 / 0600 n'y ont pas d'équivalent simple
// (voir documentation/fr/10-espaces-de-supervision.md).
func checkTrusted(*Workspace) error { return nil }
