package warpfocus

import (
	"strings"
	"testing"
)

const validHex = "6b7be92641ae8ced80188a4d87e4b200"

func TestNormalize_SessionURLs(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"warp://session/" + validHex, "warp://session/" + validHex},
		{"WARP://SESSION/" + strings.ToUpper(validHex), "warp://session/" + validHex},
		{"warppreview://session/" + validHex, "warppreview://session/" + validHex},
		{"warposs://session/" + validHex, "warposs://session/" + validHex},
		{"  warp://session/" + validHex + " \n", "warp://session/" + validHex},
		{"warp://session/6b7be926-41ae-8ced-8018-8a4d87e4b200", "warp://session/" + validHex},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalize_RejectsNonSessionURLs(t *testing.T) {
	rejected := []string{
		"",
		"https://example.com",
		"javascript:alert(1)",
		"file:///etc/passwd",
		"warp://action/new_tab",
		"warp://action/new_window?path=/tmp",
		"warp://conversation/" + validHex,
		"warp://settings",
		"warp://launch/foo",
		"warp://session/",
		"warp://session/not-hex",
		"warp://session/abc",
		"warp://session/" + validHex + "/extra",
		"warp://session/" + validHex + "@evil",
		"http://session/" + validHex,
	}
	for _, in := range rejected {
		if got := Normalize(in); got != "" {
			t.Errorf("Normalize(%q) = %q, want empty", in, got)
		}
	}
}

func TestFromSessionUUID(t *testing.T) {
	if got := FromSessionUUID(validHex); got != "warp://session/"+validHex {
		t.Errorf("FromSessionUUID(hex) = %q", got)
	}
	dashed := "6b7be926-41ae-8ced-8018-8a4d87e4b200"
	if got := FromSessionUUID(dashed); got != "warp://session/"+validHex {
		t.Errorf("FromSessionUUID(dashed) = %q", got)
	}
	if got := FromSessionUUID("nope"); got != "" {
		t.Errorf("FromSessionUUID(invalid) = %q, want empty", got)
	}
}

func TestFromEnv_PrefersFocusURL(t *testing.T) {
	t.Setenv(FocusURLEnv, "warposs://session/"+validHex)
	t.Setenv(SessionUUIDEnv, "ffffffffffffffffffffffffffffffff")
	t.Setenv("TMUX", "")
	if got := FromEnv(); got != "warposs://session/"+validHex {
		t.Errorf("FromEnv() = %q, want channel-aware focus URL", got)
	}
}

func TestFromEnv_UUIDFallback(t *testing.T) {
	t.Setenv(FocusURLEnv, "")
	t.Setenv(SessionUUIDEnv, validHex)
	t.Setenv("TMUX", "")
	if got := FromEnv(); got != "warp://session/"+validHex {
		t.Errorf("FromEnv() = %q, want UUID-derived stable URL", got)
	}
}

func TestFromEnv_Empty(t *testing.T) {
	t.Setenv(FocusURLEnv, "")
	t.Setenv(SessionUUIDEnv, "")
	t.Setenv("TMUX", "")
	if got := FromEnv(); got != "" {
		t.Errorf("FromEnv() = %q, want empty", got)
	}
}

func TestOpen_RejectsInvalid(t *testing.T) {
	if err := Open("warp://action/new_tab"); err == nil {
		t.Fatal("Open should reject non-session URLs")
	}
}
