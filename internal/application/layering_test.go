package application

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Frontends are intentionally replaceable, so each is held to the set of
// project packages it may name.
//
// The point of the rule is not the size of the set. It is that no frontend
// names the domain stack - the catalogs, the plans, the save format - because
// a frontend that can name those is broken by changes nothing asked it to care
// about, and the next frontend has to be built against the same details all
// over again.
//
// The GUI's set is larger than the CLI's, and that is not a concession. The
// platform packages are a window's file dialogs, its config path and the
// system's double-click time, and curation is which picture and which name a
// card is drawn with. Neither is
// the domain: an editor with no pictures still edits saves. What the GUI may
// not do, and no longer does, is name a Price or a compatibility verdict from
// the layer that defines them - this package re-exports both, so the decision
// about whether they stay that shape is made here and in one place.
func TestFrontendsStayBehindTheApplicationBoundary(t *testing.T) {
	t.Parallel()
	const prefix = "github.com/DrippingSoup22/ER_summoning_editor/"
	frontends := []struct {
		name      string
		directory string
		// allowed is on top of the application package itself, which every
		// frontend may always name.
		allowed []string
	}{
		{"the command line", filepath.Join("..", "..", "cmd", "ersummoneditor"), nil},
		{"the GUI binary", filepath.Join("..", "..", "cmd", "ersummoneditorgui"), []string{
			"internal/platform/dialogs",
			"internal/ui/gio",
		}},
		{"the GUI", filepath.Join("..", "ui", "gio"), []string{
			"curation",
			"internal/platform/clicks",
			"internal/platform/dialogs",
			"internal/platform/paths",
		}},
	}
	for _, frontend := range frontends {
		t.Run(frontend.name, func(t *testing.T) {
			allowed := append([]string{"internal/application"}, frontend.allowed...)
			packages, err := parser.ParseDir(token.NewFileSet(), frontend.directory, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, parsedPackage := range packages {
				for filename, file := range parsedPackage.Files {
					// Tests may reach anywhere. A frontend's own tests build
					// saves and read them back, which needs the fixtures and
					// the catalogs the shipped code is kept away from.
					if strings.HasSuffix(filename, "_test.go") {
						continue
					}
					for _, imported := range file.Imports {
						path, err := strconv.Unquote(imported.Path.Value)
						if err != nil {
							t.Fatal(err)
						}
						if !strings.HasPrefix(path, prefix) &&
							!strings.HasPrefix(path, "github.com/DrippingSoup22/ER_research/") {
							continue
						}
						if slices.Contains(allowed, strings.TrimPrefix(path, prefix)) {
							continue
						}
						t.Errorf("%s bypasses the application boundary with import %q\nallowed: %v",
							filename, path, allowed)
					}
				}
			}
		})
	}
}
