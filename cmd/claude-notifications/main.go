package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/777genius/claude-notifications/internal/audio"
	"github.com/777genius/claude-notifications/internal/errorhandler"
	"github.com/777genius/claude-notifications/internal/hooks"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/notifier"
)

const version = "1.38.0"
const windowsLazyUpdateRetryAfter = time.Hour

var (
	currentGOOS               = runtime.GOOS
	scheduleWindowsLazyUpdate = scheduleWindowsLazyUpdateImpl
)

func main() {
	// Initialize global error handler with panic recovery
	// logToConsole=true: errors will be shown in console
	// exitOnCritical=false: don't exit on critical errors (let caller decide)
	// recoveryEnabled=true: recover from panics
	errorhandler.Init(true, false, true)

	// Add global panic recovery
	defer errorhandler.HandlePanic()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "handle-hook":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: hook event name required\n")
			printUsage()
			os.Exit(1)
		}
		handleHook(os.Args[2])
	case "focus-window":
		if len(os.Args) < 4 {
			fmt.Fprintf(os.Stderr, "Error: focus-window requires bundleID and cwd arguments\n")
			os.Exit(1)
		}
		opts, err := parseFocusWindowOptions(os.Args[4:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "focus-window: %v\n", err)
			os.Exit(1)
		}
		if err := notifier.FocusAppWindowWithOptions(os.Args[2], os.Args[3], opts); err != nil {
			fmt.Fprintf(os.Stderr, "focus-window: %v\n", err)
			os.Exit(1)
		}
	case "play-sound":
		runPlaySound(os.Args[2:])
	case "daemon", "--daemon":
		runDaemon()
	case "windows-hooks":
		runWindowsHooks(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("claude-notifications v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

type hookSettings struct {
	Hooks map[string][]hookMatcherGroup `json:"hooks"`
}

type hookMatcherGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	Shell   string `json:"shell"`
}

func runWindowsHooks(args []string) {
	exePath, err := parseWindowsHooksExecutable(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "windows-hooks: %v\n", err)
		os.Exit(1)
	}

	settings := hookSettings{
		Hooks: map[string][]hookMatcherGroup{
			"PreToolUse": {
				{
					Matcher: "ExitPlanMode|AskUserQuestion",
					Hooks:   []hookCommand{newPowerShellHook(exePath, "PreToolUse")},
				},
			},
			"Notification": {
				{
					Matcher: "permission_prompt",
					Hooks:   []hookCommand{newPowerShellHook(exePath, "Notification")},
				},
			},
			"Stop": {
				{
					Hooks: []hookCommand{newPowerShellHook(exePath, "Stop")},
				},
			},
			"SubagentStop": {
				{
					Hooks: []hookCommand{newPowerShellHook(exePath, "SubagentStop")},
				},
			},
			"TeammateIdle": {
				{
					Hooks: []hookCommand{newPowerShellHook(exePath, "TeammateIdle")},
				},
			},
		},
	}

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "windows-hooks: failed to render JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(out))
}

func parseWindowsHooksExecutable(args []string) (string, error) {
	exeOverride := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--exe":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--exe requires a path")
			}
			i++
			exeOverride = args[i]
		default:
			return "", fmt.Errorf("unknown windows-hooks option: %s", args[i])
		}
	}

	if exeOverride != "" {
		return filepath.Abs(exeOverride)
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to detect executable path: %w", err)
	}

	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executable path: %w", err)
	}

	if strings.EqualFold(filepath.Ext(exePath), ".exe") {
		return exePath, nil
	}

	pluginRoot := getPluginRoot()
	return filepath.Abs(filepath.Join(pluginRoot, "bin", "claude-notifications-windows-amd64.exe"))
}

func newPowerShellHook(exePath, hookName string) hookCommand {
	return hookCommand{
		Type:    "command",
		Command: "$OutputEncoding = [System.Text.UTF8Encoding]::new($false); $input | & " + powershellDoubleQuoted(exePath) + " handle-hook " + hookName,
		Timeout: 30,
		Shell:   "powershell",
	}
}

func powershellDoubleQuoted(value string) string {
	replacer := strings.NewReplacer(
		"`", "``",
		"$", "`$",
		"\"", "`\"",
	)
	return `"` + replacer.Replace(value) + `"`
}

