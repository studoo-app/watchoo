package git

// Classification of the failures of git commands that contact the remote (remoteError).

import (
	"errors"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider"
)

func TestRemoteError(t *testing.T) {
	sshFooter := "fatal: Could not read from remote repository.\n\n" +
		"Please make sure you have the correct access rights\nand the repository exists.\n"
	for _, tc := range []struct {
		name   string
		stderr string
		want   error
	}{
		{"unknown host key", "Host key verification failed.\n" + sshFooter, provider.ErrHostKeyUnknown},
		{"key refused", "git@github.com: Permission denied (publickey).\n" + sshFooter, provider.ErrAccessDenied},
		{"https without credentials",
			"fatal: could not read Username for 'https://github.com': terminal prompts disabled\n", provider.ErrAccessDenied},
		{"network", "ssh: Could not resolve hostname github.com: nodename nor servname provided\n" + sshFooter,
			provider.ErrUnavailable},
		{"other", "fatal: something unexpected\n", provider.ErrUnexpected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := remoteError("update", result{stderr: tc.stderr, exitCode: 128})
			var apiErr *provider.APIError
			if !errors.As(err, &apiErr) || apiErr.Err != tc.want {
				t.Fatalf("remoteError = %v, want cause %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "fatal") || strings.Contains(err.Error(), "github.com:") {
				t.Errorf("the output of git is copied into the error: %q", err)
			}
		})
	}
}

// TestRemoteErrorHostKey checks that an unknown SSH host key stays an access failure (repository in
// error, inaccessible at addition) with its own advice, not the SSH key one.
func TestRemoteErrorHostKey(t *testing.T) {
	err := remoteError("update", result{stderr: "Host key verification failed.\n", exitCode: 128})
	if !errors.Is(err, provider.ErrAccessDenied) || !errors.Is(err, provider.ErrHostKeyUnknown) {
		t.Errorf("remoteError = %v, want ErrHostKeyUnknown and ErrAccessDenied", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "ssh -T git@github.com") || strings.Contains(msg, "check the SSH key") {
		t.Errorf("message = %q", msg)
	}
}
