package application

import (
	"strings"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
)

// The catalog's needs and the save core's surface are one list written
// twice, and the service refuses to start when they drift.
func TestTheCatalogAndTheSaveCoreAgreeOnTheSharedWrites(t *testing.T) {
	t.Parallel()
	needs, err := catalog.LoadSharedEffectNeeds()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkSharedEffectNeeds(needs); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		mutate func([]catalog.SharedEffectNeed) []catalog.SharedEffectNeed
		want   string
	}{
		{"a field the save core may not write", func(n []catalog.SharedEffectNeed) []catalog.SharedEffectNeed {
			n[0].Row = 13614
			return n
		}, "may not write"},
		{"another value", func(n []catalog.SharedEffectNeed) []catalog.SharedEffectNeed {
			n[0].Value = 44
			return n
		}, "moves"},
		{"a field no entry needs", func(n []catalog.SharedEffectNeed) []catalog.SharedEffectNeed {
			return n[1:]
		}, "no catalog entry needs"},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := test.mutate(append([]catalog.SharedEffectNeed(nil), needs...))
			if err := checkSharedEffectNeeds(changed); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}
