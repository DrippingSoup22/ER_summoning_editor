// Package gio is the desktop editor's view layer.
//
// It owns window layout, retained widget state, the drag gesture and the
// worker lifecycle that keeps loading and saving off the frame goroutine. It
// owns nothing else. Everything the player has actually changed lives in
// internal/application, so this package renders read models and calls the
// session; it never decides what an edit means. See docs/GUI.md.
package gio

// The palettes.
//
// Five themes. The sibling merchant editor's three: Spirit, the cold faint
// palette built on the colour of the game's own summon flame and the default;
// Light, warm paper; and Elden Ring, the game's own menus - near-black, bronze
// edges, parchment text and a muted gold. The author asked for the last two
// (2026-10-27). And two of an old book, drawn from the catalog's pictures -
// crayon on golden parchment, umber and slate-blue ink: Parchment, the page itself; and Leather, its binding, with the
// page's colour for text.
//
// The colours below are read during layout on the frame goroutine, and
// usePalette writes them on the same goroutine, from Settings, so there is
// nothing to guard. Each palette sets every one of them: a colour one theme
// forgot would be the previous theme's, and only after a switch.

import "image/color"

var (
	colorBg      color.NRGBA // window ground
	colorPanelBg color.NRGBA // panels, barely lifted off it
	colorFg      color.NRGBA
	colorMuted   color.NRGBA // secondary text: counts, families, hints
	colorDivider color.NRGBA
	// colorInputBg is recessed: it is below the panel, which is what a text
	// field and a control that is currently unavailable should both look like.
	colorInputBg color.NRGBA
	// colorControl is the opposite, and the two together are the whole
	// language: a button a player can press sits ABOVE the panel it is on.
	colorControl color.NRGBA
	// colorCard is a group of settings: a step off the panel, short of a
	// control, so a card reads as a place that holds controls rather than as
	// one.
	colorCard color.NRGBA

	// colorAccent is INK: the edge of a filled slot, the bar down a selected
	// row, the ring under an active drag. It is never a ground behind text;
	// colorAccentFill is, and is chosen so colorFg reads on it.
	colorAccent     color.NRGBA
	colorAccentFill color.NRGBA
	// colorGlow rings a filled slot at low alpha, and is the ink of a name
	// that is lit: staged, or the one an explanation is about.
	colorGlow color.NRGBA

	colorWarn  color.NRGBA // large actors, warn verdicts
	colorError color.NRGBA // refusals
	// colorContrastFg is text on colorGlow: the drag ghost, the one place a
	// glow is a ground.
	colorContrastFg color.NRGBA

	// modalScrim dims the window behind a blocking dialog, so what is behind
	// stays as context without competing with the thing being asked about.
	modalScrim color.NRGBA

	// colorSlotEdge and colorSlotFill draw a slot: the formation's, a chosen
	// row's, an opened card's.
	colorSlotEdge color.NRGBA
	colorSlotFill color.NRGBA

	// The placement grid. A cell is a tile inside a calm frame, apart from the
	// next by a gap, so every cell reads on its own. What the grid has to say
	// about a cell changes the inside or the frame and never both, so the two
	// can be read at once. The inside says what stands there: a body
	// (colorBodyFill), or two bodies at once - the size warning, in yellow. The
	// frame says who sees it: an ally in blue, an enemy in red, the chosen
	// summon's stronger still.
	colorCellFill    color.NRGBA
	colorCellEdge    color.NRGBA
	colorCellGap     color.NRGBA
	colorBodyFill    color.NRGBA
	colorCrowdFill   color.NRGBA
	colorAllyChosen  color.NRGBA
	colorEnemySight  color.NRGBA
	colorEnemyChosen color.NRGBA
)

// palette is one theme: a value for every colour above.
type palette struct {
	bg, panel, fg, muted, divider, inputBg, control, card color.NRGBA
	accent, accentFill, glow, warn, error, contrastFg     color.NRGBA
	scrim, slotEdge, slotFill                             color.NRGBA
	cellFill, cellEdge, cellGap, bodyFill, crowdFill      color.NRGBA
	allyChosen, enemySight, enemyChosen                   color.NRGBA
}

