package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/logo"
)

// wideTerminal est un terminal simulé de 80 colonnes, Unicode, en couleur.
var wideTerminal = TermInfo{IsTerminal: true, Width: 80, Unicode: true, Color: true}

// ansiSeq reconnaît une séquence d'échappement ANSI de couleur ou d'attribut.
var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// onTerminal fait passer la sortie standard du harness pour le terminal décrit par info, et renvoie un
// compteur des appels à Terminal.
func onTerminal(h *harness, info TermInfo) *int {
	calls := new(int)
	h.Terminal = func() TermInfo {
		*calls++
		return info
	}
	return calls
}

// cardFor est la carte d'en-tête de Watchoo attendue sur le terminal info.
func cardFor(info TermInfo, subtitle, tip string) string {
	return logo.Card(logo.CardOptions{Title: "Watchoo", Version: Version, Subtitle: subtitle, Tip: tip,
		Width: info.Width, Unicode: info.Unicode, Color: info.Color})
}

// statusFor est la ligne d'état attendue sur le terminal info.
func statusFor(info TermInfo, text string) string {
	return logo.Status(text, logo.CardOptions{Unicode: info.Unicode, Color: info.Color})
}

// helpHeader est l'en-tête attendu en haut de l'aide générale sur le terminal info : la carte, une ligne
// vide, la ligne d'état de l'espace, une ligne vide.
func helpHeader(info TermInfo, status, tip string) string {
	return cardFor(info, helpSubtitle, tip) + "\n" + statusFor(info, status) + "\n"
}

// versionLine est la sortie de « watchoo version » hors terminal.
func versionLine() string {
	return "watchoo " + Version + " (" + runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH + ")\n"
}

// versionCard est la sortie de « watchoo version » sur le terminal info.
func versionCard(info TermInfo) string {
	return cardFor(info, runtime.Version()+" · "+runtime.GOOS+"/"+runtime.GOARCH, "")
}

// assertNoCard vérifie qu'out ne contient ni carte, ni ligne d'état, ni séquence d'échappement.
func assertNoCard(t *testing.T, what, out string) {
	t.Helper()
	for _, part := range []string{"╭", "█", "●", "+--", `|\    /|`, "Astuce", "\x1b"} {
		if strings.Contains(out, part) {
			t.Errorf("%s : carte inattendue (%q) :\n%s", what, part, out)
			return
		}
	}
}

// generalHelp sont les façons d'afficher l'aide générale.
var generalHelp = [][]string{nil, {"help"}, {"--help"}, {"-h"}}

// Sur un terminal, l'aide générale commence par la carte (astuce de « watchoo init » s'il n'y a pas
// d'espace) et la ligne d'état, puis l'aide habituelle, inchangée.
func TestCardInGeneralHelp(t *testing.T) {
	plain := newHarness(t)
	h := newHarness(t)
	onTerminal(h, wideTerminal)
	header := helpHeader(wideTerminal, "Aucun espace de supervision ici", tipInit)
	for _, part := range []string{"Watchoo", "dev", "Qui a travaillé, sur quoi, et quand ?", "watchoo init", "╭", "╯"} {
		if !strings.Contains(ansiSeq.ReplaceAllString(header, ""), part) {
			t.Fatalf("en-tête sans %q :\n%s", part, header)
		}
	}
	for _, args := range generalHelp {
		want := plain.run(args...).mustSucceed(t).stdout
		r := h.run(args...).mustSucceed(t)
		if r.stdout != header+want {
			t.Errorf("watchoo %q sur un terminal :\n%s\nattendu la carte, la ligne d'état puis :\n%s", args, r.stdout, want)
		}
		if r.stderr != "" {
			t.Errorf("watchoo %q : sortie d'erreur inattendue :\n%s", args, r.stderr)
		}
	}
	// L'aide ne lance pas gh, même pour chercher l'espace.
	if len(h.configs) != 0 || h.fake().StatusCalls() != 0 {
		t.Error("l'aide a construit un accès à gh")
	}
}

