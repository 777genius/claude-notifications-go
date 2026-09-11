package installruntime

import "fmt"

// PathAnchor binds a staged destination's existing parents to their opened
// filesystem identities. Recovery must not reinterpret a substituted directory.
type PathAnchor struct{ Path, Identity string }

// PhysicalPath rewrites only Darwin root-owned /var, /tmp, and /etc aliases.
// Arbitrary user-directory symlinks are not canonicalized.
func PhysicalPath(path string) (string, error) {
	return platformAnchorPath(path)
}

func checkAnchors(want, got []PathAnchor) error {
	for i, a := range want {
		if i >= len(got) || a != got[i] {
			return fmt.Errorf("managed destination parent substituted: %s", a.Path)
		}
	}
	return nil
}