// The themes as Settings offers them: the stored keys, and the names shown.
var (
	themeKeys  = []string{"spirit", "light", "elden", "parchment", "leather"}
	themeNames = []string{"Spirit", "Light", "Elden Ring", "Parchment", "Leather"}
)

var palettes = map[string]palette{
	// Spirit: black with blue in it, ash text, and the pale blue-white of a
	// spirit summon. The accent fill is deep because the body face's stems are
	// about a pixel wide at button size: on the pale accent with dark text
	// they antialiased into grey and every label read as disabled. Gio blends
	// in linear light, so the scrim is four fifths black to obscure.
	"spirit": {
		bg: rgb(0x0B0E12), panel: rgb(0x121821), fg: rgb(0xD6E2E8), muted: rgb(0x6E7F8C),
		divider: rgb(0x202A35), inputBg: rgb(0x0E141B), control: rgb(0x1E2836), card: rgb(0x161E28),
		accent: rgb(0x7FD4E0), accentFill: rgb(0x1D5E6E), glow: rgb(0xA8ECF2),
		warn: rgb(0xC9945A), error: rgb(0xC0473E), contrastFg: rgb(0x08111A),
		scrim: color.NRGBA{A: 0xD0}, slotEdge: rgb(0x3E6A78), slotFill: rgb(0x16202B),
		cellFill: rgb(0x0C1218), cellEdge: rgb(0x263B52), cellGap: rgb(0x05080B),
		bodyFill: rgb(0x1F2B37), crowdFill: rgb(0xC9A431),
		allyChosen: rgb(0x8DC4FF),
		enemySight: rgb(0xB9463C), enemyChosen: rgb(0xFF7B6B),
	},
	// Light: the merchant editor's warm paper, with a slate-blue accent. The
	// accent fill is pale, so the dark text reads on it; a control is a step
	// darker than the panel, since nothing is lighter than paper.
	"light": {
		bg: rgb(0xE9E5DC), panel: rgb(0xF8F6F1), fg: rgb(0x24211D), muted: rgb(0x6B6459),
		divider: rgb(0xD3CBBB), inputBg: rgb(0xEFEBE3), control: rgb(0xE2DCD0), card: rgb(0xF2EEE7),
		accent: rgb(0x2F5D8C), accentFill: rgb(0xC3D5E8), glow: rgb(0x1F6F9A),
		warn: rgb(0x9A5E0A), error: rgb(0xA82A22), contrastFg: rgb(0xFFFFFF),
		scrim: color.NRGBA{R: 0x24, G: 0x21, B: 0x1D, A: 0x80}, slotEdge: rgb(0x9DB3C8), slotFill: rgb(0xE1E8EF),
		cellFill: rgb(0xF4F1EA), cellEdge: rgb(0xD9E0E7), cellGap: rgb(0xE8E3DA),
		bodyFill: rgb(0xD6DEE7), crowdFill: rgb(0xE2BE4A),
		allyChosen: rgb(0x1A4FA0),
		enemySight: rgb(0xD0655B), enemyChosen: rgb(0xB0281C),
	},
	// Elden Ring: matched to the game's inventory screen as the merchant
	// editor's is - a near-black warm ground, panels barely lighter, a dim
	// bronze edge, parchment text and the menu's muted gold. The warning
	// leans to ember so it is not the accent.
	"elden": {
		bg: rgb(0x0E0C0A), panel: rgb(0x16130F), fg: rgb(0xD8CDB4), muted: rgb(0x8C7E63),
		divider: rgb(0x3A3122), inputBg: rgb(0x110E0B), control: rgb(0x2A231A), card: rgb(0x1B1712),
		accent: rgb(0xB29353), accentFill: rgb(0x55411F), glow: rgb(0xE2C682),
		warn: rgb(0xD9803A), error: rgb(0xC03A2E), contrastFg: rgb(0x120F0B),
		scrim: color.NRGBA{A: 0xD0}, slotEdge: rgb(0x6B5634), slotFill: rgb(0x221C14),
		cellFill: rgb(0x120F0B), cellEdge: rgb(0x3D3222), cellGap: rgb(0x080706),
		bodyFill: rgb(0x2A231A), crowdFill: rgb(0xC9A431),
		allyChosen: rgb(0x8DC4FF),
		enemySight: rgb(0xB9463C), enemyChosen: rgb(0xFF7B6B),
	},
	// Parchment: the pictures' aged page, a mid tone so it is never bright,
	// with umber ink for text and the drawings' slate blue for the accent; red
	// ink stays the enemy's. As on Light, a control is a step darker than the
	// page, and the warning is an amber lighter and stronger than the tan
	// bodies it marks.
	"parchment": {
		bg: rgb(0xB39A6E), panel: rgb(0xC9B283), fg: rgb(0x2A1D12), muted: rgb(0x5E4A30),
		divider: rgb(0xA68B5E), inputBg: rgb(0xBFA677), control: rgb(0xB89D6C), card: rgb(0xCFB98C),
		accent: rgb(0x2E4A78), accentFill: rgb(0xA9B4B4), glow: rgb(0x1E3A6A),
		warn: rgb(0x7A3C0E), error: rgb(0x8A1C12), contrastFg: rgb(0xF2E6C8),
		scrim: color.NRGBA{R: 0x2A, G: 0x1D, B: 0x12, A: 0x90}, slotEdge: rgb(0x7E6646), slotFill: rgb(0xBEA577),
		cellFill: rgb(0xC6AE80), cellEdge: rgb(0x957C52), cellGap: rgb(0xA88F63),
		bodyFill: rgb(0xAA8F60), crowdFill: rgb(0xF2B53A),
		allyChosen: rgb(0x1C3C88),
		enemySight: rgb(0xA3362A), enemyChosen: rgb(0x76140C),
	},
	// Leather: the book's dark binding, panels a worn brown, the page's own
	// golden colour for text and gold leaf for the accent.
	"leather": {
		bg: rgb(0x1A120C), panel: rgb(0x241911), fg: rgb(0xE2CB98), muted: rgb(0x9C8560),
		divider: rgb(0x3E2E1F), inputBg: rgb(0x1D150E), control: rgb(0x36281B), card: rgb(0x2B1F15),
		accent: rgb(0xC9A24E), accentFill: rgb(0x5C4320), glow: rgb(0xEFD38C),
		warn: rgb(0xD9803A), error: rgb(0xCC4A3A), contrastFg: rgb(0x1A120B),
		scrim: color.NRGBA{A: 0xD0}, slotEdge: rgb(0x6F5535), slotFill: rgb(0x2A1E14),
		cellFill: rgb(0x1E160F), cellEdge: rgb(0x4C3A25), cellGap: rgb(0x0F0A06),
		bodyFill: rgb(0x3C2D1D), crowdFill: rgb(0xD6A13A),
		allyChosen: rgb(0x8FB6E6),
		enemySight: rgb(0xBF4E3C), enemyChosen: rgb(0xFF8A70),
	},
}

