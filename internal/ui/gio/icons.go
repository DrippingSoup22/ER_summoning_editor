package gio

// The Spirit Ash item icons.
//
// These are the icons the game draws in the player's inventory, so the rail
// shows the same picture a player would be looking at if they were choosing an
// Ash in game rather than in this editor. 84 of them, one per family, which is
// the whole set - an Ash without an icon is a case that does not exist today,
// and TestEveryAshHasAnIcon is what keeps that true.
//
// They live under internal/ui/gio rather than in a shared assets package
// because embed variables are not dead-code eliminated: a package the
// command-line binary imports would put nearly 3 MB of PNG into a binary that
// never draws anything. Only the desktop editor imports this one.
//
// The file for an Ash is derived from its name rather than named beside it in
// the catalog. That is a real coupling and it is deliberate - the alternative
// is an 84-row column in catalog-ashes.csv that repeats what the name already
// says - and the test above fails loudly if the derivation stops covering the
// set.

import (
	"embed"
	"image"
	_ "image/png"
	"regexp"
	"strings"
	"unicode"

	"gioui.org/op/paint"
	// Aliased: the package name collides with this package's own draw test
	// helper, and the helper is the one that reads better at its call sites.
	xdraw "golang.org/x/image/draw"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// unaccent decomposes a name and drops the combining marks, so "Jolán" folds
// to "jolan" rather than to "jol_n". Two Ashes need it - Swordhand of Night
// Jolán and Jolán and Anna - and a regexp that treats an accented letter as a
// separator would silently give both of them the wrong file.
var unaccent = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

//go:embed assets/ashes/*.png
var ashIcons embed.FS

// The entity portraits, one per catalog variant, named for the row it ships
// (pictures.go).
//
// They are packed down to 128px by the author's picture packer before they land here, and
// the reason is arithmetic: the masters are 1254px square and about 3 MB each,
// embed variables are not dead-code eliminated, and the finished roster of 282
// would put 849 MB of PNG inside the binary. At 128px the whole roster is a
// handful of megabytes and still four times the largest cell it is drawn in.
//
// The set is INCOMPLETE and stays incomplete for a long time - 270 of about
// 1,250 variants are drawn - so every lookup has to be allowed to miss. Nothing
// here invents a placeholder: a variant with no picture borrows one by the
// rules in pictures.go, or draws the empty cell.
//
//go:embed assets/entities/*.png
var entityIcons embed.FS

// The item icons the character builder shows, one per item the builder
// offers, named by its table and row (weapon-2000000). the author's item pack tool packs them
// to 80px from the merchant editor's vendored set; at that size the 1,803 of
// them are about 15 MB, which only this desktop binary carries.
//
//go:embed assets/items/*.png
var itemIcons embed.FS

// unslug is every run of characters that is not a letter or a digit, which is
// what separates the words of a file name.
var unslug = regexp.MustCompile(`[^a-z0-9]+`)

// dropped is the punctuation that vanishes rather than becoming a separator.
// Built once: a Replacer compiles a trie, which is most of what naming an Ash
// used to cost.
var dropped = strings.NewReplacer("'", "", "’", "", ".", "")

// iconName is the file an Ash's icon is stored under: its name folded to
// lowercase words joined by underscores.
//
// Accents are folded away first and apostrophes and full stops are dropped
// rather than turned into separators, so "Jolán and Anna" and "D, Beholder of
// Death" come out as the names the icon set already uses rather than with a
// stray underscore where a letter was.
func iconName(ash string) string {
	folded := strings.ToLower(ash)
	if plain, _, err := transform.String(unaccent, folded); err == nil {
		folded = plain
	}
	return unslug.ReplaceAllString(dropped.Replace(folded), "_") + ".png"
}

// icons decodes an icon once and keeps the paint operation for the size it is
// drawn at.
//
// Two caches rather than one. sources holds the decoded 256px master, which is
// the expensive part and does not depend on the window; ops holds one resampled
// operation per size actually drawn, because the pixel size of a 34dp cell
// depends on the scale factor the player chooses in Settings. Both are frame
// state, like everything else not guarded by the state's mutex.
type icons struct {
	// files and dir are where this cache reads from, so one implementation
	// serves both sets: the Ash item icons and the entity portraits are the
	// same problem - decode once, resample per size, remember the misses - and
	// were not worth writing twice.
	files embed.FS
	dir   string
	// file turns whatever the caller keys by into a file name. The Ashes key
	// by name and pay a fold and a regexp for it; the entities key by the row
	// a variant ships, and the file is that row.
	file func(string) string

	sources map[string]image.Image
	ops     map[drawn]paint.ImageOp
	missing map[string]bool
}

// ashArt and entityArt are the two caches, built once with what they read.
func ashArt() icons {
	return icons{files: ashIcons, dir: "assets/ashes/", file: iconName}
}

func entityArt() icons {
	return icons{files: entityIcons, dir: "assets/entities/", file: func(model string) string {
		return model + ".png"
	}}
}

func itemArt() icons {
	return icons{files: itemIcons, dir: "assets/items/", file: func(key string) string {
		return key + ".png"
	}}
}

// drawn keys the resampled operations by what the caller actually has - the
// Ash's name and the size it is being drawn at.
//
// Keyed on the name rather than on the file it resolves to, because the whole
// point is that a hit must not have to derive the file: iconName normalises,
// folds accents and runs a regexp, and paying that per cell per frame was 45%
// of a frame's allocations before this. A comparable struct key rather than a
// joined string, so a lookup allocates nothing at all.
type drawn struct {
	ash string
	px  int
}

// op is the icon for one Ash at one pixel size, and whether there is one.
func (cache *icons) op(ash string, px int) (paint.ImageOp, bool) {
	if ash == "" {
		return paint.ImageOp{}, false
	}
	key := drawn{ash: ash, px: px}
	if op, known := cache.ops[key]; known {
		return op, true
	}
	if cache.missing[ash] {
		return paint.ImageOp{}, false
	}
	source, ok := cache.source(cache.file(ash))
	if !ok {
		// Recorded under the Ash's name as well, so the next frame does not
		// derive the file again to discover the same absence.
		cache.miss(ash)
		return paint.ImageOp{}, false
	}
	if cache.ops == nil {
		cache.ops = make(map[drawn]paint.ImageOp, 84)
	}
	op := paint.NewImageOp(resize(source, px))
	cache.ops[key] = op
	return op, true
}

// source is the decoded master, decoded on first use.
func (cache *icons) source(name string) (image.Image, bool) {
	if decoded, known := cache.sources[name]; known {
		return decoded, true
	}
	file, err := cache.files.Open(cache.dir + name)
	if err != nil {
		cache.miss(name)
		return nil, false
	}
	defer file.Close()
	decoded, _, err := image.Decode(file)
	if err != nil {
		cache.miss(name)
		return nil, false
	}
	if cache.sources == nil {
		cache.sources = make(map[string]image.Image, 84)
	}
	cache.sources[name] = decoded
	return decoded, true
}

// miss records an icon that is not there, so a missing file costs one failed
// open rather than one per frame for as long as the save is loaded.
func (cache *icons) miss(name string) {
	if cache.missing == nil {
		cache.missing = make(map[string]bool)
	}
	cache.missing[name] = true
}

// resize resamples a master down to the size it is drawn at.
//
// CatmullRom rather than the GPU's own filtering: bilinear minification samples
// too sparsely to keep an icon's outline. Done once per size and then cached,
// so the cost is paid on the first frame that needs it and never again.
//
// The masters are stored at 128px rather than the 256px they were vendored at,
// which is twice the 64dp the rail draws them at and so still sharp at a 2x
// scale factor. That is not a storage decision: a kernel scaler's cost is in
// its source pixels, and resampling from 256px made filling the rail's cache
// take 280ms - a visible stall the first time the Editor was opened.
func resize(source image.Image, px int) image.Image {
	if bounds := source.Bounds(); bounds.Dx() == px && bounds.Dy() == px {
		return source
	}
	target := image.NewNRGBA(image.Rect(0, 0, px, px))
	xdraw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), xdraw.Src, nil)
	return target
}
