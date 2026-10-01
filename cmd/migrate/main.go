package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/radynsade/faryengo/db/migrations"
	"github.com/radynsade/faryengo/internal/config"
)

const migrationLockKey int64 = 0x66617279656e6d67

var migrationFilePattern = regexp.MustCompile(`^([0-9]+)_([a-z0-9_]+)\.(up|down)\.sql$`)

type migration struct {
	version  int64
	name     string
	up       []byte
	down     []byte
	checksum string
}

type appliedMigration struct {
	name     string
	checksum string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}

	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	databaseURL := flags.String("database", settings.DatabaseURL, "PostgreSQL connection string (defaults to DATABASE_URL)")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: migrate [-database URL] up|down|status")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return fmt.Errorf("parse migration arguments: %w", err)
	}

	if flags.NArg() != 1 || !slices.Contains([]string{"up", "down", "status"}, flags.Arg(0)) {
		flags.Usage()
		return errors.New("expected one of: up, down, status")
	}

	if strings.TrimSpace(*databaseURL) == "" {
		return errors.New("PostgreSQL connection string is required: set DATABASE_URL or -database")
	}

	files, err := loadMigrations(migrations.Files)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	if len(files) == 0 || files[0].version != 0 {
		return errors.New("version-zero migration is required to create public.schema_migration")
	}

	pool, err := pgxpool.New(ctx, *databaseURL)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL connection: %w", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", migrationLockKey).Scan(&locked); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}

	if !locked {
		return errors.New("another migration command is running")
	}

	applied, err := readApplied(ctx, conn)
	if err != nil {
		return err
	}

	if err := validateApplied(files, applied); err != nil {
		return fmt.Errorf("validate migration history: %w", err)
	}

	switch flags.Arg(0) {
	case "up":
		err = migrateUp(ctx, conn, files, applied, stdout)
	case "down":
		err = migrateDown(ctx, conn, files, applied, stdout)
	case "status":
		err = printStatus(files, applied, stdout)
	}

	return err
}

func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}

	byVersion := make(map[int64]*migration)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := migrationFilePattern.FindStringSubmatch(entry.Name())
		if parts == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		version, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}

		contents, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if len(strings.TrimSpace(string(contents))) == 0 {
			return nil, fmt.Errorf("migration %s is empty", entry.Name())
		}

		file := byVersion[version]
		if file == nil {
			file = &migration{version: version, name: parts[2]}
			byVersion[version] = file
		} else if file.name != parts[2] {
			return nil, fmt.Errorf("migration version %d has conflicting names", version)
		}

		switch parts[3] {
		case "up":
			if file.up != nil {
				return nil, fmt.Errorf("migration version %d has duplicate up files", version)
			}

			file.up = contents
		case "down":
			if file.down != nil {
				return nil, fmt.Errorf("migration version %d has duplicate down files", version)
			}

			file.down = contents
		}
	}

	files := make([]migration, 0, len(byVersion))
	for _, version := range slices.Sorted(maps.Keys(byVersion)) {
		file := byVersion[version]
		if file.up == nil || file.down == nil {
			return nil, fmt.Errorf("migration version %d needs both up and down files", file.version)
		}

		file.checksum = fmt.Sprintf("%x", sha256.Sum256(bytes.Join([][]byte{file.up, file.down}, []byte{0})))
		files = append(files, *file)
	}

	return files, nil
}

func readApplied(ctx context.Context, conn *pgxpool.Conn) (map[int64]appliedMigration, error) {
	var tableExists bool
	var legacyTableExists bool
	if err := conn.QueryRow(ctx, `
		SELECT to_regclass('public.schema_migration') IS NOT NULL,
		       to_regclass('public.schema_migrations') IS NOT NULL
	`).Scan(&tableExists, &legacyTableExists); err != nil {
		return nil, fmt.Errorf("check migration history table: %w", err)
	}

	if legacyTableExists {
		return nil, errors.New("legacy public.schema_migrations exists; reconcile its history before using public.schema_migration")
	}

	applied := make(map[int64]appliedMigration)
	if !tableExists {
		return applied, nil
	}

	rows, err := conn.Query(ctx, "SELECT version, name, checksum FROM public.schema_migration ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("read migration history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var version int64
		var item appliedMigration
		if err := rows.Scan(&version, &item.name, &item.checksum); err != nil {
			return nil, fmt.Errorf("scan migration history: %w", err)
		}

		applied[version] = item
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read migration history rows: %w", err)
	}

	return applied, nil
}

