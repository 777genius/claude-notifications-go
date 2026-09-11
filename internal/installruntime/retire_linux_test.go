package installruntime

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Exercise production transactions on disposable Linux directories using the
// kernel's atomic exchange. This does not qualify Darwin lsof or LaunchServices.
func TestRetirementTransitionAndRecovery(t *testing.T) {
	oldExchange, oldDrain := nativeExchange, nativeDrainCheck
	defer func() { nativeExchange, nativeDrainCheck = oldExchange, oldDrain }()
	nativeExchange = func(a, b string, expected ...[]PathAnchor) error {
		return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
	}
	for _, tc := range []struct{ crash, legacy bool }{{}, {crash: true}, {legacy: true}, {crash: true, legacy: true}} {
		t.Run(fmt.Sprintf("crash=%v/legacy=%v", tc.crash, tc.legacy), func(t *testing.T) {
			crash := tc.crash
			ctx, r := request(t)
			r.Consumer.Commands = []string{"existing hook --literal"}
			stage := func(label string) *NativeChange {
				source := nativeFixture(t)
				if err := os.WriteFile(filepath.Join(source, "version"), []byte(label), 0600); err != nil {
					t.Fatal(err)
				}
				c, err := StageNative(ctx, r.ControlRoot, source)
				if err != nil {
					t.Fatal(err)
				}
				c.After.DecoderFloor = 1
				return c
			}
			r.Native = stage("A")
			a, err := Commit(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			r.Native = stage("B")
			if tc.legacy {
				// Model an older allocator before commit, never migrate a retained record.
				legacy := strings.TrimSuffix(r.Native.Staged, ".app")
				if err := os.Rename(r.Native.Staged, legacy); err != nil {
					t.Fatal(err)
				}
				r.Native.Staged = legacy
			}
			candidate := r.Native
			assertBundles := func() {
				t.Helper()
				for path, want := range map[string]NativeRecord{candidate.Staged: *a.Native, candidate.After.Path: candidate.After} {
					if hash, err := treeFingerprint(path); err != nil || hash != want.SHA256 {
						t.Fatalf("bundle bytes at %s: %s %v", path, hash, err)
					}
					if err := checkNativeDirectoryID(path, want.DirectoryID); err != nil {
						t.Fatal(err)
					}
				}
			}
			if crash {
				r.Fault = func(phase string) error {
					if phase == "native" {
						return fmt.Errorf("crash after exchange")
					}
					return nil
				}
				if _, err := Commit(ctx, r); err == nil {
					t.Fatal("missing promotion crash")
				}
				if err := DiscardNative(ctx, r.ControlRoot, candidate); err != nil {
					t.Fatal(err)
				}
				assertBundles()
				r.Fault, r.Native = nil, nil
			}
			b, err := Commit(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if b.Native.Path != a.Native.Path || b.Native.PreviousPath == "" {
				t.Fatal("stable reader or predecessor lost")
			}
			if b.Native.PreviousPath != candidate.Staged || b.Native.PreviousSHA256 != a.Native.SHA256 || b.Native.PreviousDirectoryID != a.Native.DirectoryID {
				t.Fatal("predecessor record lost exact path/hash/inode")
			}
			if err := DiscardNative(ctx, r.ControlRoot, candidate); err != nil {
				t.Fatal(err)
			}
			assertBundles()
			r.Native = nil
			if again, err := Commit(ctx, r); err != nil || !reflect.DeepEqual(again.Native, b.Native) {
				t.Fatalf("repeat recovery changed native record: %v", err)
			} else {
				b = again
			}
			c := stage("C")
			r.Native = c
			if _, err = Commit(ctx, r); err == nil {
				t.Fatal("third artifact accepted without retirement")
			}
			r.Native = nil
			registeredConsumer := r.Consumer
			r.Consumer = Consumer{}
			r.RetireNative = true
			r.ExpectedGeneration = &b.Generation
			nativeDrainCheck = func(context.Context, string) error { return fmt.Errorf("owned resource busy") }
			if _, err = Commit(ctx, r); err == nil {
				t.Fatal("busy predecessor retired")
			}
			if _, err = os.Stat(b.Native.PreviousPath); err != nil {
				t.Fatal("busy predecessor removed")
			}
			scans := 0
			nativeDrainCheck = func(ctx context.Context, path string) error {
				short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
				_, release, admissionErr := AcquireInstalledLease(short, r.ControlRoot, InstalledSnapshot{})
				cancel()
				if admissionErr == nil {
					release()
					return fmt.Errorf("new owned launch admitted during drain")
				}
				if !errors.Is(admissionErr, context.DeadlineExceeded) {
					return fmt.Errorf("drain did not hold component lease: %w", admissionErr)
				}
				scans++
				if path != b.Native.PreviousPath {
					return fmt.Errorf("wrong predecessor")
				}
				return nil
			}
			if crash {
				r.Fault = func(p string) error {
					if strings.HasPrefix(p, "purge-entry:") {
						return fmt.Errorf("crash during retirement")
					}
					return nil
				}
			}
			retired, err := Commit(ctx, r)
			if crash {
				if err == nil {
					t.Fatal("missing interior crash")
				}
				r.Fault = nil
				// Recovery commits the original retirement before the next request's CAS.
				r.ExpectedGeneration = nil
				r.RetireNative = false
				r.Consumer = registeredConsumer
				retired, err = Commit(ctx, r)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(retired.Consumers, b.Consumers) || retired.Enabled != b.Enabled {
				t.Fatal("retirement mutated consumer registration or eligibility")
			}
			if scans == 0 || retired.Native.PreviousPath != "" || retired.Native.Path != b.Native.Path || retired.Native.SHA256 != b.Native.SHA256 {
				t.Fatal("invalid retirement")
			}
			if _, err = os.Stat(b.Native.PreviousPath); !os.IsNotExist(err) {
				t.Fatal("predecessor remains")
			}
			r.RetireNative = false
			r.ExpectedGeneration = nil
			r.Native = c
			c.Before = *retired.Native
			final, err := Commit(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if final.Native.SHA256 != c.After.SHA256 || final.Native.PreviousSHA256 != b.Native.SHA256 || final.Native.Path != a.Native.Path {
				t.Fatal("B to C lost compatible stable path")
			}
		})
	}
}
