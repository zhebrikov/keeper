package errors_test

import (
	"errors"
	"testing"

	domainerrors "github.com/zhebrikov/gophkeeper/internal/domain/errors"
)

func TestDomainErrors(t *testing.T) {
	cases := []error{
		domainerrors.ErrNotFound,
		domainerrors.ErrAlreadyExists,
		domainerrors.ErrInvalidCredentials,
		domainerrors.ErrUnauthorized,
		domainerrors.ErrConflict,
		domainerrors.ErrInvalidInput,
	}
	for _, err := range cases {
		if err == nil || err.Error() == "" {
			t.Fatalf("unexpected error value: %v", err)
		}
		if !errors.Is(err, err) {
			t.Fatalf("errors.Is failed for %v", err)
		}
	}
}
