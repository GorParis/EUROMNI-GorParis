package migrations

import (
	"testing"
	"testing/fstest"
)

func TestListRejectsDuplicateMigrationVersion(t *testing.T) {
	source := fstest.MapFS{
		"0001_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		"0001_second.sql":  &fstest.MapFile{Data: []byte("SELECT 2;")},
	}
	if _, err := list(source); err == nil {
		t.Fatal("list() accepted duplicate migration versions")
	}
}

func TestListRejectsInvalidMigrationFilename(t *testing.T) {
	source := fstest.MapFS{
		"1_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	if _, err := list(source); err == nil {
		t.Fatal("list() accepted an invalid migration filename")
	}
}
