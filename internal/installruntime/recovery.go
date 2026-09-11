package installruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// A checksum detects valid-JSON corruption of the recovery record. It is not
// authentication against the user who owns the private control directory.
type transactionEnvelope struct {
	SHA256      string
	Transaction json.RawMessage
}

func writeTransaction(path string, tx transaction) error {
	data, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	return writeJSON(path, transactionEnvelope{hex.EncodeToString(digest[:]), data})
}
func decodeTransaction(data []byte) (transaction, error) {
	var tx transaction
	var envelope transactionEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return tx, err
	}
	// MarshalIndent reformats embedded RawMessage; checksum canonical compact JSON.
	var compact bytes.Buffer
	if err := json.Compact(&compact, envelope.Transaction); err != nil {
		return tx, err
	}
	digest := sha256.Sum256(compact.Bytes())
	if envelope.SHA256 != hex.EncodeToString(digest[:]) {
		return tx, fmt.Errorf("transaction checksum mismatch")
	}
	if err := json.Unmarshal(envelope.Transaction, &tx); err != nil {
		return tx, err
	}
	if (tx.Schema != transactionSchemaV1 && tx.Schema != transactionSchemaV2) || tx.After.Generation <= tx.Before.Generation || tx.After.PolicyGeneration <= tx.Before.PolicyGeneration || tx.After.ID == "" || tx.After.Consumers == nil || tx.After.Files == nil || tx.Before.Consumers == nil || tx.Before.Files == nil {
		return tx, fmt.Errorf("invalid transaction schema")
	}
	seen := map[string]bool{}
	for _, f := range tx.Files {
		if !filepath.IsAbs(f.Path) || seen[f.Path] {
			return tx, fmt.Errorf("invalid transaction path")
		}
		seen[f.Path] = true
		if f.Before.Exists && f.Before.Link == "" && identity(f.BeforeData, f.Before.Mode) != f.Before {
			return tx, fmt.Errorf("invalid per-file recovery identity")
		}
	}
	return tx, nil
}

type runtimePolicy struct {
	Generation uint64
	Enabled    bool
}

func checkPolicyGeneration(root string, l Ledger) error {
	data, err := readRegularFile(filepath.Join(root, "policy-generation.json"))
	if os.IsNotExist(err) {
		if l.Generation != 0 {
			return fmt.Errorf("missing policy generation")
		}
		return nil
	}
	if err != nil {
		return err
	}
	var policy runtimePolicy
	if json.Unmarshal(data, &policy) != nil || policy.Enabled != l.Enabled || policy.Generation != l.PolicyGeneration {
		return fmt.Errorf("policy/ledger mismatch without transaction; refusing ambiguous repair")
	}
	return nil
}

// reverseTransaction builds a new durable decision from checked individual
// snapshots. It never restores an entire directory and never downgrades an
// already promoted callback reader. A foreign edit makes rollback refuse.
func reverseTransaction(current Ledger, tx transaction) (transaction, error) {
	if tx.Native != nil && tx.Native.Retire {
		return transaction{}, fmt.Errorf("retirement deletion must finish forward recovery before rollback")
	}
	after := tx.Before
	after.Enabled = false
	after.WriterFloor = WriterFloor
	after.Generation = tx.After.Generation + 1
	after.PolicyGeneration = tx.After.PolicyGeneration + 1
	if after.ID == "" {
		after.ID = tx.After.ID
		after.Owner = tx.After.Owner
		after.RuntimeRoot = tx.After.RuntimeRoot
	}
	if tx.Native != nil {
		digest, err := treeFingerprint(tx.Native.After.Path)
		if err != nil {
			return transaction{}, err
		}
		if digest == tx.Native.After.SHA256 && !tx.Native.Purge {
			after.Native = tx.After.Native
			after.DecoderFloor = tx.After.DecoderFloor
		} else if digest != tx.Native.Before.SHA256 {
			return transaction{}, fmt.Errorf("cannot rollback an ambiguous or purged native callback")
		}
	}
	reverse := transaction{Schema: transactionSchemaV2, Before: current, After: after, ConfigPaths: tx.ConfigPaths}
	for _, f := range tx.Files {
		if err := checkReplacementRollback(f); err != nil {
			return transaction{}, err
		}
		actual, err := Fingerprint(f.Path)
		if err != nil {
			return transaction{}, err
		}
		if actual != f.Before && actual != desired(f) {
			return transaction{}, fmt.Errorf("rollback conflict; preserving foreign edit: %s", f.Path)
		}
		undo := File{Parents: f.Parents, Path: f.Path, Before: actual, Data: f.BeforeData, Mode: f.Before.Mode, Link: f.Before.Link, Remove: !f.Before.Exists}
		if actual.Exists && actual.Link == "" {
			undo.WindowsReplacementID, err = windowsReplacementIdentity(f.Path)
			if err != nil {
				return transaction{}, err
			}
			undo.BeforeData, err = readRegularFile(f.Path)
			if err != nil {
				return transaction{}, err
			}
		}
		// A rollback to a pre-managed installation retains the compatible kernel
		// needed to maintain the callback/policy protocol. It is not an implicit reset.
		if managedWriter(f.Path) && undo.Remove && actual.Exists {
			after.Files[f.Path] = actual
			continue
		}
		reverse.Files = append(reverse.Files, undo)
	}
	if err := validateWriterFiles(reverse.Files); err != nil {
		return transaction{}, err
	}
	return reverse, nil
}
