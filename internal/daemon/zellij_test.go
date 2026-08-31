//go:build linux

package daemon

import "testing"

func TestGetZellijFocusHints_InsideZellij(t *testing.T) {
	// Zellij sets ZELLIJ=0 inside a session; the value is a marker, not a flag.
	t.Setenv("ZELLIJ", "0")
	t.Setenv("ZELLIJ_SESSION_NAME", "cubic-weasel")
	t.Setenv("ZELLIJ_PANE_ID", "2")

	session, paneID := GetZellijFocusHints()
	if session != "cubic-weasel" || paneID != "2" {
		t.Errorf("GetZellijFocusHints() = (%q, %q), want (%q, %q)", session, paneID, "cubic-weasel", "2")
	}
}

func TestGetZellijFocusHints_OutsideZellij(t *testing.T) {
	// Setting a variable empty is how these tests spell "unset": the code reads
	// every one of them through os.Getenv, which cannot tell the two apart.
	t.Setenv("ZELLIJ", "")
	t.Setenv("ZELLIJ_SESSION_NAME", "")
	t.Setenv("ZELLIJ_PANE_ID", "")

	session, paneID := GetZellijFocusHints()
	if session != "" || paneID != "" {
		t.Errorf("GetZellijFocusHints() outside zellij = (%q, %q), want empty", session, paneID)
	}
}

// A background job inherits the pane variables without $ZELLIJ, and its pane is
// as real as any other. Observed on a Claude Code background session whose click
// raised the window but switched no pane.
func TestGetZellijFocusHints_WithoutZellijMarker(t *testing.T) {
	t.Setenv("ZELLIJ", "")
	t.Setenv("ZELLIJ_SESSION_NAME", "cubic-weasel")
	t.Setenv("ZELLIJ_PANE_ID", "2")

	session, paneID := GetZellijFocusHints()
	if session != "cubic-weasel" || paneID != "2" {
		t.Errorf("GetZellijFocusHints() = (%q, %q), want (%q, %q)", session, paneID, "cubic-weasel", "2")
	}
}

func TestInZellij(t *testing.T) {
	cases := []struct {
		name    string
		marker  string
		session string
		paneID  string
		want    bool
	}{
		{"marker only", "0", "", "", true},
		{"pane variables only", "", "cubic-weasel", "2", true},
		{"everything set", "0", "cubic-weasel", "2", true},
		{"session without pane", "", "cubic-weasel", "", false},
		{"pane without session", "", "", "2", false},
		{"nothing set", "", "", "", false},
		{"blank pane variables", "", "  ", " ", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("ZELLIJ", testCase.marker)
			t.Setenv("ZELLIJ_SESSION_NAME", testCase.session)
			t.Setenv("ZELLIJ_PANE_ID", testCase.paneID)

			if got := InZellij(); got != testCase.want {
				t.Errorf("InZellij() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestGetZellijFocusHints_TrimsWhitespace(t *testing.T) {
	t.Setenv("ZELLIJ", "0")
	t.Setenv("ZELLIJ_SESSION_NAME", "  cubic-weasel\n")
	t.Setenv("ZELLIJ_PANE_ID", " 2 ")

	session, paneID := GetZellijFocusHints()
	if session != "cubic-weasel" || paneID != "2" {
		t.Errorf("GetZellijFocusHints() = (%q, %q), want trimmed values", session, paneID)
	}
}

func TestTryZellijPane_MissingHints(t *testing.T) {
	cases := []struct {
		name    string
		session string
		paneID  string
	}{
		{"no session", "", "2"},
		{"no pane", "cubic-weasel", ""},
		{"neither", "", ""},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := TryZellijPane(testCase.session, testCase.paneID); err == nil {
				t.Errorf("TryZellijPane(%q, %q) = nil, want error", testCase.session, testCase.paneID)
			}
		})
	}
}

// zellij exits 2 both when the pane is already focused and when it does not
// exist, so the two have to be told apart by their message.
func TestIsZellijAlreadyFocused(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{"already focused", "Pane Terminal(2) is already focused", true},
		{"already focused, different case", "PANE TERMINAL(2) IS ALREADY FOCUSED", true},
		{"pane not found shares the exit code", "Pane with id Terminal(9999) not found", false},
		{"session not found", "Session 'no-such-session' not found.", false},
		{"empty", "", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isZellijAlreadyFocused(testCase.output); got != testCase.want {
				t.Errorf("isZellijAlreadyFocused(%q) = %v, want %v", testCase.output, got, testCase.want)
			}
		})
	}
}
