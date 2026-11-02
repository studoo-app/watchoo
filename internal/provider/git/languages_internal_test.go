package git

// Classification of the files by language (1.1.0) and reading of the "ls-tree -r -l -z" output.

import (
	"slices"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider"
)

func TestClassifyLanguage(t *testing.T) {
	for file, want := range map[string]string{
		"main.go":                     "Go",
		"src/App.TSX":                 "TypeScript",
		"web/index.html":              "HTML",
		"db/schema.sql":               "SQL",
		"Dockerfile":                  "Dockerfile",
		"docker/Dockerfile.prod":      "Dockerfile",
		"Makefile":                    "Makefile",
		"README.md":                   "", // prose
		"config.yaml":                 "", // data
		"data.json":                   "",
		"logo.png":                    "", // unknown
		"sans-extension":              "",
		"node_modules/lib/index.js":   "", // excluded directories, wherever they are
		"front/node_modules/lib/x.js": "",
		"vendor/autoload.php":         "",
		"app/dist/bundle.js":          "",
		"app/build/out.js":            "",
		"target/Main.java":            "",
		"src/vendors/lib.js":          "JavaScript", // only the exact name is excluded
		"static/jquery.min.js":        "",
		"static/theme.min.css":        "",
		"package-lock.json":           "",
		"go.sum":                      "",
		"build.gradle":                "Groovy", // a file named like a directory is not excluded
	} {
		if got := classifyLanguage(file); got != want {
			t.Errorf("classifyLanguage(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestParseLanguages(t *testing.T) {
	const sha = "0123456789012345678901234567890123456789"
	raw := "100644 blob " + sha + "     300\tmain.go\x00" +
		"100755 blob " + sha + "     100\tutil.go\x00" +
		"100644 blob " + sha + "     400\tapp.js\x00" +
		"100644 blob " + sha + "     400\tindex.css\x00" + // tie: by name
		"100644 blob " + sha + "       0\tvide.py\x00" + // empty: no language
		"120000 blob " + sha + "      20\tlien.rb\x00" + // symbolic link
		"160000 commit " + sha + "       -\tsous-module.c\x00" + // submodule
		"100644 blob " + sha + "     999\tREADME.md\x00" +
		"illisible\x00"
	want := []provider.Language{{Name: "CSS", Bytes: 400}, {Name: "Go", Bytes: 400}, {Name: "JavaScript", Bytes: 400}}
	if got := parseLanguages([]byte(raw)); !slices.Equal(got, want) {
		t.Errorf("parseLanguages = %+v, want %+v", got, want)
	}
	if got := parseLanguages(nil); got == nil || len(got) != 0 {
		t.Errorf("parseLanguages(nil) = %#v, want an empty non-nil list", got)
	}
}
