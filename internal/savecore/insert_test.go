// Row insertion is proved differently from patching. A patch is proved by
// comparing byte windows; an insertion moves the bytes it does not change, so
// these tests hold it to the stronger claim instead: every shipped row survives
// byte-identically compared by row ID, nothing appears that was not declared,
// and the result reads back through savefixture's independent reader rather
// than only through the code that wrote it.
package savecore

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/DrippingSoup22/ER_shared/regulation"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savefixture"
)

var (
	insertSaveOnce sync.Once
	insertSave     []byte
	insertSaveErr  error
)

func fixtureSave(t *testing.T) []byte {
	t.Helper()
	insertSaveOnce.Do(func() { insertSave, insertSaveErr = savefixture.Build(savefixture.Defaults()) })
	if insertSaveErr != nil {
		t.Fatal(insertSaveErr)
	}
	return insertSave
}

func loadFixture(t *testing.T, save []byte) *regulation.Regulation {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.dat")
	if err := os.WriteFile(path, save, 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := regulation.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return source
}
