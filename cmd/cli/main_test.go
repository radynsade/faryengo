package main

import (
	"bytes"
	"errors"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestRunWithoutDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	for _, tt := range []struct {
		name       string
		args       []string
		wantStdout string
		wantStderr string
		wantErr    error
	}{
		{name: "help", args: []string{"help"}, wantStdout: usage + "\n"},
		{name: "help shorthand", args: []string{"-h"}, wantStdout: usage + "\n"},
		{name: "help option", args: []string{"--help"}, wantStdout: usage + "\n"},
		{name: "missing command", wantStderr: usage + "\n", wantErr: ErrCommandInvalid},
		{name: "missing action", args: []string{"languages"}, wantStderr: usage + "\n", wantErr: ErrCommandInvalid},
		{
			name:       "malformed create",
			args:       []string{"languages", "create-language", "lv"},
			wantStderr: usage + "\n",
			wantErr:    ErrCommandInvalid,
		},
		{
			name:    "missing connection for create",
			args:    []string{"languages", "create-language", "lv", "Latvian", "Latviešu"},
			wantErr: ErrDatabaseURLMissing,
		},
		{
			name:    "missing connection for delete",
			args:    []string{"languages", "delete-language", "lv"},
			wantErr: ErrDatabaseURLMissing,
		},
		{
			name:    "missing connection for role creation",
			args:    []string{"users", "create-role", "en:Administrator", "--super"},
			wantErr: ErrDatabaseURLMissing,
		},
		{
			name:    "missing connection for user creation",
			args:    createUserArguments(),
			wantErr: ErrDatabaseURLMissing,
		},
		{
			name:       "malformed translations before connection",
			args:       []string{"users", "create-role", "broken"},
			wantStderr: usage + "\n",
			wantErr:    ErrTranslationsInvalid,
		},
		{
			name:       "malformed UUID before connection",
			args:       []string{"users", "delete-user", "invalid-uuid"},
			wantStderr: usage + "\n",
			wantErr:    ErrUUIDInvalid,
		},
		{
			name:       "unknown group",
			args:       []string{"security", "delete-user", testUserUUID},
			wantStderr: usage + "\n",
			wantErr:    ErrCommandInvalid,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := run(t.Context(), tt.args, &stdout, &stderr)

			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Fatalf("run(%v) error = %v, want %v", tt.args, err, tt.wantErr)
			}

			if stdout.String() != tt.wantStdout || stderr.String() != tt.wantStderr {
				t.Fatalf("run(%v) output = (%q, %q), want (%q, %q)", tt.args, stdout.String(), stderr.String(), tt.wantStdout, tt.wantStderr)
			}
		})
	}
}