func handleHook(hookEvent string) {
	// Add panic recovery for this function
	defer errorhandler.HandlePanic()

	// Determine plugin root
	pluginRoot := getPluginRoot()
	maybeScheduleWindowsLazyUpdate(pluginRoot)

	// Initialize logger
	if _, err := logging.InitLogger(pluginRoot); err != nil {
		errorhandler.HandleCriticalError(err, "Failed to initialize logger")
		os.Exit(1)
	}
	defer logging.Close()

	// Create handler
	handler, err := hooks.NewHandler(pluginRoot)
	if err != nil {
		errorhandler.HandleCriticalError(err, "Failed to create handler")
		os.Exit(1)
	}

	// Handle hook
	if err := handler.HandleHook(hookEvent, os.Stdin); err != nil {
		errorhandler.HandleCriticalError(err, "Failed to handle hook")
		os.Exit(1)
	}
}

type pluginManifest struct {
	Version string `json:"version"`
}

func maybeScheduleWindowsLazyUpdate(pluginRoot string) {
	if currentGOOS != "windows" {
		return
	}

	pluginVersion := readPluginManifestVersion(pluginRoot)
	if pluginVersion == "" || pluginVersion == version {
		return
	}

	stampKey := version + "->" + pluginVersion
	stampPath := windowsLazyUpdateStampPath(pluginRoot)
	if windowsLazyUpdateRecentlyScheduled(stampPath, stampKey) {
		return
	}

	stampWritten := writeWindowsLazyUpdateStamp(stampPath, stampKey) == nil
	if err := scheduleWindowsLazyUpdate(pluginRoot); err != nil && stampWritten {
		_ = os.Remove(stampPath)
	}
}

func readPluginManifestVersion(pluginRoot string) string {
	data, err := os.ReadFile(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"))
	if err != nil {
		return ""
	}

	var manifest pluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	return manifest.Version
}

func windowsLazyUpdateStampPath(pluginRoot string) string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		cacheDir = filepath.Join(pluginRoot, ".cache")
	}
	return filepath.Join(cacheDir, "claude-notifications-go", "windows-lazy-update-stamp")
}

func windowsLazyUpdateRecentlyScheduled(stampPath, stampKey string) bool {
	info, err := os.Stat(stampPath)
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) > windowsLazyUpdateRetryAfter {
		return false
	}

	data, err := os.ReadFile(stampPath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(data)) == stampKey
}

func writeWindowsLazyUpdateStamp(stampPath, stampKey string) error {
	if err := os.MkdirAll(filepath.Dir(stampPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(stampPath, []byte(stampKey+"\n"), 0o600)
}

func scheduleWindowsLazyUpdateImpl(pluginRoot string) error {
	installScript := filepath.Join(pluginRoot, "bin", "install.sh")
	if _, err := os.Stat(installScript); err != nil {
		return err
	}

	powershellPath, err := findWindowsPowerShell()
	if err != nil {
		return err
	}

	bashPath, err := findWindowsBash()
	if err != nil {
		return err
	}

	targetDir := filepath.ToSlash(filepath.Join(pluginRoot, "bin"))
	installScript = filepath.ToSlash(installScript)
	shCommand := "INSTALL_TARGET_DIR=" + shellSingleQuoted(targetDir) + " " + shellSingleQuoted(installScript) + " --force"
	psCommand := "$ErrorActionPreference = 'SilentlyContinue'; " +
		"Start-Sleep -Milliseconds 750; " +
		"for ($i = 0; $i -lt 6; $i++) { " +
		"& " + powershellSingleQuoted(bashPath) + " -lc " + powershellSingleQuoted(shCommand) + " *> $null; " +
		"if ($LASTEXITCODE -eq 0) { break }; " +
		"Start-Sleep -Seconds 5 }"

	cmd := exec.Command(powershellPath, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psCommand)
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err == nil {
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}

	if err := cmd.Start(); err != nil {
		if devNull != nil {
			_ = devNull.Close()
		}
		return err
	}

	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	if devNull != nil {
		_ = devNull.Close()
	}
	return nil
}

func findWindowsPowerShell() (string, error) {
	if path, err := exec.LookPath("powershell.exe"); err == nil {
		return path, nil
	}

	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		candidate := filepath.Join(systemRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("powershell.exe not found")
}

func findWindowsBash() (string, error) {
	if override := os.Getenv("CLAUDE_NOTIFICATIONS_BASH"); override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, nil
		}
		return "", fmt.Errorf("CLAUDE_NOTIFICATIONS_BASH not found: %s", override)
	}

	for _, name := range []string{"bash.exe", "bash"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	for _, candidate := range windowsBashCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("bash.exe not found")
}

func windowsBashCandidates() []string {
	var candidates []string
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if root == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "Git", "bin", "bash.exe"),
			filepath.Join(root, "Programs", "Git", "bin", "bash.exe"),
		)
	}
	return candidates
}

func shellSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func powershellSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func getPluginRoot() string {
	// Try CLAUDE_PLUGIN_ROOT environment variable first
	if root := os.Getenv("CLAUDE_PLUGIN_ROOT"); root != "" {
		return root
	}

	// Try to find plugin root relative to executable
	exe, err := os.Executable()
	if err == nil {
		// Executable is in bin/, so plugin root is parent directory
		exeDir := filepath.Dir(exe)
		if filepath.Base(exeDir) == "bin" {
			return filepath.Dir(exeDir)
		}
		// Otherwise, try parent of executable dir
		return filepath.Dir(exeDir)
	}

	// Fallback to current directory
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// runPlaySound plays a sound file and exits. Designed to be spawned as a detached
// child process so the parent hook process does not wait for audio to finish.
// Usage: play-sound <path> [--volume <0.0-1.0>] [--device <name>]
func runPlaySound(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "play-sound: sound file path required\n")
		os.Exit(1)
	}

	soundPath := args[0]
	volume := 1.0
	deviceName := ""

	// Parse optional flags
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--volume":
			if i+1 < len(args) {
				i++
				if v, err := strconv.ParseFloat(args[i], 64); err == nil {
					volume = v
				}
			}
		case "--device":
			if i+1 < len(args) {
				i++
				deviceName = args[i]
			}
		}
	}

	player, err := audio.NewPlayer(deviceName, volume)
	if err != nil {
		fmt.Fprintf(os.Stderr, "play-sound: failed to init player: %v\n", err)
		os.Exit(1)
	}
	defer player.Close()

	if err := player.Play(soundPath); err != nil {
		fmt.Fprintf(os.Stderr, "play-sound: failed to play %s: %v\n", soundPath, err)
		os.Exit(1)
	}
}

func parseFocusWindowOptions(args []string) (notifier.FocusWindowOptions, error) {
	var opts notifier.FocusWindowOptions

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ghostty-terminal-id":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("--ghostty-terminal-id requires a value")
			}
			i++
			opts.GhosttyTerminalID = args[i]
		default:
			return opts, fmt.Errorf("unknown focus-window option: %s", args[i])
		}
	}

	return opts, nil
}

func printUsage() {
	fmt.Println("claude-notifications - Smart notifications for Claude Code")
	fmt.Println()
	fmt.Printf("Version: %s\n", version)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  claude-notifications handle-hook <HookName>")
	fmt.Println("  claude-notifications daemon")
	fmt.Println("  claude-notifications windows-hooks [--exe <path>]")
	fmt.Println("  claude-notifications version")
	fmt.Println("  claude-notifications help")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  handle-hook <HookName>  Handle a Claude Code hook event")
	fmt.Println("                          HookName: PreToolUse, Stop, SubagentStop, Notification")
	fmt.Println("  daemon                  Run the notification daemon (Linux only)")
	fmt.Println("                          For click-to-focus support on desktop notifications")
	fmt.Println("  focus-window <bundleID> <cwd> [--ghostty-terminal-id <id>]")
	fmt.Println("                          Focus specific app window (internal, used by click-to-focus)")
	fmt.Println("  windows-hooks           Print PowerShell hook JSON for Windows settings")
	fmt.Println("                          Does not modify ~/.claude/settings.json")
	fmt.Println("  version                 Show version information")
	fmt.Println("  help                    Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Handle PreToolUse hook (reads JSON from stdin)")
	fmt.Println("  echo '{\"session_id\":\"test\",\"tool_name\":\"ExitPlanMode\"}' | claude-notifications handle-hook PreToolUse")
	fmt.Println()
	fmt.Println("  # Handle Stop hook")
	fmt.Println("  echo '{\"session_id\":\"test\",\"transcript_path\":\"/path/to/transcript.jsonl\"}' | claude-notifications handle-hook Stop")
	fmt.Println()
	fmt.Println("  # Run notification daemon (Linux only, started automatically)")
	fmt.Println("  claude-notifications daemon")
	fmt.Println()
	fmt.Println("  # Print Windows PowerShell hook configuration")
	fmt.Println("  claude-notifications windows-hooks")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  CLAUDE_PLUGIN_ROOT  Plugin root directory (auto-detected if not set)")
	fmt.Println()
}
