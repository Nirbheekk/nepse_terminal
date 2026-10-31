package repositories_test

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"testing"
)

// snapshotChecksum hashes the snapshot in sorted table order.
//
// Go randomises map iteration, so hashing while ranging over Tables makes the
// digest depend on the run rather than on the data. Sorting the keys is what
// makes the checksum reproducible across processes, which is the whole point of
// verifying a backup.
func snapshotChecksum(tables map[string][]string) string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)

	hasher := sha256.New()
	for _, name := range names {
		hasher.Write([]byte(name))
		for _, row := range tables[name] {
			hasher.Write([]byte(row))
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

// BackupSnapshot simulates a point-in-time logical backup of database tables.
type BackupSnapshot struct {
	Version   string
	Timestamp string
	Tables    map[string][]string // tableName -> SQL insert statements
	Checksum  string
}

func generateMockSnapshot() *BackupSnapshot {
	tables := map[string][]string{
		"companies": {
			"INSERT INTO companies (symbol, name, sector, status) VALUES ('NABIL', 'Nabil Bank Limited', 'Commercial Banks', 'active');",
			"INSERT INTO companies (symbol, name, sector, status) VALUES ('SCB', 'Standard Chartered Bank Nepal', 'Commercial Banks', 'active');",
		},
		"roles": {
			"INSERT INTO roles (code, name) VALUES ('org_admin', 'Organization Administrator');",
			"INSERT INTO roles (code, name) VALUES ('portfolio_manager', 'Portfolio Manager');",
		},
		"licensing_rules": {
			"INSERT INTO licensing_rules (dataset, provider, policy) VALUES ('market_live', 'NEPSE', 'prohibited');",
			"INSERT INTO licensing_rules (dataset, provider, policy) VALUES ('financials', 'ISSUER', 'permitted_with_attribution');",
		},
	}

	return &BackupSnapshot{
		Version:   "0014_developer_keys_exports",
		Timestamp: "2026-10-30T10:00:00Z",
		Tables:    tables,
		Checksum:  snapshotChecksum(tables),
	}
}

func TestBackupGenerationAndIntegrityVerification(t *testing.T) {
	snap := generateMockSnapshot()

	if snap.Checksum == "" {
		t.Fatal("backup snapshot missing checksum")
	}

	// Verify checksum recalculation matches exactly, and that it is stable
	// across repeated runs (the map order must not leak into the digest).
	recalculated := snapshotChecksum(snap.Tables)
	if again := snapshotChecksum(snap.Tables); again != recalculated {
		t.Fatalf("checksum is not reproducible: %s then %s", recalculated, again)
	}

	if recalculated != snap.Checksum {
		t.Fatalf("checksum mismatch: got %s, want %s", recalculated, snap.Checksum)
	}
}

func TestRestorationAndRollbackSimulation(t *testing.T) {
	snap := generateMockSnapshot()

	// 1. Restoration simulation: target state matches snapshot
	restoredState := make(map[string]int)
	for tName, rows := range snap.Tables {
		restoredState[tName] = len(rows)
	}

	if restoredState["companies"] != 2 {
		t.Errorf("expected 2 companies restored, got %d", restoredState["companies"])
	}

	// 2. Rollback simulation: down-migration reverse
	rollbackSteps := []string{
		"DROP TABLE IF EXISTS export_jobs CASCADE;",
		"DROP TABLE IF EXISTS entitlements CASCADE;",
		"DROP TABLE IF EXISTS developer_keys CASCADE;",
	}

	for _, step := range rollbackSteps {
		if !strings.HasPrefix(step, "DROP TABLE IF EXISTS") {
			t.Errorf("unsafe rollback statement: %s", step)
		}
		if !strings.HasSuffix(step, "CASCADE;") {
			t.Errorf("rollback statement should cascade foreign keys: %s", step)
		}
	}
}
