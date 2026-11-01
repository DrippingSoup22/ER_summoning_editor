package application

import (
	"testing"
)

func newService(t *testing.T) *Service {
	t.Helper()
	service, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return service
}
