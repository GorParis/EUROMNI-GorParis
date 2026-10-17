package seed

import (
	"regexp"
	"strings"
	"testing"

	"euromni/migrations"
)

func TestFixedIDsMatchSeedMigrations(t *testing.T) {
	idsByTable := map[string][]string{
		"shops":      {ShopID},
		"modules":    {ModuleID},
		"categories": {CategoryID},
		"products": {
			ProductID,
			SecondProductID,
			ThirdProductID,
			MugProductID,
			CandleProductID,
		},
		"variants": {
			VariantID,
			SecondVariantID,
			ShirtVariantID,
			MugVariantID,
			CandleVariantID,
		},
	}
	uuidPattern := regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	allIDs := make(map[string]string)
	for table, ids := range idsByTable {
		for _, id := range ids {
			if !uuidPattern.MatchString(id) {
				t.Errorf("%s seed ID %q is not a UUID", table, id)
			}
			if previous, exists := allIDs[id]; exists {
				t.Errorf("seed ID %q is shared by %s and %s", id, previous, table)
			}
			allIDs[id] = table
		}
	}
	if !uuidPattern.MatchString(OtherShop) {
		t.Errorf("other-shop seed ID %q is not a UUID", OtherShop)
	}
	if previous, exists := allIDs[OtherShop]; exists {
		t.Errorf("other-shop ID %q is shared by %s", OtherShop, previous)
	}

	var sql strings.Builder
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migration directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		content, err := migrations.Files.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read embedded migration %s: %v", entry.Name(), err)
		}
		sql.Write(content)
		sql.WriteByte('\n')
	}

	for table, ids := range idsByTable {
		pattern := regexp.MustCompile(`(?is)INSERT\s+INTO\s+` + regexp.QuoteMeta(table) + `\b.*?;`)
		statements := pattern.FindAllString(sql.String(), -1)
		if len(statements) == 0 {
			t.Fatalf("no INSERT statement for seeded table %s found in migrations", table)
		}
		for _, id := range ids {
			found := false
			for _, statement := range statements {
				if strings.Contains(statement, "'"+id+"'") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s seed ID %q is absent from INSERT statements for %s", table, id, table)
			}
		}
	}
}