// La ligne d'état de l'aide décrit l'espace trouvé comme le feraient les autres commandes.
func TestHelpWorkspaceStatus(t *testing.T) {
	isolateEnv(t)
	home := os.Getenv("HOME")
	root := newDir(t, home, "supervision", "licence-projet")
	(&harness{t: t, cwd: root, Client: authtest.NewLoggedIn("")}).run("init").mustSucceed(t)
	other := newDir(t, t.TempDir(), "master-projet")
	(&harness{t: t, cwd: other, Client: authtest.NewLoggedIn("")}).run("init").mustSucceed(t)
	empty := t.TempDir()
	requireNoWorkspaceAbove(t, empty)

	inHome := "Espace : licence-projet (" + filepath.Join("~", "supervision", "licence-projet") + ")"
	inOther := "Espace : master-projet (" + other + ")"
	for _, tt := range []struct {
		name string
		cwd  string
		args []string
		env  string // WATCHOO_DIR
		want string
		tip  string
	}{
		{"espace du répertoire courant, sous le répertoire personnel", root, nil, "", inHome, tipWorkspace},
		{"depuis un sous-répertoire", newDir(t, root, "src", "module"), []string{"help"}, "", inHome, tipWorkspace},
		{"hors d'un espace", empty, []string{"--help"}, "", "Aucun espace de supervision ici", tipInit},
		{"option -C", empty, []string{"-C", other, "--help"}, "", inOther, tipWorkspace},
		{"option -C relative", root, []string{"-C", "src", "help"}, "", inHome, tipWorkspace},
		{"variable WATCHOO_DIR", empty, nil, other, inOther, tipWorkspace},
		{"option -C sans espace", root, []string{"-C", empty}, "", "Aucun espace de supervision ici", tipInit},
		{"option -C inexistante", root, []string{"-C", "absent", "-h"}, "",
			"Espace : option -C: directory " + filepath.Join(root, "absent") + " does not exist", tipHelp},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("WATCHOO_DIR", tt.env)
			}
			h := &harness{t: t, cwd: tt.cwd, Client: authtest.NewLoggedIn("")}
			onTerminal(h, wideTerminal)
			r := h.run(tt.args...).mustSucceed(t)
			if want := helpHeader(wideTerminal, tt.want, tt.tip); !strings.HasPrefix(r.stdout, want) {
				t.Errorf("aide :\n%s\nattendu l'en-tête :\n%s", r.stdout, want)
			}
			if len(h.configs) != 0 {
				t.Error("l'aide a construit un accès à gh")
			}
		})
	}
}

// Des réglages invalides, un nom d'espace avec des caractères de contrôle ou un répertoire courant
// inaccessible ne font jamais échouer l'aide.
func TestHelpWorkspaceStatusErrors(t *testing.T) {
	h := newHarness(t)
	h.run("init").mustSucceed(t)
	config := filepath.Join(h.cwd, ".watchoo", "config.yaml")
	onTerminal(h, wideTerminal)
	plainHelp := newHarness(t).run("--help").mustSucceed(t).stdout

	// statusOf renvoie la ligne d'état de l'aide, sans couleur, et vérifie que l'aide habituelle suit.
	statusOf := func(r result) string {
		t.Helper()
		_, rest, ok := strings.Cut(ansiSeq.ReplaceAllString(r.stdout, ""), "╯\n\n") // après la carte
		if !ok {
			t.Fatalf("aide sans carte :\n%s", r.stdout)
		}
		line, help, _ := strings.Cut(rest, "\n\n")
		if help != plainHelp {
			t.Errorf("aide habituelle absente ou modifiée :\n%s", r.stdout)
		}
		return line
	}

	writeFile(t, config, "sync:\n  intervall: 1h\n")
	r := h.run("--help").mustSucceed(t)
	if got := statusOf(r); !strings.HasPrefix(got, "● Espace : ") || !strings.Contains(got, "intervall") {
		t.Errorf(`invalid settings : status line %q`, got)
	}
	if r.stderr != "" {
		t.Errorf("sortie d'erreur inattendue :\n%s", r.stderr)
	}

	writeFile(t, config, "sync:\n  interval: 5m\n")
	if got := statusOf(h.run("help").mustSucceed(t)); !strings.HasPrefix(got, "● Espace : invalid settings: ") {
		t.Errorf("valeur hors limites : ligne d'état %q", got)
	}

	// Nom avec une séquence d'échappement et un saut de ligne (YAML « \e », « \n ») : neutralisés.
	writeFile(t, config, `name: "promo\e[2J\nlicence"`+"\n")
	r = h.run().mustSucceed(t)
	if got := statusOf(r); got != "● Espace : promo?[2J licence ("+h.cwd+")" {
		t.Errorf("nom avec caractères de contrôle : ligne d'état %q", got)
	}
	if rest := ansiSeq.ReplaceAllString(r.stdout, ""); strings.Contains(rest, "\x1b") {
		t.Errorf("séquence d'échappement venue du nom de l'espace :\n%q", r.stdout)
	}

	// Répertoire courant inaccessible : signalé, sauf avec un -C absolu.
	deps := Deps{Getwd: func() (string, error) { return "", errors.New("répertoire supprimé") }}
	status, tip := helpStatus(deps, &options{})
	if status != "Espace : répertoire courant inaccessible : répertoire supprimé" || tip != tipHelp {
		t.Errorf("répertoire courant inaccessible : %q, %q", status, tip)
	}
	writeFile(t, config, "name: perso\n")
	if status, _ := helpStatus(deps, &options{workspaceDir: h.cwd}); status != "Espace : perso ("+h.cwd+")" {
		t.Errorf("répertoire courant inaccessible, -C absolu : %q", status)
	}
}

