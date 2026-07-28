//go:build linux

package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
	"os"
	"strings"
	"testing"
)

func TestWriteKWinScript_RendersMatchers(t *testing.T) {
	path, cleanup, err := writeKWinScript("konsole", "my-project", ":1.42", "/org/kde/kwin/claudenotifications/r1_1")
	if err != nil {
		t.Fatalf("writeKWinScript() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read rendered script: %v", err)
	}
	source := string(body)

	// The class comes from the same mapping kdotool uses, lowercased for the
	// case-insensitive compare the script does.
	if want := strings.ToLower(GetKdotoolClass("konsole")); !strings.Contains(source, want) {
		t.Errorf("rendered script is missing the window class %q", want)
	}
	if !strings.Contains(source, "my-project") {
		t.Error("rendered script is missing the folder name used for caption matching")
	}
	// Asserted as one expression rather than by the presence of its parts. The
	// destination is three plain strings in a single call, so a swapped pair still
	// renders both values and satisfies any check that only asks whether each
	// appears. The cost lands on the reply rather than the run: the script reports
	// where nobody is listening, this call waits out its budget, and focus falls
	// through to a tool KDE users are unlikely to have installed.
	wantCallback := fmt.Sprintf("callDBus(':1.42', '/org/kde/kwin/claudenotifications/r1_1', '%s', 'Report', ok)", kwinReplyIface)
	if !strings.Contains(source, wantCallback) {
		t.Errorf("rendered script does not report back correctly, want %q", wantCallback)
	}
	// Plasma 5 compatibility: both API generations must be handled.
	for _, api := range []string{"windowList", "clientList", "activeWindow", "activeClient"} {
		if !strings.Contains(source, api) {
			t.Errorf("rendered script does not handle %s", api)
		}
	}
}

func TestWriteKWinScript_EscapesFolderName(t *testing.T) {
	// loadScript takes a path, so the folder name reaches the compositor as JS source
	// and has to survive quoting the same way the GNOME Shell Eval path does.
	path, cleanup, err := writeKWinScript("konsole", `evil'); workspace.slotWindowClose(); ('`, ":1.42", "/reply")
	if err != nil {
		t.Fatalf("writeKWinScript() error: %v", err)
	}
	defer cleanup()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read rendered script: %v", err)
	}

	if strings.Contains(string(body), `evil'); workspace`) {
		t.Errorf("folder name was interpolated unescaped:\n%s", body)
	}
	if !strings.Contains(string(body), `\'`) {
		t.Error("expected the quote in the folder name to be escaped")
	}
}

func TestWriteKWinScript_CleanupRemovesFile(t *testing.T) {
	path, cleanup, err := writeKWinScript("konsole", "", ":1.42", "/reply")
	if err != nil {
		t.Fatalf("writeKWinScript() error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("script file should exist before cleanup: %v", err)
	}

	cleanup()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup should remove the script file, stat gave: %v", err)
	}
}

// TestGetFocusMethods_KWinBeforeKdotool pins the ordering. kdotool drives the same
// KWin interface, so trying it first would make the fallback depend on a binary the
// user has to install for no added capability.
func TestGetFocusMethods_KWinBeforeKdotool(t *testing.T) {
	var kwin, kdotool = -1, -1
	for i, m := range GetFocusMethods() {
		switch m.Name {
		case "KWin script":
			kwin = i
		case "kdotool":
			kdotool = i
		}
	}

	if kwin < 0 {
		t.Fatal("KWin script is not in the focus method list")
	}
	if kdotool < 0 {
		t.Fatal("kdotool is not in the focus method list")
	}
	if kwin > kdotool {
		t.Errorf("KWin script runs at %d, after kdotool at %d", kwin, kdotool)
	}
}

// TestExportKWinReply_PathsAreUnique pins the isolation between overlapping calls.
// A single well-known path would let a late reply from a call that already timed out
// be delivered to whichever call is listening next, handing it someone else's verdict.
func TestExportKWinReply_PathsAreUnique(t *testing.T) {
	conn, err := dbus.SessionBus()
	if err != nil {
		t.Skipf("no session bus: %v", err)
	}

	seen := make(map[dbus.ObjectPath]bool)
	for i := 0; i < 4; i++ {
		replyPath, replies, release, err := exportKWinReply(conn)
		if err != nil {
			t.Fatalf("exportKWinReply() error: %v", err)
		}
		defer release()

		if seen[replyPath] {
			t.Errorf("reply path %s was handed out twice", replyPath)
		}
		seen[replyPath] = true

		if replies == nil {
			t.Error("exportKWinReply returned a nil channel")
		}
	}
}

// TestKWinReplyReceiver_DoesNotBlockOnLateReply covers a script reporting after the
// caller has stopped listening. The receiver runs on the D-Bus dispatch goroutine, so
// blocking there would stall every later message on the connection.
func TestKWinReplyReceiver_DoesNotBlockOnLateReply(t *testing.T) {
	replies := make(chan bool, 1)
	receiver := kwinReplyReceiver{replies: replies}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// The second and third have nowhere to go once the buffer is full.
		for i := 0; i < 3; i++ {
			if err := receiver.Report(true); err != nil {
				t.Errorf("Report() returned %v", err)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Report blocked once the channel was full")
	}
}

// recordingBusObject captures the context each call runs under. Only CallWithContext
// is exercised; the embedded interface is nil, so any other method would panic rather
// than pass silently.
type recordingBusObject struct {
	dbus.BusObject
	contexts  []context.Context
	methods   []string
	errAtCall []error
	remaining []time.Duration
	bounded   []bool
}

// CallWithContext samples the context as godbus would see it — at the moment of the
// call. Inspecting it afterwards proves nothing: unloadKWinScript cancels its own
// context on the way out, as it should.
func (r *recordingBusObject) CallWithContext(ctx context.Context, method string, _ dbus.Flags, _ ...any) *dbus.Call {
	r.contexts = append(r.contexts, ctx)
	r.methods = append(r.methods, method)
	r.errAtCall = append(r.errAtCall, ctx.Err())

	deadline, ok := ctx.Deadline()
	r.bounded = append(r.bounded, ok)
	r.remaining = append(r.remaining, time.Until(deadline))

	return &dbus.Call{}
}

// TestUnloadKWinScript_RunsOnItsOwnBudget pins the reason unloadKWinScript takes no
// context from its caller. godbus refuses to send on a cancelled context, so running
// cleanup under the focus budget skipped the unload on exactly the path that creates
// the mess — a timed-out run — and left the script registered with KWin.
func TestUnloadKWinScript_RunsOnItsOwnBudget(t *testing.T) {
	obj := &recordingBusObject{}
	unloadKWinScript(obj, "claude-notifications-r1_1")

	if len(obj.contexts) != 1 {
		t.Fatalf("expected exactly one call, got %d", len(obj.contexts))
	}
	if want := kwinScriptIface + ".unloadScript"; obj.methods[0] != want {
		t.Errorf("called %q, want %q", obj.methods[0], want)
	}
	if err := obj.errAtCall[0]; err != nil {
		t.Errorf("cleanup ran under a cancelled context (%v); godbus would not send it", err)
	}
	if !obj.bounded[0] {
		t.Fatal("cleanup must stay bounded, so an unresponsive compositor cannot hold up the fallback chain")
	}
	if r := obj.remaining[0]; r <= 0 || r > kwinUnloadTimeout {
		t.Errorf("cleanup deadline %v away, want within (0, %v]", r, kwinUnloadTimeout)
	}
}
