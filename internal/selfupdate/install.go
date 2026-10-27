package selfupdate

// Installation d'une version : téléchargement de l'archive et du fichier de sommes, vérification
// SHA-256, extraction du binaire, remplacement atomique de l'exécutable en cours.

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Erreurs de refus de la mise à jour (CheckInstallable, CheckWritable) ; la CLI les traduit en
// messages qui disent quoi faire.
var (
	// ErrDocker : Watchoo tourne dans l'image Docker, mise à jour en changeant de tag.
	ErrDocker = errors.New("watchoo tourne dans un conteneur Docker")
	// ErrDevBuild : la version installée n'est pas une version de release (compilée depuis les sources).
	ErrDevBuild = errors.New("version de développement")
	// ErrNotWritable : le répertoire du binaire n'est pas accessible en écriture.
	ErrNotWritable = errors.New("répertoire du binaire non accessible en écriture")
	// ErrChecksum : la somme SHA-256 de l'archive ne correspond pas au fichier de sommes.
	ErrChecksum = errors.New("somme SHA-256 de l'archive incorrecte : archive corrompue ou modifiée")
)

// ArchiveName renvoie le nom de l'archive de la release tag pour goos / goarch, comme
// .goreleaser.yaml : watchoo_0.6.0_linux_amd64.tar.gz, watchoo_0.6.0_windows_arm64.zip.
func ArchiveName(tag, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("watchoo_%s_%s_%s%s", strings.TrimPrefix(tag, "v"), goos, goarch, ext)
}

// ChecksumsName renvoie le nom du fichier de sommes de la release tag.
func ChecksumsName(tag string) string {
	return fmt.Sprintf("watchoo_%s_checksums.txt", strings.TrimPrefix(tag, "v"))
}

// binaryName renvoie le nom du binaire dans l'archive.
func binaryName(goos string) string {
	if goos == "windows" {
		return "watchoo.exe"
	}
	return "watchoo"
}

// CheckInstallable vérifie que la version current peut être remplacée par « watchoo update » :
// ErrDocker dans un conteneur (inDocker), ErrDevBuild si current n'est pas une version de release.
func CheckInstallable(current string, inDocker bool) error {
	if inDocker {
		return ErrDocker
	}
	if !IsRelease(current) {
		return fmt.Errorf("%w : %s", ErrDevBuild, current)
	}
	return nil
}

// CheckWritable vérifie qu'un fichier peut être créé dans dir (répertoire du binaire), en y créant
// puis supprimant un fichier temporaire. Erreur enveloppant ErrNotWritable sinon.
func CheckWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".watchoo-update-check-*")
	if err != nil {
		return fmt.Errorf("%w : %s : %w", ErrNotWritable, dir, err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("%w : %s : %w", ErrNotWritable, dir, err)
	}
	return nil
}

// Executable renvoie le chemin réel du binaire en cours : os.Executable, liens symboliques résolus
// (le binaire remplacé est la cible du lien, pas le lien).
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("chemin du binaire en cours introuvable : %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("chemin du binaire en cours introuvable : %w", err)
	}
	return resolved, nil
}

// Install télécharge la release tag pour le système du Client, vérifie l'archive et remplace exe
// (Replace). Rien n'est modifié si une étape échoue avant le remplacement.
func (c *Client) Install(ctx context.Context, tag, exe string) error {
	bin, err := c.Download(ctx, tag)
	if err != nil {
		return err
	}
	return Replace(exe, bin, c.goos)
}

// Download télécharge l'archive de la release tag et le fichier de sommes, vérifie la somme SHA-256
// de l'archive et renvoie le binaire qu'elle contient.
func (c *Client) Download(ctx context.Context, tag string) ([]byte, error) {
	if v, ok := ParseVersion(tag); !ok || !v.IsFinal() || !strings.HasPrefix(tag, "v") {
		return nil, fmt.Errorf("version invalide : %q (attendu : vX.Y.Z)", tag)
	}
	archive := ArchiveName(tag, c.goos, c.goarch)
	sums, err := c.download(ctx, maxChecksumsSize, "download", tag, ChecksumsName(tag))
	if err != nil {
		return nil, err
	}
	want, err := findChecksum(sums, archive)
	if err != nil {
		return nil, err
	}
	data, err := c.download(ctx, maxArchiveSize, "download", tag, archive)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(data)
	if subtle.ConstantTimeCompare(got[:], want) != 1 {
		return nil, fmt.Errorf("%s : %w", archive, ErrChecksum)
	}
	return extractBinary(data, archive, binaryName(c.goos))
}

