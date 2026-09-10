//go:build windows

package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsUnavailableBaseDoesNotPublishHomeGuard(t *testing.T) {
	env := storeEnv(t)
	env.Vars["APPDATA"] = `relative\base`
	guard := filepath.Join(env.Vars["USERPROFILE"], ".agent-notifications-config.lock")
	_, err := EnsureInitialized(context.Background(), InitRequest{Env: env})
	var ce *Error
	if !errors.As(err, &ce) || ce.Code != ConfigBaseUnavailable {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(guard); !os.IsNotExist(statErr) {
		t.Fatalf("guard was created for rejected selection: %v", statErr)
	}
}

func TestWindowsLegacySurvivesUnavailableBase(t *testing.T) {
	env := storeEnv(t)
	legacy := filepath.Join(env.Vars["USERPROFILE"], ".claude", "claude-notifications-go", "config.json")
	if err := os.MkdirAll(filepath.Dir(legacy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	env.Vars["APPDATA"] = `relative\base`
	selected, err := resolveForMutation(context.Background(), env)
	if err != nil || !selected.Exists || selected.Source != "legacy" {
		t.Fatalf("selection=%+v err=%v", selected, err)
	}
}
