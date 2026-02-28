package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/claude-notifications/internal/errorhandler"
	"github.com/777genius/claude-notifications/internal/hooks"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/notifier"
)

const version = "1.27.0"

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
		// Initialize logger so focus-window results appear in notification-debug.log.
		// focus-window runs as a background subprocess (notification click callback),
		// so stderr is discarded; the log file is the only observable output.
		pluginRoot := getPluginRoot()
		if _, err := logging.InitLogger(pluginRoot); err == nil {
			defer logging.Close()
		}
		bundleID, cwd := os.Args[2], os.Args[3]
		logging.Debug("focus-window: bundleID=%s cwd=%s", bundleID, cwd)
		if err := notifier.FocusAppWindow(bundleID, cwd); err != nil {
			logging.Error("focus-window failed: %v", err)
			fmt.Fprintf(os.Stderr, "focus-window: %v\n", err)
			os.Exit(1)
		}
		logging.Debug("focus-window: success")
	case "daemon", "--daemon":
		runDaemon()
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

func handleHook(hookEvent string) {
	// Add panic recovery for this function
	defer errorhandler.HandlePanic()

	// Determine plugin root
	pluginRoot := getPluginRoot()

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

func printUsage() {
	fmt.Println("claude-notifications - Smart notifications for Claude Code")
	fmt.Println()
	fmt.Printf("Version: %s\n", version)
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  claude-notifications handle-hook <HookName>")
	fmt.Println("  claude-notifications daemon")
	fmt.Println("  claude-notifications version")
	fmt.Println("  claude-notifications help")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  handle-hook <HookName>  Handle a Claude Code hook event")
	fmt.Println("                          HookName: PreToolUse, Stop, SubagentStop, Notification")
	fmt.Println("  daemon                  Run the notification daemon (Linux only)")
	fmt.Println("                          For click-to-focus support on desktop notifications")
	fmt.Println("  focus-window <bundleID> <cwd>")
	fmt.Println("                          Focus app window by bundleID and cwd (internal, used by click-to-focus)")
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
	fmt.Println("Environment Variables:")
	fmt.Println("  CLAUDE_PLUGIN_ROOT  Plugin root directory (auto-detected if not set)")
	fmt.Println()
}
