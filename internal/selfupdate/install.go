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

	"github.com/studoo-app/watchoo/internal/i18n"
)

// i18nError est une erreur sentinelle dont le message vient du catalogue i18n : le texte est traduit
// au moment de l'affichage (Error), dans la langue du processus. La valeur reste comparable avec ==
// (une structure d'un champ) : errors.Is retrouve l'erreur sentinelle à travers les emballages.
type i18nError struct{ key string }

func (e i18nError) Error() string { return i18n.T(e.key) }

// sep renvoie le séparateur « : » de la langue du processus (espace avant en français).
func sep() string { return i18n.T("selfupdate.err.sep") }

// wrapErr compose un texte traduit et une erreur enveloppée, avec la ponctuation de la langue.
func wrapErr(text string, err error) error {
	return fmt.Errorf("%s%s%w", text, sep(), err)
}

// notWritable renvoie l'erreur ErrNotWritable du répertoire dir, cause err.
func notWritable(dir string, err error) error {
	return fmt.Errorf("%w%s%s%s%w", ErrNotWritable, sep(), dir, sep(), err)
}

// Erreurs de refus de la mise à jour (CheckInstallable, CheckWritable) ; la CLI les traduit en
// messages qui disent quoi faire.
var (
	// ErrDocker : Watchoo tourne dans l'image Docker, mise à jour en changeant de tag.
	ErrDocker error = i18nError{"selfupdate.err.docker"}
	// ErrDevBuild : la version installée n'est pas une version de release (compilée depuis les sources).
	ErrDevBuild error = i18nError{"selfupdate.err.dev_build"}
	// ErrNotWritable : le répertoire du binaire n'est pas accessible en écriture.
	ErrNotWritable error = i18nError{"selfupdate.err.not_writable"}
	// ErrChecksum : la somme SHA-256 de l'archive ne correspond pas au fichier de sommes.
	ErrChecksum error = i18nError{"selfupdate.err.checksum"}
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
		return fmt.Errorf("%w%s%s", ErrDevBuild, sep(), current)
	}
	return nil
}

// CheckWritable vérifie qu'un fichier peut être créé dans dir (répertoire du binaire), en y créant
// puis supprimant un fichier temporaire. Erreur enveloppant ErrNotWritable sinon.
func CheckWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".watchoo-update-check-*")
	if err != nil {
		return notWritable(dir, err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return notWritable(dir, err)
	}
	return nil
}

// Executable renvoie le chemin réel du binaire en cours : os.Executable, liens symboliques résolus
// (le binaire remplacé est la cible du lien, pas le lien).
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", wrapErr(i18n.T("selfupdate.exe.not_found"), err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", wrapErr(i18n.T("selfupdate.exe.not_found"), err)
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
		return nil, errors.New(i18n.T("selfupdate.version.invalid", tag))
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
		return nil, wrapErr(archive, ErrChecksum)
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
		return nil, wrapErr(i18n.T("selfupdate.download", name), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(i18n.T("selfupdate.download.http", name, resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, wrapErr(i18n.T("selfupdate.download", name), err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New(i18n.T("selfupdate.download.too_big", name, limit>>20))
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
			return nil, errors.New(i18n.T("selfupdate.checksum.unreadable", archive))
		}
		return sum, nil
	}
	return nil, errors.New(i18n.T("selfupdate.checksum.missing", archive))
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
		return nil, wrapErr(i18n.T("selfupdate.extract", archive), err)
	}
	if bin == nil {
		return nil, errors.New(i18n.T("selfupdate.extract.missing", archive, name))
	}
	if len(bin) == 0 {
		return nil, errors.New(i18n.T("selfupdate.extract.empty", archive, name))
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
		return nil, errors.New(i18n.T("selfupdate.binary.too_big", maxBinarySize>>20))
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
		return notWritable(dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(bin); err != nil {
		_ = tmp.Close()
		return wrapErr(i18n.T("selfupdate.replace.write"), err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return wrapErr(i18n.T("selfupdate.replace.write"), err)
	}
	if err := tmp.Close(); err != nil {
		return wrapErr(i18n.T("selfupdate.replace.write"), err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return wrapErr(i18n.T("selfupdate.replace.perms"), err)
	}
	if goos != "windows" {
		if err := os.Rename(tmpName, exe); err != nil {
			return wrapErr(i18n.T("selfupdate.replace.rename", exe), err)
		}
		return nil
	}
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return wrapErr(i18n.T("selfupdate.replace.remove", old), err)
	}
	if err := os.Rename(exe, old); err != nil {
		return wrapErr(i18n.T("selfupdate.replace.aside", exe), err)
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Rename(old, exe)
		return wrapErr(i18n.T("selfupdate.replace.rename", exe), err)
	}
	return nil
}
