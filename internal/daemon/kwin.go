//go:build linux

// ABOUTME: Window focus for KDE via KWin's D-Bus scripting interface.
// ABOUTME: Needs no external tool, unlike kdotool which wraps the same interface.
package daemon

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	kwinService      = "org.kde.KWin"
	kwinScriptPath   = "/Scripting"
	kwinScriptIface  = "org.kde.kwin.Scripting"
	kwinScriptRunner = "org.kde.kwin.Script"

	// The interface the loaded script reports back on, so a run that matched nothing
	// is a failure and the next focus method still gets its turn. The destination is
	// this connection's unique bus name and a path minted per call: a well-known name
	// could only be held by one caller at a time, and a late reply from a call that
	// already timed out would then be delivered to whoever holds it next.
	kwinReplyIface = "org.kde.kwin.claudenotifications"

	// KWin loads and runs the script asynchronously. Focus is a click response, so
	// the budget is short: either the compositor answers quickly or the caller moves
	// on to another method. It covers the D-Bus round-trips as well as the wait —
	// an unresponsive compositor blocks in loadScript just as effectively.
	kwinReplyTimeout = 2 * time.Second

	// Withdrawing the script gets its own budget rather than the remains of the one
	// above, which on the timeout path is spent by definition. It is a backstop, not
	// a wait: by the time it runs KWin has already answered loadScript and run, and
	// the call itself measures 0.15ms median, 0.8ms worst over 40 samples.
	kwinUnloadTimeout = 250 * time.Millisecond
)

// kwinReplySeq keeps reply paths distinct between overlapping calls in one process.
var kwinReplySeq atomic.Uint64

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

	// One budget for the whole exchange. The reply wait is the obvious part, but a
	// compositor that never answers loadScript would otherwise block here with no
	// bound at all, and the caller would never reach kdotool or xdotool.
	ctx, cancel := context.WithTimeout(context.Background(), kwinReplyTimeout)
	defer cancel()

	replyPath, replies, release, err := exportKWinReply(conn)
	if err != nil {
		return err
	}
	defer release()

	scriptPath, cleanup, err := writeKWinScript(terminalName, folderName, conn.Names()[0], string(replyPath))
	if err != nil {
		return err
	}
	defer cleanup()

	name := fmt.Sprintf("claude-notifications-%s", path.Base(string(replyPath)))
	scripting := conn.Object(kwinService, kwinScriptPath)

	// FlagNoAutoStart so a desktop that merely has Plasma installed cannot be made to
	// launch KWin by a notification click. Without it this method's failure on a
	// non-KDE session would depend on how the distribution packages KWin rather than
	// on anything stated here.
	var scriptID int32
	if err := scripting.CallWithContext(ctx, kwinScriptIface+".loadScript", dbus.FlagNoAutoStart, scriptPath, name).Store(&scriptID); err != nil {
		return fmt.Errorf("kwin loadScript failed: %w", err)
	}
	defer unloadKWinScript(scripting, name)

	runner := conn.Object(kwinService, dbus.ObjectPath(fmt.Sprintf("%s/Script%d", kwinScriptPath, scriptID)))
	if err := runner.CallWithContext(ctx, kwinScriptRunner+".run", dbus.FlagNoAutoStart).Store(); err != nil {
		return fmt.Errorf("kwin script run failed: %w", err)
	}

	select {
	case activated := <-replies:
		if !activated {
			return fmt.Errorf("kwin script found no window for %q", terminalName)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("kwin script did not report back within %v", kwinReplyTimeout)
	}
}

// unloadKWinScript withdraws the loaded script from KWin.
//
// It deliberately takes no context from its caller. The focus budget is spent on the
// very path that most needs cleaning up — a run that timed out — and godbus declines
// to send a message whose context is already cancelled ("short path: don't even send
// the message if context already cancelled", dbus/conn.go). Inheriting it therefore
// meant the script stayed registered exactly when the timeout repeated, so a
// compositor that never reported back accumulated a /Scripting/Script<id> object per
// notification click for as long as it ran.
func unloadKWinScript(scripting dbus.BusObject, name string) {
	ctx, cancel := context.WithTimeout(context.Background(), kwinUnloadTimeout)
	defer cancel()

	var unloaded bool
	_ = scripting.CallWithContext(ctx, kwinScriptIface+".unloadScript", dbus.FlagNoAutoStart, name).Store(&unloaded)
}

// exportKWinReply publishes a receiver on a path unique to this call and returns it
// along with the channel carrying the script's verdict. The returned func withdraws
// the export, so a reply arriving after the caller has given up is discarded rather
// than delivered to whoever runs next.
func exportKWinReply(conn *dbus.Conn) (dbus.ObjectPath, <-chan bool, func(), error) {
	replyPath := dbus.ObjectPath(fmt.Sprintf("/org/kde/kwin/claudenotifications/r%d_%d",
		os.Getpid(), kwinReplySeq.Add(1)))
	replies := make(chan bool, 1)

	if err := conn.Export(kwinReplyReceiver{replies: replies}, replyPath, kwinReplyIface); err != nil {
		return "", nil, nil, fmt.Errorf("cannot export reply object: %w", err)
	}

	release := func() { _ = conn.Export(nil, replyPath, kwinReplyIface) }
	return replyPath, replies, release, nil
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
func writeKWinScript(terminalName, folderName, replyService, replyPath string) (string, func(), error) {
	source := fmt.Sprintf(kwinFocusScript,
		escapeJS(strings.ToLower(GetKdotoolClass(terminalName))),
		escapeJS(folderName),
		escapeJS(replyService),
		escapeJS(replyPath),
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