// Sur un terminal, « watchoo version » et « watchoo --version » affichent la carte et rien d'autre.
func TestCardVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	h := newHarness(t)
	onTerminal(h, wideTerminal)
	for _, tt := range []struct{ version, shown string }{{"dev", "Watchoo dev"}, {"0.1.0", "Watchoo v0.1.0"},
		{"v1.2.3", "Watchoo v1.2.3"}} {
		Version = tt.version
		want := versionCard(wideTerminal)
		plain := ansiSeq.ReplaceAllString(want, "")
		if !strings.Contains(plain, tt.shown) || !strings.Contains(plain, runtime.Version()+" · "+runtime.GOOS) ||
			strings.Contains(plain, "Astuce") {
			t.Fatalf("carte de version inattendue :\n%s", plain)
		}
		for _, args := range [][]string{{"version"}, {"--version"}, {"-C", "ailleurs", "--version"}} {
			r := h.run(args...).mustSucceed(t)
			if r.stdout != want || r.stderr != "" {
				t.Errorf("watchoo %q sur un terminal = %q (stderr %q), attendu la carte :\n%s", args, r.stdout,
					r.stderr, want)
			}
		}
	}
	if len(h.configs) != 0 {
		t.Error("version a construit un accès à gh")
	}
}

// Sur un terminal, le compte rendu de « watchoo init » est la carte, puis des lignes d'état.
func TestCardInit(t *testing.T) {
	h := newHarness(t)
	root := h.cwd
	onTerminal(h, wideTerminal)
	r := h.run("init").mustSucceed(t)
	want := cardFor(wideTerminal, "Espace de supervision « licence-projet » créé", tipInitDone) + "\n" +
		statusFor(wideTerminal, "Espace : licence-projet ("+root+")") +
		statusFor(wideTerminal, "Réglages : "+filepath.Join(root, ".watchoo", "config.yaml")) +
		statusFor(wideTerminal, "Base : "+filepath.Join(root, ".watchoo", "watchoo.db")) +
		statusFor(wideTerminal, "Compte gh : watchoo-test sur github.com (droits : gist, read:org, repo)")
	if r.stdout != want {
		t.Errorf("init sur un terminal :\n%s\nattendu :\n%s", r.stdout, want)
	}
	if !strings.Contains(ansiSeq.ReplaceAllString(r.stdout, ""), tipInitDone) {
		t.Errorf("astuce absente :\n%s", r.stdout)
	}
	if r.stderr != "" {
		t.Errorf("sortie d'erreur inattendue :\n%s", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".watchoo", "watchoo.db")); err != nil {
		t.Errorf("base absente : %v", err)
	}

	// Sous le répertoire personnel : « ~ » dans les chemins ; sans droits indiqués par gh.
	h2 := newHarness(t)
	h2.cwd = newDir(t, os.Getenv("HOME"), "perso")
	h2.fake().SetStatus(auth.Status{Host: "github.com", Login: "ci-bot", TokenSource: "GH_TOKEN", Active: true,
		State: "success"}, nil)
	info := TermInfo{IsTerminal: true, Width: 80, Unicode: true}
	onTerminal(h2, info)
	r = h2.run("init").mustSucceed(t)
	_, report, _ := strings.Cut(r.stdout, "╯\n\n")
	wantReport := "● Espace : perso (" + filepath.Join("~", "perso") + ")\n" +
		"● Réglages : " + filepath.Join("~", "perso", ".watchoo", "config.yaml") + "\n" +
		"● Base : " + filepath.Join("~", "perso", ".watchoo", "watchoo.db") + "\n" +
		"● Compte gh : ci-bot sur github.com (droits non indiqués par gh)\n"
	if report != wantReport {
		t.Errorf("compte rendu :\n%s\nattendu :\n%s", report, wantReport)
	}
}

