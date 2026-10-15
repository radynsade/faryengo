package pgxdb

import "testing"

func TestLikeSubstring(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"plain", "ada", "%ada%"},
		{"empty", "", "%%"},
		{"wildcards", "50%_off", `%50\%\_off%`},
		{"backslash", `a\b`, `%a\\b%`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := LikeSubstring(test.value); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