// usePalette makes the named theme the colours every layout reads. An unknown
// name is Spirit's.
func usePalette(theme string) {
	p, known := palettes[theme]
	if !known {
		p = palettes["spirit"]
	}
	colorBg, colorPanelBg, colorFg, colorMuted = p.bg, p.panel, p.fg, p.muted
	colorDivider, colorInputBg, colorControl, colorCard = p.divider, p.inputBg, p.control, p.card
	colorAccent, colorAccentFill, colorGlow = p.accent, p.accentFill, p.glow
	colorWarn, colorError, colorContrastFg = p.warn, p.error, p.contrastFg
	modalScrim, colorSlotEdge, colorSlotFill = p.scrim, p.slotEdge, p.slotFill
	colorCellFill, colorCellEdge, colorCellGap = p.cellFill, p.cellEdge, p.cellGap
	colorBodyFill, colorCrowdFill = p.bodyFill, p.crowdFill
	colorAllyChosen = p.allyChosen
	colorEnemySight, colorEnemyChosen = p.enemySight, p.enemyChosen
}

// Spirit until Settings says otherwise, so a layout drawn before a State
// exists has colours.
func init() { usePalette("spirit") }

// rgb expands a 0xRRGGBB literal into an opaque colour, so the tables above
// read as the palettes they are rather than as four fields per line.
func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xFF}
}

// alpha returns c at the given opacity, for the glow ring and the faded lower
// half of the slot panel.
func alpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
