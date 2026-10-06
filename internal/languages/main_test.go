package languages_test

import (
	"errors"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func assertValidationErrors(t *testing.T, err error, want ...error) {
	t.Helper()

	if len(want) == 0 {
		if err != nil {
			t.Fatalf("Validate() error = %v, want nil", err)
		}
	} else {
		for _, target := range want {
			if !errors.Is(err, target) {
				t.Errorf("Validate() error = %v, want errors.Is(_, %v)", err, target)
			}
		}
	}
}