func validateApplied(files []migration, applied map[int64]appliedMigration) error {
	known := make(map[int64]bool, len(files))
	seenPending := false
	for _, file := range files {
		known[file.version] = true
		item, exists := applied[file.version]
		if !exists {
			seenPending = true
			continue
		}

		if seenPending {
			return fmt.Errorf("migration %d is applied after a missing earlier migration", file.version)
		}

		if item.name != file.name || item.checksum != file.checksum {
			return fmt.Errorf("migration %d differs from its applied file", file.version)
		}
	}

	for _, version := range slices.Sorted(maps.Keys(applied)) {
		if !known[version] {
			return fmt.Errorf("database migration %d is absent from the binary", version)
		}
	}

	return nil
}

func migrateUp(ctx context.Context, conn *pgxpool.Conn, files []migration, applied map[int64]appliedMigration, output io.Writer) error {
	changed := false
	for _, file := range files {
		if _, exists := applied[file.version]; exists {
			continue
		}

		if err := applyMigration(ctx, conn, file, true); err != nil {
			return err
		}

		changed = true
		if _, err := fmt.Fprintf(output, "applied %06d %s\n", file.version, file.name); err != nil {
			return fmt.Errorf("write migration result: %w", err)
		}
	}

	if !changed {
		if _, err := fmt.Fprintln(output, "no pending migrations"); err != nil {
			return fmt.Errorf("write migration result: %w", err)
		}
	}

	return nil
}

func migrateDown(ctx context.Context, conn *pgxpool.Conn, files []migration, applied map[int64]appliedMigration, output io.Writer) error {
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if _, exists := applied[file.version]; !exists {
			continue
		}

		if err := applyMigration(ctx, conn, file, false); err != nil {
			return err
		}

		if _, err := fmt.Fprintf(output, "rolled back %06d %s\n", file.version, file.name); err != nil {
			return fmt.Errorf("write migration result: %w", err)
		}

		return nil
	}

	if _, err := fmt.Fprintln(output, "no applied migrations"); err != nil {
		return fmt.Errorf("write migration result: %w", err)
	}

	return nil
}

func applyMigration(ctx context.Context, conn *pgxpool.Conn, file migration, up bool) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", file.version, err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = tx.Rollback(rollbackCtx)
	}()

	if !up {
		if _, err := tx.Exec(ctx, "DELETE FROM public.schema_migration WHERE version = $1", file.version); err != nil {
			return fmt.Errorf("remove migration %d history: %w", file.version, err)
		}
	}

	query := file.down
	if up {
		query = file.up
	}

	if _, err := tx.Exec(ctx, string(query), pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("execute migration %d: %w", file.version, err)
	}

	if up {
		if _, err := tx.Exec(ctx, "INSERT INTO public.schema_migration (version, name, checksum) VALUES ($1, $2, $3)", file.version, file.name, file.checksum); err != nil {
			return fmt.Errorf("record migration %d: %w", file.version, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %d: %w", file.version, err)
	}

	return nil
}

func printStatus(files []migration, applied map[int64]appliedMigration, output io.Writer) error {
	for _, file := range files {
		status := "pending"
		if _, exists := applied[file.version]; exists {
			status = "applied"
		}

		if _, err := fmt.Fprintf(output, "%06d %s %s\n", file.version, file.name, status); err != nil {
			return fmt.Errorf("write migration status: %w", err)
		}
	}

	return nil
}