// Un espace dont le nom a des accents décomposés (« e » suivi de U+0301), comme un répertoire créé par le
// Finder de macOS, ne décale pas la bordure de droite de la carte de « watchoo init » ; sans Unicode,
// ses accents sont retirés, et non remplacés par « ? », dans la carte et les lignes d'état de init et de
// l'aide.
func TestCardDecomposedWorkspaceName(t *testing.T) {
	const name = "promo-re\u0301seaux"
	for _, unicodeOK := range []bool{true, false} {
		h := newHarness(t)
		h.cwd = newDir(t, filepath.Dir(h.cwd), name)
		onTerminal(h, TermInfo{IsTerminal: true, Width: 80, Unicode: unicodeOK, Color: true})
		initOut := ansiSeq.ReplaceAllString(h.run("init").mustSucceed(t).stdout, "")
		helpOut := ansiSeq.ReplaceAllString(h.run("--help").mustSucceed(t).stdout, "")

		card, report, ok := strings.Cut(initOut, "\n\n")
		if !ok {
			t.Fatalf("Unicode %t : init sans carte :\n%s", unicodeOK, initOut)
		}
		lines := strings.Split(card, "\n")
		for i, line := range lines {
			if got, want := columns(line), columns(lines[0]); got != want {
				t.Errorf("Unicode %t : ligne %d de la carte sur %d colonnes, attendu %d comme la première :\n%s",
					unicodeOK, i+1, got, want, card)
			}
		}
		shown, status := "« "+name+" »", "● Espace : "+name+" ("+h.cwd+")"
		if !unicodeOK {
			// Toute la ligne d'état est ramenée à l'ASCII, chemin compris.
			shown = `"promo-reseaux"`
			status = "* Espace : promo-reseaux (" + strings.ReplaceAll(h.cwd, name, "promo-reseaux") + ")"
		}
		if !strings.Contains(card, shown) {
			t.Errorf("Unicode %t : %s absent de la carte :\n%s", unicodeOK, shown, card)
		}
		if !strings.HasPrefix(report, status+"\n") {
			t.Errorf("Unicode %t : compte rendu\n%s\nattendu en tête %q", unicodeOK, report, status)
		}
		header, _, ok := strings.Cut(helpOut, "\n\n"+status+"\n\n")
		if !ok {
			t.Errorf("Unicode %t : aide sans la ligne d'état %q :\n%s", unicodeOK, status, helpOut)
		}
		if !unicodeOK && (!isASCII(initOut) || !isASCII(header) || strings.Contains(initOut, "re?s")) {
			t.Errorf("sans Unicode : accents mal retirés :\n%s\n%s", initOut, header)
		}
	}
}

