package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
)

// The reported output digest describes the file on disk, grants included.
//
// It did not, and the bug was invisible from inside: grants are written into the
// output AFTER the save core has produced its summary, so a commit that granted
// an item reported the digest of the file as it stood one write earlier. The
// session reloads the output at the end, so its own view was right and only the
// number handed to the caller was wrong - which is the worst shape for a defect
// in an audit artifact.
//
// It surfaced when a research manifest pinned to that digest refused the file it
// was built from.
func TestTheReportedDigestIsTheFileOnDisk(t *testing.T) {
	t.Parallel()
	service, err := application.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// The synthetic fixture is built from the shipped regulation and carries no
	// characters, and a grant is exactly what this test needs, so it uses the
	// real save like every other slot test and skips without one.
	session, people := realCharacters(t)
	_ = service
	slot := -1
	for index, person := range people {
		if person.Used {
			slot = index
			break
		}
	}
	if slot < 0 {
		t.Skip("the save holds no character")
	}
	service = newService(t)
	// A grant is the whole point: without one the digest was already right.
	granted := false
	for _, ash := range service.Ashes() {
		if _, err := session.StageGrant(slot, ash.GoodsFamily, 0); err == nil {
			granted = true
			break
		}
	}
	if !granted {
		t.Skip("the fixture character already owns every Ash")
	}

	out := filepath.Join(t.TempDir(), "granted.dat")
	commit, err := session.PrepareCommit()
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Commit(ctx, commit, out, "")
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(written)
	if want := hex.EncodeToString(sum[:]); result.OutputSHA256 != want {
		t.Errorf("reported %s, file is %s", result.OutputSHA256, want)
	}
}
