package auth

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseStatus(t *testing.T) {
	const host = "github.com"
	tests := []struct {
		name    string
		stdout  string
		user    string
		want    Status
		wantErr error
		notErr  error    // erreur que l'erreur renvoyée ne doit pas envelopper
		msg     []string // extraits attendus dans le message d'erreur
		noMsg   []string // extraits interdits dans le message d'erreur
	}{
		{
			name:   "compte actif",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"gist, read:org, repo","gitProtocol":"ssh"}]}}`,
			want:   Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "success", Scopes: []string{"gist", "read:org", "repo"}},
		},
		{
			name: "compte actif parmi plusieurs",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":false,"host":"github.com","login":"bob","tokenSource":"keyring","scopes":"repo"},
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"}]}}`,
			want: Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "success", Scopes: []string{"repo"}},
		},
		{
			name: "compte choisi par github.user",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"},
				{"state":"success","active":false,"host":"github.com","login":"Bob","tokenSource":"oauth_token","scopes":"repo, read:org"}]}}`,
			user: "Bob",
			want: Status{Host: host, Login: "Bob", TokenSource: "oauth_token", State: "success", Scopes: []string{"repo", "read:org"}},
		},
		{
			// « gh auth token --user bob » échouerait : gh distingue les majuscules.
			name: "github.user avec une autre casse",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"},
				{"state":"success","active":false,"host":"github.com","login":"Bob","tokenSource":"oauth_token","scopes":"repo"}]}}`,
			user:    "bob",
			wantErr: ErrUnusableUser,
			msg:     []string{`github.user vaut "bob"`, `sous le nom "Bob"`, "majuscules", "corrigez le réglage github.user"},
			noMsg:   []string{"gh auth login"},
		},
		{
			name: "github.user préfère le compte enregistré à GH_TOKEN",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"},
				{"state":"success","active":false,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"}]}}`,
			user: "alice",
			want: Status{Host: host, Login: "alice", TokenSource: "keyring", State: "success", Scopes: []string{"repo"}},
		},
		{
			// « gh auth token --user alice » ignore GH_TOKEN et échouerait (vérifié avec gh 2.103).
			name:    "github.user ne correspond qu'à GH_TOKEN",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"}]}}`,
			user:    "alice",
			wantErr: ErrUnusableUser,
			msg:     []string{`github.user vaut "alice"`, "que par la variable GH_TOKEN", "videz-le"},
			noMsg:   []string{"gh auth login"},
		},
		{
			name:    "github.user ne correspond qu'à GH_TOKEN, avec une autre casse",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"Alice","tokenSource":"GH_TOKEN"}]}}`,
			user:    "alice",
			wantErr: ErrUnusableUser,
			msg:     []string{"que par la variable GH_TOKEN"},
		},
		{
			name: "github.user : la casse d'un compte enregistré l'emporte sur GH_TOKEN",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"},
				{"state":"success","active":false,"host":"github.com","login":"Alice","tokenSource":"keyring"}]}}`,
			user:    "alice",
			wantErr: ErrUnusableUser,
			msg:     []string{`sous le nom "Alice"`},
		},
		{
			name:   "compte actif issu de GH_TOKEN, sans github.user",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"ci-bot","tokenSource":"GH_TOKEN"}]}}`,
			want:   Status{Host: host, Login: "ci-bot", TokenSource: "GH_TOKEN", Active: true, State: "success", Scopes: []string{}},
			notErr: ErrNotLoggedIn,
		},
		{
			name:    "github.user inconnu",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring"}]}}`,
			user:    "carol",
			wantErr: ErrNotLoggedIn,
			notErr:  ErrUnusableUser,
			msg:     []string{`"carol"`, "github.user", "gh auth login"},
		},
		{
			name:    "aucun compte actif",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":false,"host":"github.com","login":"alice","tokenSource":"keyring"}]}}`,
			wantErr: ErrNotLoggedIn,
		},
		{
			name:    "aucun compte (réponse de gh non connecté)",
			stdout:  `{"hosts":{}}`,
			wantErr: ErrNotLoggedIn,
		},
		{
			name:    "comptes d'un autre hôte seulement",
			stdout:  `{"hosts":{"ghe.example.test":[{"state":"success","active":true,"host":"ghe.example.test","login":"alice"}]}}`,
			wantErr: ErrNotLoggedIn,
		},
		{
			name:   "clé d'hôte en majuscules",
			stdout: `{"hosts":{"GitHub.com":[{"state":"success","active":true,"host":"GitHub.com","login":"alice","tokenSource":"keyring","scopes":""}]}}`,
			want:   Status{Host: "GitHub.com", Login: "alice", TokenSource: "keyring", Active: true, State: "success", Scopes: []string{}},
		},
		{
			name:   "hôte absent de l'entrée",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"login":"alice"}]}}`,
			want:   Status{Host: host, Login: "alice", Active: true, State: "success", Scopes: []string{}},
		},
		{
			name:   "scopes sous forme de liste",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"login":"alice","scopes":[" repo ","gist",""]}]}}`,
			want:   Status{Host: host, Login: "alice", Active: true, State: "success", Scopes: []string{"repo", "gist"}},
		},
		{
			name:   "scopes d'un type inattendu",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"login":"alice","scopes":42}]}}`,
			want:   Status{Host: host, Login: "alice", Active: true, State: "success", Scopes: []string{}},
		},
		{
			name:    "token invalide",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"HTTP 401: Bad credentials (https://api.github.com/)","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"}]}}`,
			want:    Status{Host: host, Login: "alice", TokenSource: "GH_TOKEN", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
			msg:     []string{`"alice"`, "n'est pas utilisable", "gh auth login"},
			noMsg:   []string{"Bad credentials"},
		},
		{
			name:    "token refusé, message sans code HTTP",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"bad credentials","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
		},
		{
			// Format réel de gh 2.103, sans le texte « Bad credentials ».
			name:    "token refusé, format « status code: 401 » de gh",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"non-200 OK status code: 401 Unauthorized body: \"\"","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
		},
		{
			name:    "état error sans détail",
			stdout:  `{"hosts":{"github.com":[{"state":"error","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
		},
		{
			name:    "état inconnu",
			stdout:  `{"hosts":{"github.com":[{"state":"expired","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "expired", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
		},
		{
			name:    "github.com injoignable (délai)",
			stdout:  `{"hosts":{"github.com":[{"state":"timeout","error":"net/http: request canceled (Client.Timeout exceeded)","active":true,"host":"github.com","login":"alice","tokenSource":"keyring"}]}}`,
			want:    Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "timeout", Scopes: []string{}},
			wantErr: ErrCheckFailed,
			notErr:  ErrNotLoggedIn,
			msg:     []string{"n'a pas pu être vérifié", "« timeout »", "Client.Timeout exceeded", "réseau"},
			noMsg:   []string{"gh auth login"},
		},
		{
			// Réponse réelle de gh 2.103 avec HTTPS_PROXY=http://127.0.0.1:9 et un token valide.
			name:    "github.com injoignable (proxy)",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"Get \"https://api.github.com/\": proxyconnect tcp: dial tcp 127.0.0.1:9: connect: connection refused","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"}]}}`,
			want:    Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "error", Scopes: []string{"repo"}},
			wantErr: ErrCheckFailed,
			notErr:  ErrNotLoggedIn,
			msg:     []string{"proxyconnect tcp", "connection refused"},
			noMsg:   []string{"gh auth login"},
		},
		{
			name:    "erreur de gh sur plusieurs lignes, avec un token",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"HTTP 502: Bad Gateway gho_secret123\nsuite","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrCheckFailed,
			msg:     []string{"HTTP 502: Bad Gateway " + redacted},
			noMsg:   []string{"gho_secret123", "suite"},
		},
		{name: "sortie vide", stdout: ``, wantErr: ErrUnexpectedOutput},
		{name: "pas du JSON", stdout: `github.com: Logged in`, wantErr: ErrUnexpectedOutput},
		{name: "objet sans hosts", stdout: `{}`, wantErr: ErrUnexpectedOutput},
		{name: "hosts nul", stdout: `{"hosts":null}`, wantErr: ErrUnexpectedOutput},
		{name: "hosts d'un mauvais type", stdout: `{"hosts":[]}`, wantErr: ErrUnexpectedOutput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatus([]byte(tt.stdout), host, tt.user)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("erreur = %v, veut %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("erreur inattendue : %v", err)
			}
			if tt.notErr != nil && errors.Is(err, tt.notErr) {
				t.Errorf("l'erreur %q ne devrait pas envelopper %q", err, tt.notErr)
			}
			for _, part := range tt.msg {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("message %v sans %q", err, part)
				}
			}
			for _, part := range tt.noMsg {
				if err != nil && strings.Contains(err.Error(), part) {
					t.Errorf("message %q avec %q", err, part)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Status = %#v, veut %#v", got, tt.want)
			}
		})
	}
}

// ErrUnusableUser enveloppe ErrNotLoggedIn, comme l'erreur que Token renverrait dans ce cas, mais son
// message ne conseille pas « gh auth login », qui ne réglerait rien.
func TestErrUnusableUserWrapsNotLoggedIn(t *testing.T) {
	if !errors.Is(ErrUnusableUser, ErrNotLoggedIn) {
		t.Error("ErrUnusableUser n'enveloppe pas ErrNotLoggedIn")
	}
	if errors.Is(ErrNotLoggedIn, ErrUnusableUser) {
		t.Error("ErrNotLoggedIn ne doit pas être ErrUnusableUser")
	}
	if msg := ErrUnusableUser.Error(); strings.Contains(msg, "gh auth login") || !strings.Contains(msg, "github.user") {
		t.Errorf("message de ErrUnusableUser = %q", msg)
	}
}

func TestParseStatusErrorDoesNotQuoteStdout(t *testing.T) {
	stdout := `{"hosts": oups gho_secret_dans_la_sortie`
	_, err := parseStatus([]byte(stdout), "github.com", "")
	if !errors.Is(err, ErrUnexpectedOutput) {
		t.Fatalf("erreur = %v, veut ErrUnexpectedOutput", err)
	}
	if strings.Contains(err.Error(), "oups") || strings.Contains(err.Error(), "gho_") {
		t.Errorf("le message %q recopie la sortie standard de gh", err)
	}
}

func TestSplitScopes(t *testing.T) {
	tests := map[string][]string{
		"":                       {},
		"   ":                    {},
		"repo":                   {"repo"},
		"gist, read:org, repo":   {"gist", "read:org", "repo"},
		" gist ,,read:org , ":    {"gist", "read:org"},
		"admin:org,workflow,gis": {"admin:org", "workflow", "gis"},
	}
	for in, want := range tests {
		got := splitScopes(in)
		if got == nil || !reflect.DeepEqual(got, want) {
			t.Errorf("splitScopes(%q) = %#v, veut %#v", in, got, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	long := strings.Repeat("é", maxSummaryRunes+50)
	tests := []struct {
		name, in, want string
	}{
		{"vide", "", ""},
		{"blancs seulement", " \n\t\n", ""},
		{"première ligne non vide", "\n\n  unknown flag: --json  \nUsage: gh auth status\n", "unknown flag: --json"},
		{"retour chariot", "ligne\r\nsuite", "ligne"},
		{"caractères de contrôle", "a\x1b[31mb\x00c", "a [31mb c"},
		{"UTF-8 invalide", "a\xffb", "a?b"},
		{"token classique masqué", "bad token gho_ABCdef123 here", "bad token " + redacted + " here"},
		{"plusieurs tokens masqués", "ghp_x1 ghs_y2 ghu_z3 ghr_w4", strings.Join([]string{redacted, redacted, redacted, redacted}, " ")},
		{"token fin masqué", "github_pat_11AAA_bbb", redacted},
		{"faux token masqué", "token " + fakeToken + " refusé", "token " + redacted + " refusé"},
		{"raccourci", long, strings.Repeat("é", maxSummaryRunes) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarize(tt.in)
			if got != tt.want {
				t.Errorf("summarize(%q) = %q, veut %q", tt.in, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("summarize(%q) n'est pas de l'UTF-8 valide", tt.in)
			}
		})
	}
}

func TestLooksNotLoggedIn(t *testing.T) {
	yes := []string{
		"You are not logged into any GitHub hosts. To log in, run: gh auth login",
		"You are not logged into any accounts on github.com",
		"no oauth token found for github.com",
		"To get started with GitHub CLI, please run:  gh auth login",
	}
	for _, s := range yes {
		if !looksNotLoggedIn(s) {
			t.Errorf("looksNotLoggedIn(%q) = false, veut true", s)
		}
	}
	no := []string{"", "unknown flag: --json", "HTTP 502: Bad Gateway"}
	for _, s := range no {
		if looksNotLoggedIn(s) {
			t.Errorf("looksNotLoggedIn(%q) = true, veut false", s)
		}
	}
}

func TestCappedBuffer(t *testing.T) {
	b := &cappedBuffer{max: 5}
	for _, chunk := range []string{"ab", "cd", "efg", "hij"} {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v ; veut %d, nil", chunk, n, err, len(chunk))
		}
	}
	if got := b.buf.String(); got != "abcde" {
		t.Errorf("contenu = %q, veut %q", got, "abcde")
	}
	if !b.overflow {
		t.Error("overflow = false, veut true")
	}

	b = &cappedBuffer{max: 4}
	if _, err := b.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if b.overflow {
		t.Error("overflow = true pour une sortie qui tient exactement dans la limite")
	}
}