// columns renvoie la largeur affichée d'une ligne sans séquence ANSI : son nombre de caractères, sans
// les marques combinantes (Mn, Me), que le terminal dessine dans la colonne du caractère précédent.
func columns(line string) int {
	n := 0
	for _, r := range line {
		if !unicode.In(r, unicode.Mn, unicode.Me) {
			n++
		}
	}
	return n
}

// Hors terminal (sortie redirigée, tube), aucune sortie n'a de carte : elles restent identiques.
func TestNoCardOffTerminal(t *testing.T) {
	h := newHarness(t) // Terminal par défaut : DetectTerminal du tampon de sortie
	plainHelp := h.run("--help").mustSucceed(t).stdout
	calls := 0
	for _, info := range []func() TermInfo{nil, func() TermInfo {
		calls++
		return TermInfo{Width: 80, Unicode: true, Color: true} // pas un terminal
	}} {
		h.Terminal = info
		for _, args := range generalHelp {
			if r := h.run(args...).mustSucceed(t); r.stdout != plainHelp {
				t.Errorf("watchoo %q hors terminal :\n%s", args, r.stdout)
			}
		}
		for _, args := range [][]string{{"version"}, {"--version"}} {
			if r := h.run(args...).mustSucceed(t); r.stdout != versionLine() || r.stderr != "" {
				t.Errorf("watchoo %q hors terminal = %q (stderr %q), attendu %q", args, r.stdout, r.stderr,
					versionLine())
			}
		}
	}
	if calls == 0 {
		t.Error("Terminal n'a pas été consulté")
	}
	assertNoCard(t, "aide", plainHelp)

	h2 := newHarness(t)
	h2.Terminal = func() TermInfo { return TermInfo{} }
	r := h2.run("init").mustSucceed(t)
	if !strings.HasPrefix(r.stdout, "Espace de supervision « licence-projet » créé.\n  répertoire  "+h2.cwd+"\n") {
		t.Errorf("init hors terminal : sortie inattendue :\n%s", r.stdout)
	}
	assertNoCard(t, "init", r.stdout)
}

// Avec --json, jamais de carte, même sur un terminal : la sortie reste du JSON pur. Le terminal n'est
// même pas consulté.
func TestNoCardWithJSON(t *testing.T) {
	h := newHarness(t)
	plainHelp := newHarness(t).run("--help").mustSucceed(t).stdout
	calls := onTerminal(h, wideTerminal)
	for _, args := range [][]string{{"version", "--json"}, {"--json", "version"}, {"--version", "--json"},
		{"--json", "--version"}} {
		r := h.run(args...).mustSucceed(t)
		var got versionReport
		if err := json.Unmarshal([]byte(r.stdout), &got); err != nil || got.Version != Version {
			t.Errorf("watchoo %q : sortie JSON invalide (%v) :\n%s", args, err, r.stdout)
		}
	}
	for _, args := range [][]string{{"--json"}, {"--help", "--json"}, {"help", "--json"}} {
		if r := h.run(args...).mustSucceed(t); r.stdout != plainHelp {
			t.Errorf("watchoo %q : attendu l'aide sans en-tête :\n%s", args, r.stdout)
		}
	}
	r := h.run("init", "--json").mustSucceed(t)
	if report := decodeInitReport(t, r.stdout); report.Name != "licence-projet" {
		t.Errorf("init --json : rapport inattendu : %+v", report)
	}
	if *calls != 0 {
		t.Errorf("Terminal consulté %d fois avec --json", *calls)
	}
}

