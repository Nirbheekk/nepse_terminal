package migrations_test

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"nepse/terminal/api/migrations"
)

func TestMigrationFilesCompletenessAndPairing(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("failed to read migrations embed FS: %v", err)
	}

	upFiles := make(map[string]string)
	downFiles := make(map[string]string)

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		contentBytes, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatalf("failed to read %s: %v", name, err)
		}
		content := strings.TrimSpace(string(contentBytes))
		if len(content) == 0 {
			t.Errorf("migration file %s is empty", name)
		}

		parts := strings.Split(name, ".")
		if len(parts) < 3 {
			t.Errorf("migration file name %s does not match <id>_<name>.<up|down>.sql format", name)
			continue
		}

		base := parts[0]
		direction := parts[1]

		if direction == "up" {
			upFiles[base] = name
		} else if direction == "down" {
			downFiles[base] = name
		} else {
			t.Errorf("unexpected migration direction %s in %s", direction, name)
		}
	}

	// Verify all up migrations have a matching down migration
	for base, upName := range upFiles {
		if _, ok := downFiles[base]; !ok {
			t.Errorf("missing .down.sql pair for %s", upName)
		}
	}

	// Verify all down migrations have a matching up migration
	for base, downName := range downFiles {
		if _, ok := upFiles[base]; !ok {
			t.Errorf("orphaned .down.sql file %s without matching .up.sql", downName)
		}
	}

	// Verify minimum expected migration count (14 migration pairs)
	const expectedMinMigrations = 16
	if len(upFiles) < expectedMinMigrations {
		t.Errorf("expected at least %d migrations, found %d", expectedMinMigrations, len(upFiles))
	}

	// Verify chronological ordering 0001 -> 0014
	for i := 1; i <= len(upFiles); i++ {
		prefix := fmt.Sprintf("%04d", i)
		found := false
		for base := range upFiles {
			if strings.HasPrefix(base, prefix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing sequential migration prefix %s", prefix)
		}
	}
}

func TestMigrationSQLIdempotencyAndCleanups(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("failed to read migrations: %v", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		contentBytes, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			t.Fatalf("failed to read %s: %v", name, err)
		}
		content := string(contentBytes)

		// Down migrations should have DROP or DELETE statements
		if strings.Contains(name, ".down.sql") {
			if !strings.Contains(content, "DROP") && !strings.Contains(content, "DELETE") {
				t.Errorf("down migration %s should contain DROP or DELETE statements", name)
			}
			if strings.Contains(content, "DROP TABLE") && !strings.Contains(content, "IF EXISTS") {
				t.Errorf("down migration %s should use DROP TABLE IF EXISTS for safe rollback", name)
			}
		}

		// Check for balanced quotes and comments
		openQuotes := strings.Count(content, "'")
		if openQuotes%2 != 0 {
			t.Errorf("migration %s has unbalanced single quotes (count: %d)", name, openQuotes)
		}
	}
}
