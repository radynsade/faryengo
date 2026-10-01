package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/radynsade/faryengo/db/migrations"
)

func TestLoadMigrations(t *testing.T) {
	for _, tt := range []struct {
		name      string
		files     fstest.MapFS
		wantCount int
		wantFirst int64
		wantErr   string
	}{
		{
			name: "sorted paired migrations",
			files: fstest.MapFS{
				"000002_second.up.sql":   {Data: []byte("SELECT 2")},
				"000002_second.down.sql": {Data: []byte("SELECT 2")},
				"000001_first.up.sql":    {Data: []byte("SELECT 1")},
				"000001_first.down.sql":  {Data: []byte("SELECT 1")},
			},
			wantCount: 2,
			wantFirst: 1,
		},
		{
			name: "version zero",
			files: fstest.MapFS{
				"000000_bootstrap.up.sql":   {Data: []byte("CREATE TABLE schema_migration (version bigint)")},
				"000000_bootstrap.down.sql": {Data: []byte("DROP TABLE schema_migration")},
			},
			wantCount: 1,
			wantFirst: 0,
		},
		{
			name: "missing down file",
			files: fstest.MapFS{
				"000001_first.up.sql": {Data: []byte("SELECT 1")},
			},
			wantErr: "needs both up and down files",
		},
		{
			name: "invalid filename",
			files: fstest.MapFS{
				"first.up.sql": {Data: []byte("SELECT 1")},
			},
			wantErr: "invalid migration filename",
		},
		{
			name: "empty migration",
			files: fstest.MapFS{
				"000001_first.up.sql": {Data: []byte(" \n ")},
			},
			wantErr: "is empty",
		},
		{
			name: "conflicting names",
			files: fstest.MapFS{
				"000001_first.up.sql":    {Data: []byte("SELECT 1")},
				"000001_second.down.sql": {Data: []byte("SELECT 1")},
			},
			wantErr: "conflicting names",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files, err := loadMigrations(tt.files)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("loadMigrations() error = %v, want %q", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("loadMigrations() error = %v", err)
			}

			if len(files) != tt.wantCount || files[0].version != tt.wantFirst || files[0].checksum == "" {
				t.Fatalf("loadMigrations() = %+v, want %d sorted files with checksums", files, tt.wantCount)
			}
		})
	}
}

func TestValidateApplied(t *testing.T) {
	files := []migration{
		{version: 1, name: "first", checksum: "abc"},
		{version: 2, name: "second", checksum: "def"},
	}

	for _, tt := range []struct {
		name    string
		applied map[int64]appliedMigration
		wantErr string
	}{
		{name: "none", applied: map[int64]appliedMigration{}},
		{name: "prefix", applied: map[int64]appliedMigration{1: {name: "first", checksum: "abc"}}},
		{name: "all", applied: map[int64]appliedMigration{1: {name: "first", checksum: "abc"}, 2: {name: "second", checksum: "def"}}},
		{name: "changed file", applied: map[int64]appliedMigration{1: {name: "first", checksum: "old"}}, wantErr: "differs from its applied file"},
		{name: "gap", applied: map[int64]appliedMigration{2: {name: "second", checksum: "def"}}, wantErr: "applied after a missing earlier migration"},
		{name: "unknown version", applied: map[int64]appliedMigration{3: {name: "third", checksum: "ghi"}}, wantErr: "absent from the binary"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateApplied(files, tt.applied)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("validateApplied() error = %v", err)
			}

			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("validateApplied() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestPrintStatus(t *testing.T) {
	files := []migration{
		{version: 1, name: "first"},
		{version: 2, name: "second"},
	}
	applied := map[int64]appliedMigration{1: {name: "first"}}
	var output bytes.Buffer

	if err := printStatus(files, applied, &output); err != nil {
		t.Fatalf("printStatus() error = %v", err)
	}

	want := "000001 first applied\n000002 second pending\n"
	if output.String() != want {
		t.Fatalf("printStatus() = %q, want %q", output.String(), want)
	}
}

func TestRunRequiresConnectionString(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var output bytes.Buffer
	var errors bytes.Buffer

	err := run(context.Background(), []string{"up"}, &output, &errors)
	if err == nil || !strings.Contains(err.Error(), "connection string is required") {
		t.Fatalf("run(up) error = %v, want missing connection string", err)
	}
}

func TestBundledMigrations(t *testing.T) {
	files, err := loadMigrations(migrations.Files)
	if err != nil {
		t.Fatalf("loadMigrations(embedded files) error = %v", err)
	}

	if len(files) == 0 || files[0].version != 0 {
		t.Fatalf("loadMigrations(embedded files) = %+v, want version zero first", files)
	}

	if !strings.Contains(string(files[0].up), "CREATE TABLE public.schema_migration") ||
		!strings.Contains(string(files[0].down), "DROP TABLE public.schema_migration") {
		t.Fatal("version-zero migration must create and drop public.schema_migration")
	}
}

func TestMigrationChecksumIncludesDownSQL(t *testing.T) {
	files := fstest.MapFS{
		"000001_first.up.sql":   {Data: []byte("SELECT 1")},
		"000001_first.down.sql": {Data: []byte("SELECT 2")},
	}

	original, err := loadMigrations(files)
	if err != nil {
		t.Fatalf("loadMigrations(original) error = %v", err)
	}

	files["000001_first.down.sql"] = &fstest.MapFile{Data: []byte("SELECT 3")}
	changed, err := loadMigrations(files)
	if err != nil {
		t.Fatalf("loadMigrations(changed) error = %v", err)
	}

	if original[0].checksum == changed[0].checksum {
		t.Fatal("changing down SQL did not change the migration checksum")
	}
}

func TestValidateAppliedRequiresZeroBeforeLaterVersions(t *testing.T) {
	files := []migration{
		{version: 0, name: "bootstrap", checksum: "zero"},
		{version: 1, name: "first", checksum: "one"},
	}
	applied := map[int64]appliedMigration{1: {name: "first", checksum: "one"}}

	err := validateApplied(files, applied)
	if err == nil || !strings.Contains(err.Error(), "missing earlier migration") {
		t.Fatalf("validateApplied() error = %v, want missing version zero", err)
	}
}