// La sortie d'erreur (erreurs, avertissements) n'a jamais de carte, et une commande qui échoue n'affiche
// rien sur la sortie standard, même sur un terminal.
func TestNoCardOnStderr(t *testing.T) {
	h := newHarness(t)
	onTerminal(h, wideTerminal)
	for _, args := range [][]string{{"version", "extra"}, {"--version", "extra"}, {"inti"}, {"--inconnue"},
		{"help", "inconnue"}} {
		r := h.run(args...).mustFail(t)
		assertNoCard(t, strings.Join(args, " ")+" (stderr)", r.stderr)
	}

	// Avertissement d'un espace imbriqué : sur la sortie d'erreur, sans carte ; la carte reste en haut du
	// compte rendu, sur la sortie standard.
	parent := h.cwd
	h.run("init").mustSucceed(t)
	h.cwd = newDir(t, parent, "groupe-a")
	r := h.run("init").mustSucceed(t)
	if !strings.HasPrefix(r.stderr, "attention : ") {
		t.Errorf("avertissement attendu sur la sortie d'erreur :\n%s", r.stderr)
	}
	assertNoCard(t, "init imbriqué (stderr)", r.stderr)
	if want := cardFor(wideTerminal, "Espace de supervision « groupe-a » créé", tipInitDone); !strings.HasPrefix(r.stdout, want) {
		t.Errorf("init imbriqué : carte absente du compte rendu :\n%s", r.stdout)
	}

	// gh non connecté et git absent : erreur seulement.
	h2 := newHarness(t)
	h2.Client = authtest.NewLoggedOut()
	h2.GitMissing = true
	onTerminal(h2, wideTerminal)
	r = h2.run("init").mustFail(t, "gh auth login")
	assertNoCard(t, "init sans gh connecté (stderr)", r.stderr)
}

// L'aide d'une sous-commande n'a pas de carte, même sur un terminal.
func TestNoCardInSubcommandHelp(t *testing.T) {
	plain := newHarness(t)
	h := newHarness(t)
	onTerminal(h, wideTerminal)
	for _, args := range [][]string{{"help", "init"}, {"init", "--help"}, {"init", "-h"}, {"version", "--help"},
		{"help", "version"}, {"help", "help"}} {
		want := plain.run(args...).mustSucceed(t).stdout
		r := h.run(args...).mustSucceed(t)
		if r.stdout != want {
			t.Errorf("watchoo %q sur un terminal :\n%s\nattendu, comme hors terminal :\n%s", args, r.stdout, want)
		}
		assertNoCard(t, strings.Join(args, " "), r.stdout)
	}
}

