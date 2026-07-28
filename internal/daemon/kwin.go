//go:build linux

// ABOUTME: Window focus for KDE via KWin's D-Bus scripting interface.
// ABOUTME: Needs no external tool, unlike kdotool which wraps the same interface.
package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	kwinService      = "org.kde.KWin"
	kwinScriptPath   = "/Scripting"
	kwinScriptIface  = "org.kde.kwin.Scripting"
	kwinScriptRunner = "org.kde.kwin.Script"

	// The reply the loaded script sends back, so a run that matched nothing is
	// reported as a failure and the next focus method still gets its turn.
	kwinReplyService = "org.kde.kwin.claudenotifications"
	kwinReplyPath    = "/"
	kwinReplyIface   = "org.kde.kwin.claudenotifications"

	// KWin loads and runs the script asynchronously. Focus is a click response, so
	// the budget is short: either the compositor answers quickly or the caller moves
	// on to another method.
	kwinReplyTimeout = 2 * time.Second
)

// kwinFocusScript activates the best matching window and reports whether it worked.
//
// Two passes, because a class alone cannot tell two project windows apart: prefer a
// window whose caption mentions the project folder, and fall back to any window of
// the right class. The caption is where KDE puts a terminal's working directory and
// an editor's open folder, which is the only thing distinguishing them.
//
// Written for the Plasma 6 API with a Plasma 5 fallback: windowList/activeWindow
// were clientList/activeClient before Plasma 6.
const kwinFocusScript = `(function () {
    var list = (typeof workspace.windowList === "function")
        ? workspace.windowList()
        : workspace.clientList();

    var wantClass = '%s'.toLowerCase();
    var wantCaption = '%s';
    var byCaption = null, byClass = null;

    for (var i = 0; i < list.length; i++) {
        var w = list[i];
        if (!w || !w.normalWindow) { continue; }
        if (String(w.resourceClass || "").toLowerCase().indexOf(wantClass) < 0) { continue; }
        if (byClass === null) { byClass = w; }
        if (wantCaption !== "" && String(w.caption || "").indexOf(wantCaption) >= 0) {
            byCaption = w;
            break;
        }
    }

    var target = byCaption !== null ? byCaption : byClass;
    var ok = false;
    if (target !== null) {
        if (typeof workspace.activeWindow !== "undefined") {
            workspace.activeWindow = target;
            ok = String(workspace.activeWindow.internalId) === String(target.internalId);
        } else {
            workspace.activeClient = target;
            ok = String(workspace.activeClient.internalId) === String(target.internalId);
        }
    }
    callDBus('%s', '%s', '%s', 'Report', ok);
})();`

// TryKWinScript focuses a window through KWin's scripting interface.
//
// kdotool wraps this same interface, so this covers the same ground without asking
// the user to install anything — which matters on KDE, where the packaged window
// tools do not work: wlrctl speaks to wlroots compositors and KWin is not one, and
// xdotool only sees X11 windows while a Plasma Wayland session runs its apps
// natively.
func TryKWinScript(terminalName, folderName string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("session bus unavailable: %w", err)
	}

	replies, release, err := listenForKWinReply(conn)
	if err != nil {
		return err
	}
	defer release()

	scriptPath, cleanup, err := writeKWinScript(terminalName, folderName)
	if err != nil {
		return err
	}
	defer cleanup()

	name := fmt.Sprintf("claude-notifications-%d", os.Getpid())
	scripting := conn.Object(kwinService, kwinScriptPath)

	var scriptID int32
	if err := scripting.Call(kwinScriptIface+".loadScript", 0, scriptPath, name).Store(&scriptID); err != nil {
		return fmt.Errorf("kwin loadScript failed: %w", err)
	}
	defer func() {
		var unloaded bool
		_ = scripting.Call(kwinScriptIface+".unloadScript", 0, name).Store(&unloaded)
	}()

	runner := conn.Object(kwinService, dbus.ObjectPath(fmt.Sprintf("%s/Script%d", kwinScriptPath, scriptID)))
	if err := runner.Call(kwinScriptRunner+".run", 0).Store(); err != nil {
		return fmt.Errorf("kwin script run failed: %w", err)
	}

	select {
	case activated := <-replies:
		if !activated {
			return fmt.Errorf("kwin script found no window for %q", terminalName)
		}
		return nil
	case <-time.After(kwinReplyTimeout):
		return fmt.Errorf("kwin script did not report back within %v", kwinReplyTimeout)
	}
}

// listenForKWinReply claims the bus name the script calls back on and returns a
// channel carrying its verdict. The returned func releases the name.
func listenForKWinReply(conn *dbus.Conn) (<-chan bool, func(), error) {
	replies := make(chan bool, 1)

	// DoNotQueue so a stale claim from a crashed run fails fast instead of leaving
	// this call parked behind it.
	reply, err := conn.RequestName(kwinReplyService, dbus.NameFlagDoNotQueue|dbus.NameFlagReplaceExisting)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot claim %s: %w", kwinReplyService, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, nil, fmt.Errorf("%s already owned", kwinReplyService)
	}

	release := func() { _, _ = conn.ReleaseName(kwinReplyService) }

	if err := conn.Export(kwinReplyReceiver{replies: replies}, kwinReplyPath, kwinReplyIface); err != nil {
		release()
		return nil, nil, fmt.Errorf("cannot export reply object: %w", err)
	}

	return replies, release, nil
}

type kwinReplyReceiver struct {
	replies chan<- bool
}

// Report is called by the KWin script once it has tried to activate a window.
func (r kwinReplyReceiver) Report(activated bool) *dbus.Error {
	select {
	case r.replies <- activated:
	default:
	}
	return nil
}

// writeKWinScript renders the focus script to a file, since loadScript takes a path
// rather than source.
func writeKWinScript(terminalName, folderName string) (string, func(), error) {
	source := fmt.Sprintf(kwinFocusScript,
		escapeJS(strings.ToLower(GetKdotoolClass(terminalName))),
		escapeJS(folderName),
		escapeJS(kwinReplyService),
		escapeJS(kwinReplyPath),
		escapeJS(kwinReplyIface),
	)

	file, err := os.CreateTemp("", "claude-notifications-kwin-*.js")
	if err != nil {
		return "", nil, fmt.Errorf("cannot write kwin script: %w", err)
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }

	if _, err := file.WriteString(source); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, fmt.Errorf("cannot write kwin script: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("cannot write kwin script: %w", err)
	}

	// KWin reads the file as the compositor's user; the default 0600 from CreateTemp
	// is enough for that, but the path must not sit in a directory it cannot reach.
	if !filepath.IsAbs(path) {
		cleanup()
		return "", nil, fmt.Errorf("kwin script path is not absolute: %s", path)
	}

	return path, cleanup, nil
}