// download lit <racine>/elems… (au plus limit octets).
func (c *Client) download(ctx context.Context, limit int64, elems ...string) ([]byte, error) {
	u, err := c.endpoint(elems...)
	if err != nil {
		return nil, err
	}
	name := elems[len(elems)-1]
	resp, err := c.get(ctx, c.http, u)
	if err != nil {
		return nil, fmt.Errorf("téléchargement de %s : %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("téléchargement de %s : réponse HTTP %d (version ou plateforme sans archive ?)",
			name, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("téléchargement de %s : %w", name, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("téléchargement de %s : fichier trop gros (plus de %d Mo)", name, limit>>20)
	}
	return data, nil
}

// findChecksum cherche la somme SHA-256 de archive dans un fichier de sommes (« <sha256>  <nom> »).
func findChecksum(sums []byte, archive string) ([]byte, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != archive {
			continue
		}
		sum, err := hex.DecodeString(fields[0])
		if err != nil || len(sum) != sha256.Size {
			return nil, fmt.Errorf("somme de %s illisible dans le fichier de sommes", archive)
		}
		return sum, nil
	}
	return nil, fmt.Errorf("%s absent du fichier de sommes de la release", archive)
}

// extractBinary renvoie le contenu du fichier name, à la racine de l'archive data (tar.gz ou zip selon
// le nom archive).
func extractBinary(data []byte, archive, name string) ([]byte, error) {
	var bin []byte
	var err error
	if strings.HasSuffix(archive, ".zip") {
		bin, err = extractZip(data, name)
	} else {
		bin, err = extractTarGz(data, name)
	}
	if err != nil {
		return nil, fmt.Errorf("extraction de %s : %w", archive, err)
	}
	if bin == nil {
		return nil, fmt.Errorf("extraction de %s : binaire %s absent de l'archive", archive, name)
	}
	if len(bin) == 0 {
		return nil, fmt.Errorf("extraction de %s : binaire %s vide", archive, name)
	}
	return bin, nil
}

// isRootEntry indique une entrée d'archive name à la racine (« watchoo » ou « ./watchoo »).
func isRootEntry(entry, name string) bool {
	return path.Clean(strings.TrimPrefix(entry, "./")) == name
}

func extractTarGz(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg || !isRootEntry(hdr.Name, name) {
			continue
		}
		return readLimited(tr)
	}
}

func extractZip(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if !f.Mode().IsRegular() || !isRootEntry(f.Name, name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return readLimited(rc)
	}
	return nil, nil
}

// readLimited lit r jusqu'à maxBinarySize octets.
func readLimited(r io.Reader) ([]byte, error) {
	bin, err := io.ReadAll(io.LimitReader(r, maxBinarySize+1))
	if err != nil {
		return nil, err
	}
	if len(bin) > maxBinarySize {
		return nil, fmt.Errorf("binaire trop gros (plus de %d Mo)", maxBinarySize>>20)
	}
	return bin, nil
}

// Replace remplace le fichier exe par bin, d'un bloc : bin est écrit dans un fichier temporaire du
// même répertoire, passé en 0755, puis renommé par-dessus exe (renommage atomique sur un même système
// de fichiers ; un processus déjà lancé garde l'ancien binaire). Sous Windows (goos), où un exécutable
// lancé ne peut pas être remplacé, exe est d'abord renommé en exe.old (un ancien .old est supprimé), et
// remis en place si le renommage du nouveau échoue. En cas d'erreur, le fichier temporaire est supprimé.
func Replace(exe string, bin []byte, goos string) (err error) {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".watchoo-update-*")
	if err != nil {
		return fmt.Errorf("%w : %s : %w", ErrNotWritable, dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("écriture du nouveau binaire : %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("écriture du nouveau binaire : %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("écriture du nouveau binaire : %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("droits du nouveau binaire : %w", err)
	}
	if goos != "windows" {
		if err := os.Rename(tmpName, exe); err != nil {
			return fmt.Errorf("remplacement de %s : %w", exe, err)
		}
		return nil
	}
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("suppression de %s : %w", old, err)
	}
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("mise de côté de %s : %w", exe, err)
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Rename(old, exe)
		return fmt.Errorf("remplacement de %s : %w", exe, err)
	}
	return nil
}
