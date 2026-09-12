package main

import "testing"

func TestNotificationConfigureParserAndSetupOptIn(t *testing.T) {
	for _, args := range [][]string{{}, {"--provider", "auto"}, {"--provider", "both", "--codex-home", "relative"}, {"--provider", "claude", "--navigation", "none", "--app", "/A.app"}} {
		if _, _, err := parseNotificationConfigure(args); err == nil {
			t.Fatal(args)
		}
	}
	for _, flag := range []string{"--remove", "--print", "--dry-run"} {
		if _, err := parseSetupCodexOptions([]string{"--agent-notify", flag}); err == nil {
			t.Fatal(flag)
		}
	}
	if _, err := parseSetupCodexOptions([]string{"--skip-agent-notify", "--navigation", "none"}); err == nil {
		t.Fatal("route without agent-notify")
	}
	if _, err := parseSetupCodexOptions([]string{"--agent-notify", "--skip-agent-notify"}); err == nil {
		t.Fatal("conflicting opt-in")
	}
	if opts, err := parseSetupCodexOptions(nil); err != nil || !opts.configure || len(opts.configureArgs) != 2 {
		t.Fatal("default agent-notify", opts, err)
	}
	if _, err := parseSetupCodexOptions([]string{"--agent-notify", "--navigation", "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := parseSetupCodexOptions([]string{"--configure-notifications"}); err == nil {
		t.Fatal("retired alias accepted")
	}
	if opts, err := parseSetupCodexOptions([]string{"--print"}); err != nil || opts.configure {
		t.Fatal("print should skip agent-notify", opts, err)
	}
}