// La carte s'adapte au terminal décrit par l'environnement : largeur (COLUMNS), locale (Unicode ou ASCII
// pur), NO_COLOR et TERM=dumb (sans couleur). Le terminal est simulé, avec une largeur système inconnue.
func TestCardAdaptsToTerminal(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		want TermInfo // IsTerminal false : pas de carte (terminal trop étroit)
	}{
		{"locale UTF-8, largeur inconnue", map[string]string{"LANG": "fr_FR.UTF-8", "TERM": "xterm-256color"},
			TermInfo{IsTerminal: true, Unicode: true, Color: true}},
		{"50 colonnes", map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "50"},
			TermInfo{IsTerminal: true, Width: 50, Unicode: true, Color: true}},
		{"40 colonnes", map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "40"},
			TermInfo{IsTerminal: true, Width: 40, Unicode: true, Color: true}},
		{"30 colonnes", map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "30"}, TermInfo{}},
		{"locale C", map[string]string{"LANG": "C", "LC_ALL": "C"}, TermInfo{IsTerminal: true, Color: true}},
		{"locale C, 50 colonnes", map[string]string{"LC_ALL": "C", "COLUMNS": "50"},
			TermInfo{IsTerminal: true, Width: 50, Color: true}},
		{"locale non UTF-8", map[string]string{"LANG": "fr_FR.ISO-8859-1"}, TermInfo{IsTerminal: true, Color: true}},
		{"Windows Terminal", map[string]string{"WT_SESSION": "c2a3f0e4"},
			TermInfo{IsTerminal: true, Unicode: true, Color: true}},
		{"NO_COLOR", map[string]string{"LANG": "fr_FR.UTF-8", "NO_COLOR": "1"},
			TermInfo{IsTerminal: true, Unicode: true}},
		{"TERM=dumb", map[string]string{"LANG": "fr_FR.UTF-8", "TERM": "dumb"}, TermInfo{IsTerminal: true, Unicode: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			plainHelp := newHarness(t).run("--help").mustSucceed(t).stdout
			setTerminalEnv(t, tt.env)
			h.Terminal = func() TermInfo { return describeTerminal(os.LookupEnv, func() int { return 0 }) }

			version := h.run("version").mustSucceed(t).stdout
			help := h.run("--help").mustSucceed(t).stdout
			initOut := h.run("init").mustSucceed(t).stdout
			if !tt.want.IsTerminal { // trop étroit pour la carte : sorties habituelles
				if version != versionLine() || help != plainHelp ||
					!strings.HasPrefix(initOut, "Espace de supervision « licence-projet » créé.\n  répertoire  "+h.cwd+"\n") {
					t.Errorf("terminal trop étroit : sorties modifiées :\n%s\n%s\n%s", version, help, initOut)
				}
				assertNoCard(t, "init", initOut)
				return
			}
			info := tt.want
			if want := versionCard(info); version != want {
				t.Errorf("version :\n%s\nattendu :\n%s", version, want)
			}
			header := helpHeader(info, "Aucun espace de supervision ici", tipInit)
			if help != header+plainHelp {
				t.Errorf("aide :\n%s\nattendu l'en-tête :\n%s", help, header)
			}
			limit := tt.want.Width
			if limit == 0 {
				limit = 80
			}
			for _, out := range []string{version, header, initOut} {
				// Les lignes de la carte tiennent dans le terminal (pas forcément les lignes d'état, qui
				// citent des chemins).
				for line := range strings.Lines(ansiSeq.ReplaceAllString(out, "")) {
					line = strings.TrimSuffix(line, "\n")
					if n := utf8.RuneCountInString(line); n > limit && !strings.HasPrefix(line, "● ") &&
						!strings.HasPrefix(line, "* ") {
						t.Errorf("ligne de %d colonnes sur un terminal de %d : %q", n, limit, line)
					}
				}
				if !info.Unicode && !isASCII(out) {
					t.Errorf("sortie pas en ASCII pur sans Unicode :\n%s", out)
				}
				if !info.Color && strings.Contains(out, "\x1b") {
					t.Errorf("séquence d'échappement sans couleur :\n%q", out)
				}
				if info.Color && !strings.Contains(out, "\x1b[") {
					t.Errorf("sortie sans couleur :\n%s", out)
				}
			}
			if !info.Unicode && (!strings.Contains(version, "+--") || !strings.Contains(ansiSeq.ReplaceAllString(version, ""), "| o##o |")) {
				t.Errorf("version sans la carte en ASCII pur :\n%s", version)
			}
		})
	}
}

// isASCII indique si s ne contient que des caractères ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func TestVersionFlag(t *testing.T) {
	h := newHarness(t)
	version := h.run("version").mustSucceed(t)
	for _, args := range [][]string{{"--version"}, {"-C", "ailleurs", "--version"}} {
		if r := h.run(args...).mustSucceed(t); r != version {
			t.Errorf("watchoo %q = %+v, attendu comme « watchoo version » : %+v", args, r, version)
		}
	}
	versionJSON := h.run("version", "--json").mustSucceed(t)
	if r := h.run("--version", "--json").mustSucceed(t); r != versionJSON {
		t.Errorf("watchoo --version --json = %+v, attendu %+v", r, versionJSON)
	}

	h.run("--version", "extra").mustFail(t, "« watchoo --version » ne prend pas d'argument (reçu : extra)")
	// Option de la commande racine seulement, sans lettre courte.
	h.run("init", "--version").mustFail(t, "option inconnue : --version (aide : « watchoo init --help »)")
	h.run("-v").mustFail(t, "option courte inconnue : -v")
	assertNoWorkspace(t, h.cwd)
	if len(h.configs) != 0 {
		t.Error("--version a construit un accès à gh")
	}

	// Documentée dans l'aide générale, en français ; absente de l'aide des sous-commandes.
	if r := h.run("--help").mustSucceed(t); !strings.Contains(r.stdout, "--version") ||
		!strings.Contains(r.stdout, "affiche la version de Watchoo") {
		t.Errorf("aide générale sans --version :\n%s", r.stdout)
	}
	if r := h.run("help", "init").mustSucceed(t); strings.Contains(r.stdout, "--version") {
		t.Errorf("aide de init avec --version :\n%s", r.stdout)
	}
}
