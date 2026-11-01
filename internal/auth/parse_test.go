package auth

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestParseStatus(t *testing.T) {
	const host = "github.com"
	tests := []struct {
		name    string
		stdout  string
		user    string
		want    Status
		wantErr error
		notErr  error    // error that the returned error must not wrap
		msg     []string // excerpts expected in the error message
		noMsg   []string // excerpts forbidden in the error message
	}{
		{
			name:   "active account",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"gist, read:org, repo","gitProtocol":"ssh"}]}}`,
			want:   Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "success", Scopes: []string{"gist", "read:org", "repo"}},
		},
		{
			name: "active account among several",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":false,"host":"github.com","login":"bob","tokenSource":"keyring","scopes":"repo"},
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"}]}}`,
			want: Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "success", Scopes: []string{"repo"}},
		},
		{
			name: "account chosen by github.user",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"},
				{"state":"success","active":false,"host":"github.com","login":"Bob","tokenSource":"oauth_token","scopes":"repo, read:org"}]}}`,
			user: "Bob",
			want: Status{Host: host, Login: "Bob", TokenSource: "oauth_token", State: "success", Scopes: []string{"repo", "read:org"}},
		},
		{
			// "gh auth token --user bob" would fail: gh is case-sensitive.
			name: "github.user with another case",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"},
				{"state":"success","active":false,"host":"github.com","login":"Bob","tokenSource":"oauth_token","scopes":"repo"}]}}`,
			user:    "bob",
			wantErr: ErrUnusableUser,
			msg:     []string{`github.user is "bob"`, `as "Bob"`, "case-sensitive", "fix the github.user setting"},
			noMsg:   []string{"gh auth login"},
		},
		{
			name: "github.user prefers the stored account to GH_TOKEN",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"},
				{"state":"success","active":false,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"}]}}`,
			user: "alice",
			want: Status{Host: host, Login: "alice", TokenSource: "keyring", State: "success", Scopes: []string{"repo"}},
		},
		{
			// "gh auth token --user alice" ignores GH_TOKEN and would fail (checked with gh 2.103).
			name:    "github.user only matches GH_TOKEN",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"}]}}`,
			user:    "alice",
			wantErr: ErrUnusableUser,
			msg:     []string{`github.user is "alice"`, "only knows this account on github.com through the variable GH_TOKEN", "empty it"},
			noMsg:   []string{"gh auth login"},
		},
		{
			name:    "github.user only matches GH_TOKEN, with another case",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"Alice","tokenSource":"GH_TOKEN"}]}}`,
			user:    "alice",
			wantErr: ErrUnusableUser,
			msg:     []string{"through the variable GH_TOKEN"},
		},
		{
			name: "github.user: the case of a stored account wins over GH_TOKEN",
			stdout: `{"hosts":{"github.com":[
				{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"},
				{"state":"success","active":false,"host":"github.com","login":"Alice","tokenSource":"keyring"}]}}`,
			user:    "alice",
			wantErr: ErrUnusableUser,
			msg:     []string{`as "Alice"`},
		},
		{
			name:   "active account from GH_TOKEN, without github.user",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"ci-bot","tokenSource":"GH_TOKEN"}]}}`,
			want:   Status{Host: host, Login: "ci-bot", TokenSource: "GH_TOKEN", Active: true, State: "success", Scopes: []string{}},
			notErr: ErrNotLoggedIn,
		},
		{
			name:    "unknown github.user",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice","tokenSource":"keyring"}]}}`,
			user:    "carol",
			wantErr: ErrNotLoggedIn,
			notErr:  ErrUnusableUser,
			msg:     []string{`"carol"`, "github.user", "gh auth login"},
		},
		{
			name:    "no active account",
			stdout:  `{"hosts":{"github.com":[{"state":"success","active":false,"host":"github.com","login":"alice","tokenSource":"keyring"}]}}`,
			wantErr: ErrNotLoggedIn,
		},
		{
			name:    "no account (answer of a gh not signed in)",
			stdout:  `{"hosts":{}}`,
			wantErr: ErrNotLoggedIn,
		},
		{
			name:    "accounts of another host only",
			stdout:  `{"hosts":{"ghe.example.test":[{"state":"success","active":true,"host":"ghe.example.test","login":"alice"}]}}`,
			wantErr: ErrNotLoggedIn,
		},
		{
			name:   "host key in uppercase",
			stdout: `{"hosts":{"GitHub.com":[{"state":"success","active":true,"host":"GitHub.com","login":"alice","tokenSource":"keyring","scopes":""}]}}`,
			want:   Status{Host: "GitHub.com", Login: "alice", TokenSource: "keyring", Active: true, State: "success", Scopes: []string{}},
		},
		{
			name:   "host missing from the entry",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"login":"alice"}]}}`,
			want:   Status{Host: host, Login: "alice", Active: true, State: "success", Scopes: []string{}},
		},
		{
			name:   "scopes as a list",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"login":"alice","scopes":[" repo ","gist",""]}]}}`,
			want:   Status{Host: host, Login: "alice", Active: true, State: "success", Scopes: []string{"repo", "gist"}},
		},
		{
			name:   "scopes of an unexpected type",
			stdout: `{"hosts":{"github.com":[{"state":"success","active":true,"login":"alice","scopes":42}]}}`,
			want:   Status{Host: host, Login: "alice", Active: true, State: "success", Scopes: []string{}},
		},
		{
			name:    "invalid token",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"HTTP 401: Bad credentials (https://api.github.com/)","active":true,"host":"github.com","login":"alice","tokenSource":"GH_TOKEN"}]}}`,
			want:    Status{Host: host, Login: "alice", TokenSource: "GH_TOKEN", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
			msg:     []string{`"alice"`, "is not usable", "gh auth login"},
			noMsg:   []string{"Bad credentials"},
		},
		{
			name:    "refused token, message without HTTP code",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"bad credentials","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
		},
		{
			// Real format of gh 2.103, without the text "Bad credentials".
			name:    "refused token, \"status code: 401\" format of gh",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"non-200 OK status code: 401 Unauthorized body: \"\"","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
		},
		{
			name:    "error state without detail",
			stdout:  `{"hosts":{"github.com":[{"state":"error","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
			notErr:  ErrCheckFailed,
		},
		{
			name:    "unknown state",
			stdout:  `{"hosts":{"github.com":[{"state":"expired","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "expired", Scopes: []string{}},
			wantErr: ErrNotLoggedIn,
		},
		{
			name:    "github.com unreachable (timeout)",
			stdout:  `{"hosts":{"github.com":[{"state":"timeout","error":"net/http: request canceled (Client.Timeout exceeded)","active":true,"host":"github.com","login":"alice","tokenSource":"keyring"}]}}`,
			want:    Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "timeout", Scopes: []string{}},
			wantErr: ErrCheckFailed,
			notErr:  ErrNotLoggedIn,
			msg:     []string{"could not be checked", `"timeout"`, "Client.Timeout exceeded", "network"},
			noMsg:   []string{"gh auth login"},
		},
		{
			// Real answer of gh 2.103 with HTTPS_PROXY=http://127.0.0.1:9 and a valid token.
			name:    "github.com unreachable (proxy)",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"Get \"https://api.github.com/\": proxyconnect tcp: dial tcp 127.0.0.1:9: connect: connection refused","active":true,"host":"github.com","login":"alice","tokenSource":"keyring","scopes":"repo"}]}}`,
			want:    Status{Host: host, Login: "alice", TokenSource: "keyring", Active: true, State: "error", Scopes: []string{"repo"}},
			wantErr: ErrCheckFailed,
			notErr:  ErrNotLoggedIn,
			msg:     []string{"proxyconnect tcp", "connection refused"},
			noMsg:   []string{"gh auth login"},
		},
		{
			name:    "gh error on several lines, with a token",
			stdout:  `{"hosts":{"github.com":[{"state":"error","error":"HTTP 502: Bad Gateway gho_secret123\nnext","active":true,"login":"alice"}]}}`,
			want:    Status{Host: host, Login: "alice", Active: true, State: "error", Scopes: []string{}},
			wantErr: ErrCheckFailed,
			msg:     []string{"HTTP 502: Bad Gateway [redacted]"},
			noMsg:   []string{"gho_secret123", "next"},
		},
		{name: "empty output", stdout: ``, wantErr: ErrUnexpectedOutput},
		{name: "not JSON", stdout: `github.com: Logged in`, wantErr: ErrUnexpectedOutput},
		{name: "object without hosts", stdout: `{}`, wantErr: ErrUnexpectedOutput},
		{name: "null hosts", stdout: `{"hosts":null}`, wantErr: ErrUnexpectedOutput},
		{name: "hosts of a wrong type", stdout: `{"hosts":[]}`, wantErr: ErrUnexpectedOutput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseStatus([]byte(tt.stdout), host, tt.user)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.notErr != nil && errors.Is(err, tt.notErr) {
				t.Errorf("the error %q should not wrap %q", err, tt.notErr)
			}
			for _, part := range tt.msg {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("message %v without %q", err, part)
				}
			}
			for _, part := range tt.noMsg {
				if err != nil && strings.Contains(err.Error(), part) {
					t.Errorf("message %q with %q", err, part)
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Status = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// ErrUnusableUser wraps ErrNotLoggedIn, like the error that Token would return in this case, but its
// message does not advise "gh auth login", which would fix nothing.
func TestErrUnusableUserWrapsNotLoggedIn(t *testing.T) {
	if !errors.Is(ErrUnusableUser, ErrNotLoggedIn) {
		t.Error("ErrUnusableUser does not wrap ErrNotLoggedIn")
	}
	if errors.Is(ErrNotLoggedIn, ErrUnusableUser) {
		t.Error("ErrNotLoggedIn must not be ErrUnusableUser")
	}
	if msg := ErrUnusableUser.Error(); strings.Contains(msg, "gh auth login") || !strings.Contains(msg, "github.user") {
		t.Errorf("message of ErrUnusableUser = %q", msg)
	}
}

func TestParseStatusErrorDoesNotQuoteStdout(t *testing.T) {
	stdout := `{"hosts": oops gho_secret_in_the_output`
	_, err := parseStatus([]byte(stdout), "github.com", "")
	if !errors.Is(err, ErrUnexpectedOutput) {
		t.Fatalf("error = %v, want ErrUnexpectedOutput", err)
	}
	if strings.Contains(err.Error(), "oops") || strings.Contains(err.Error(), "gho_") {
		t.Errorf("the message %q copies the standard output of gh", err)
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
			t.Errorf("splitScopes(%q) = %#v, want %#v", in, got, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	long := strings.Repeat("é", maxSummaryRunes+50)
	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"blanks only", " \n\t\n", ""},
		{"first non-empty line", "\n\n  unknown flag: --json  \nUsage: gh auth status\n", "unknown flag: --json"},
		{"carriage return", "line\r\nnext", "line"},
		{"control characters", "a\x1b[31mb\x00c", "a [31mb c"},
		{"invalid UTF-8", "a\xffb", "a?b"},
		{"classic token masked", "bad token gho_ABCdef123 here", "bad token [redacted] here"},
		{"several tokens masked", "ghp_x1 ghs_y2 ghu_z3 ghr_w4", strings.Repeat("[redacted] ", 3) + "[redacted]"},
		{"fine-grained token masked", "github_pat_11AAA_bbb", "[redacted]"},
		{"fake token masked", "token " + fakeToken + " refused", "token [redacted] refused"},
		{"shortened", long, strings.Repeat("é", maxSummaryRunes) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarize(tt.in)
			if got != tt.want {
				t.Errorf("summarize(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("summarize(%q) is not valid UTF-8", tt.in)
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
			t.Errorf("looksNotLoggedIn(%q) = false, want true", s)
		}
	}
	no := []string{"", "unknown flag: --json", "HTTP 502: Bad Gateway"}
	for _, s := range no {
		if looksNotLoggedIn(s) {
			t.Errorf("looksNotLoggedIn(%q) = true, want false", s)
		}
	}
}

func TestCappedBuffer(t *testing.T) {
	b := &cappedBuffer{max: 5}
	for _, chunk := range []string{"ab", "cd", "efg", "hij"} {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v; want %d, nil", chunk, n, err, len(chunk))
		}
	}
	if got := b.buf.String(); got != "abcde" {
		t.Errorf("content = %q, want %q", got, "abcde")
	}
	if !b.overflow {
		t.Error("overflow = false, want true")
	}

	b = &cappedBuffer{max: 4}
	if _, err := b.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if b.overflow {
		t.Error("overflow = true for an output that exactly fits the limit")
	}
}

func TestAbsent(t *testing.T) {
	tests := map[error]bool{
		ErrGHNotFound:                      true,
		ErrNotLoggedIn:                     true,
		ErrUnusableUser:                    true,
		fmt.Errorf("x: %w", ErrGHNotFound): true,
		ErrCheckFailed:                     false,
		ErrTimeout:                         false,
		ErrUnexpectedOutput:                false,
		errors.New("other"):                false,
	}
	for err, want := range tests {
		if got := Absent(err); got != want {
			t.Errorf("Absent(%v) = %v, want %v", err, got, want)
		}
	}
}

// The messages follow the process language: French typography (space before ":"), French masking
// mark. Not parallel: the process language is shared.
func TestMessagesFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	_, err := parseStatus([]byte(`{"hosts":{}}`), "github.com", "")
	if want := "aucun compte gh connecté pour github.com : gh n'est pas connecté à GitHub : lancez « gh auth login »"; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %q", err, want)
	}
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("errors.Is(ErrNotLoggedIn) false: %v", err)
	}
	if got := summarize("bad token gho_ABCdef123"); got != "bad token [masqué]" {
		t.Errorf("summarize = %q", got)
	}
	if got := ErrUnusableUser.Error(); !strings.HasPrefix(got, "corrigez le réglage github.user") {
		t.Errorf("ErrUnusableUser = %q", got)
	}
}
