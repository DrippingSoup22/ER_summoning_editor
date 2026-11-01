package gio

// What the player can adjust, and where it is kept.
//
// A theme, a font, the window and three sizes, all about being able to read
// the window. Anything added later has to earn its line by being something a
// player genuinely needs to change, not by being something that was easy to
// make configurable.
//
// The view holds no explanatory lines. A row whose name cannot say what its
// control does carries a "?" instead (help.go).
//
// Settings are written the moment they change, because there is no Save button
// on this view and there should not be one: a preference that needs confirming
// is a preference the player has to think about twice.
//
// Reset to vanilla is the exception to every word above: it is not a preference
// and it is not stored. It is here because it is a thing done once and rarely,
// like everything else on this view, and because the footer is the wrong place
// for it - the footer is on every screen, and a whole-save revert does not
// belong one click away from Save on the screen where a player is editing.
// What it does is unchanged: it stages, it appears in the pending list, and it
// can be dropped. See reset.go.

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/application"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/platform/paths"
)

// Settings is the persisted configuration. An absent or unreadable file leaves
// every field at its default rather than failing: a broken config must not
// stop the editor from opening.
type Settings struct {
	// Theme is the palette: "spirit" (default), "light", "elden", "parchment"
	// or "leather" (theme.go).
	Theme string `json:"theme"`
	// Font is the UI typeface, "lora" (default) or "cinzel". It applies to
	// everything at once, exactly as the sibling merchant editor's does.
	Font string `json:"font"`
	// IconSize is the side, in dp, of a cell in the table of Ashes a character
	// can be given. 0 means unset.
	//
	// It is a slider rather than a set of choices, unlike everything else on
	// the Settings view: a value like this has as many settings worth having as
	// the eye wants, where a short list of named choices would just be someone
	// else's taste with the alternatives removed.
	IconSize float32 `json:"icon_size"`
	// EditorIconSize is the same for the Editor's Ash rail, and it is a second
	// number rather than the same one because the two grids are not the same
	// shape. The Characters table is a row - icon, name, button - in a panel
	// with the window's whole width. The rail is icons only, in a column down
	// the left of the editing screen, and how large they want to be there is a
	// different question with a different answer.
	EditorIconSize float32 `json:"editor_icon_size"`
	// SpiritIconSize is the entity portrait in the spirits catalog, and it has
	// its own range rather than sharing the other two. An Ash icon is browsed
	// in a grid and wants to be big; an entity portrait sits beside a name in
	// a two-column list, so the useful sizes start well below where an Ash's
	// do and stop well before one card fills the panel.
	SpiritIconSize float32 `json:"spirit_icon_size"`
	// CatalogWidth is the Editor's catalog half, in dp, as the player last
	// dragged the divider to. 0 means unset: two cards side by side
	// (defaultCatalogWidth). It is not clamped here - what fits depends on the
	// window, so the layout clamps it.
	CatalogWidth float32 `json:"catalog_width"`
	// ShowSight draws on the grid what each summon notices by itself, and
	// HideKnobs takes the turning knobs off it. Both are switched among the
	// open Ash's options and kept for every Ash and every run.
	ShowSight bool `json:"show_sight"`
	HideKnobs bool `json:"hide_knobs"`
	// FullScreen fills the screen with the editor. A window is the default on
	// every system, so a settings file without the field opens a window (the
	// old "windowed" key is no longer read).
	FullScreen bool `json:"full_screen"`
}

// The display scale was here and is gone. It multiplied the whole window -
// every glyph and every icon resampled - to solve a window that opened larger
// than the screen, which opening maximised solves properly. What a player
// wanted from it was bigger pictures, and that is what the two sizes above are.

func defaultSettings() Settings {
	return Settings{
		Theme:          "spirit",
		Font:           "lora",
		IconSize:       float32(defaultIconSize),
		EditorIconSize: float32(defaultIconSize),
		SpiritIconSize: float32(spiritSizes.fallback),
	}
}

// The icon grids' range. The floor is where an Ash's artwork stops being
// recognisable and becomes a smudge; the ceiling is where one column fills a
// panel. The step keeps the slider on whole even numbers, so a value that comes
// back from the config file is one a player could have chosen deliberately.
// sizeRange is one slider's floor, ceiling and starting value. Two ranges
// exist because the two kinds of picture are looked at differently, and one
// range covering both would be too coarse at the small end for a portrait and
// too short at the large end for a grid.
type sizeRange struct{ floor, ceiling, fallback unit.Dp }

var (
	// ashSizes is both Ash grids: the Characters table and the Editor rail.
	ashSizes = sizeRange{minIconSize, maxIconSize, defaultIconSize}
	// spiritSizes is the entity portrait in the spirits catalog. The floor is
	// where a crayon drawing stops being a picture and becomes a smudge; the
	// ceiling is where a two-column list of names turns into a gallery. The
	// fallback is what the panel was built with before it was adjustable.
	spiritSizes = sizeRange{unit.Dp(24), unit.Dp(96), unit.Dp(34)}
)

const (
	minIconSize = unit.Dp(40)
	maxIconSize = unit.Dp(160)
	// 64dp, chosen by looking at the thing rather than by reasoning about it:
	// it is where an Ash's artwork is readable and a grid still holds enough of
	// them to browse. It is also the size both grids were built around before
	// either was adjustable.
	defaultIconSize = unit.Dp(64)
	iconSizeStep    = 2
)

// iconSizeFromSlider maps the slider's 0..1 to a size on the step.
func iconSizeFromSlider(at float32, over sizeRange) float32 {
	size := float32(over.floor) + at*float32(over.ceiling-over.floor)
	return float32(int(size/iconSizeStep+0.5) * iconSizeStep)
}

// sliderFromIconSize is the inverse, for seeding the control from the config.
func sliderFromIconSize(size float32, over sizeRange) float32 {
	at := (size - float32(over.floor)) / float32(over.ceiling-over.floor)
	return min(max(at, 0), 1)
}

// IconSize is the cell of the Characters table, as that panel asks for it.
func (s *State) IconSize() unit.Dp { return unit.Dp(s.settings.IconSize) }

// EditorIconSize is the cell of the Editor's Ash rail.
func (s *State) EditorIconSize() unit.Dp { return unit.Dp(s.settings.EditorIconSize) }

// SpiritIconSize is the entity portrait in the spirits catalog.
func (s *State) SpiritIconSize() unit.Dp { return unit.Dp(s.settings.SpiritIconSize) }

// normalize repairs a config written by an older build, edited by hand, or
// truncated by a crash. An unknown value is replaced rather than reported: the
// player did not ask to be told their file was odd, they asked for an editor.
func (s Settings) normalize() Settings {
	if _, known := palettes[s.Theme]; !known {
		s.Theme = "spirit"
	}
	if typefaceFor(s.Font) == displayFace {
		s.Font = "cinzel"
	} else {
		s.Font = "lora"
	}
	s.IconSize = repairIconSize(s.IconSize, ashSizes)
	s.EditorIconSize = repairIconSize(s.EditorIconSize, ashSizes)
	s.SpiritIconSize = repairIconSize(s.SpiritIconSize, spiritSizes)
	if !(s.CatalogWidth > 0) { // NaN as well
		s.CatalogWidth = 0
	}
	return s
}

// repairIconSize brings a stored size back into the range the slider offers.
//
// Unset first, then clamp - in that order. Clamping first turns 0 into the
// smallest icon on offer, and 0 means "never chosen", not "as small as
// possible": an empty config would have opened the editor at 40dp rather than
// at the default.
func repairIconSize(size float32, over sizeRange) float32 {
	if size <= 0 {
		return float32(over.fallback)
	}
	return float32(int(min(max(size, float32(over.floor)), float32(over.ceiling))))
}

// loadSettings reads the stored configuration, falling back to defaults.
func loadSettings() Settings {
	path, err := paths.ConfigFile()
	if err != nil {
		return defaultSettings()
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return defaultSettings()
	}
	settings := defaultSettings()
	if err := json.Unmarshal(body, &settings); err != nil {
		return defaultSettings()
	}
	return settings.normalize()
}

// save writes the configuration through a temporary file, so an interrupted
// write leaves the previous settings rather than half of the new ones.
func (s Settings) save() error {
	path, err := paths.ConfigFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// applySettings puts the current settings into effect and stores them.
//
// The typeface is swapped on the theme in place. Building a fresh
// material.Theme would discard the shaper and its caches, which re-shapes
// every label the editor has ever drawn - the sibling measured that as a
// visible pause on a font switch.
func (s *State) applySettings() {
	s.settings = s.settings.normalize()
	s.th.Face = typefaceFor(s.settings.Font)
	s.applyPalette()
	if err := s.settings.save(); err != nil {
		s.setNotice(notice{text: "Could not store your settings: " + err.Error(), bad: true})
	}
}

// layoutSettings is the Settings view: two columns of cards in the middle of
// the panel - how the window looks on the left; on the right, what every Spirit
// Ash of the open save may do, and the reset of the whole save. Two columns
// rather than one, so the view fits the window without scrolling; it scrolls only in a window too short even for that.
//
// It carries no SETTINGS heading. No other view names itself either - the tab
// that is lit is what says where you are. The cards' titles name a group
// inside the view, which is a different job.
func (s *State) layoutSettings(gtx layout.Context, th *material.Theme) layout.Dimensions {
	appearance := []layout.Widget{
		s.settingRow(th, "Theme", noHelp, s.themeControl(th)),
		s.settingRow(th, "Font", noHelp, s.fontControl(th)),
		s.settingRow(th, "Full screen", helpFullScreen, s.fullScreenControl(th)),
		subheading(th, "ICON SIZES"),
		s.settingRow(th, "Characters", noHelp,
			s.sizeControl(th, ashSizes, &s.iconSlider, &s.settings.IconSize, &s.iconUnsaved)),
		s.settingRow(th, "Editor", noHelp,
			s.sizeControl(th, ashSizes, &s.editorIconSlider, &s.settings.EditorIconSize, &s.editorIconUnsaved)),
		s.settingRow(th, "Spirit portraits", noHelp,
			s.sizeControl(th, spiritSizes, &s.spiritIconSlider, &s.settings.SpiritIconSize, &s.spiritIconUnsaved)),
	}
	ashes := []layout.Widget{
		s.settingRow(th, "Wider summoning area", helpWideArea, s.monumentControl(th, &s.wideAreaBox, "Wider summoning area",
			func(on application.MonumentSwitches) bool { return on.WideArea }, s.session.SetWideArea)),
		s.settingRow(th, "No once-per-rest limit", helpNoRestLimit, s.monumentControl(th, &s.noRestLimitBox, "No once-per-rest limit",
			func(on application.MonumentSwitches) bool { return on.NoRestLimit }, s.session.SetNoRestLimit)),
		s.settingRow(th, "All free", helpAllFree, s.allFreeControl(th)),
	}
	reset := s.settingRow(th, "Reset to vanilla", helpReset, s.resetControl(th))

	return panelSurface(gtx, func(gtx layout.Context) layout.Dimensions {
		// The scrollbar floats over the panel's edge rather than taking a
		// strip of it, which would pull the columns off the centre.
		list := material.List(th, &s.settingsList)
		list.AnchorStrategy = material.Overlay
		return list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			// Centred by hand: a list hands its row the panel's width as a
			// maximum and nothing as a minimum, so a Direction has nothing to
			// centre in.
			width, gap := gtx.Constraints.Max.X, gtx.Dp(settingsGap)
			column := max(min((width-gap)/2, gtx.Dp(settingsColumn)), 1)
			defer op.Offset(image.Pt(max(width-2*column-gap, 0)/2, 0)).Push(gtx.Ops).Pop()
			in := func(w layout.Widget) layout.FlexChild {
				return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = column, column
					return w(gtx)
				})
			}
			return layout.Inset{Top: unit.Dp(16), Bottom: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					in(func(gtx layout.Context) layout.Dimensions {
						return settingsCard(gtx, th, "APPEARANCE", appearance...)
					}),
					layout.Rigid(layout.Spacer{Width: settingsGap}.Layout),
					in(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return settingsCard(gtx, th, "SPIRIT ASHES", ashes...)
							}),
							layout.Rigid(layout.Spacer{Height: settingsGap}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return settingsCard(gtx, th, "THIS SAVE", reset)
							}),
						)
					}),
				)
			})
		})
	})
}

// settingsColumn is one column: wide enough for a name and the widest control,
// and no wider, so the eye does not travel to find the control a name belongs
// to. settingsGap is between the columns.
const (
	settingsColumn = unit.Dp(500)
	settingsGap    = unit.Dp(20)
)

// settingsCard is one group: its title over a rule, then its rows, on a ground
// a step off the panel, inside the same border and corners as a dialog.
func settingsCard(gtx layout.Context, th *material.Theme, name string, rows ...layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 14, Bottom: 18, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		children := []layout.FlexChild{
			layout.Rigid(heading(th, name).Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(horizontalDivider),
		}
		for _, row := range rows {
			children = append(children, layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout), layout.Rigid(row))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := macro.Stop()

	corner := unit.Dp(4)
	paint.FillShape(gtx.Ops, colorCard, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(corner)).Op(gtx.Ops))
	widget.Border{Color: colorDivider, Width: unit.Dp(1), CornerRadius: corner}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions { return dims })
	call.Add(gtx.Ops)
	return dims
}

// subheading names a few rows inside a card: smaller and fainter than the
// card's title, with a little room above it.
func subheading(th *material.Theme, name string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			style := heading(th, name)
			style.TextSize = th.TextSize * 12 / 16
			style.Color = colorMuted
			return style.Layout(gtx)
		})
	}
}

// settingRow is one setting: its name, its "?" where the name cannot say what
// the control does, and the control at the right edge.
func (s *State) settingRow(th *material.Theme, name string, topic helpTopic, control layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		row := func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(material.Body1(th, name).Layout)}
			if topic != noHelp {
				children = append(children,
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.helpBadge(gtx, th, topic) }))
			}
			children = append(children,
				layout.Flexed(1, flexSpacer),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(control))
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		}
		if topic == noHelp {
			return row(gtx)
		}
		return s.withHelp(gtx, th, topic, row)
	}
}

// themeControl is the palette, applied to the whole window at once.
func (s *State) themeControl(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		current := max(slices.Index(themeKeys, s.settings.Theme), 0)
		dims, chose := s.themeCombo.Layout(gtx, th, fontComboWidth, themeNames, current)
		if chose >= 0 {
			s.settings.Theme = themeKeys[chose]
			s.applySettings()
		}
		return dims
	}
}

// fontControl is the typeface, applied to the whole window at once.
func (s *State) fontControl(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		dims, chose := s.fontCombo.Layout(gtx, th, fontComboWidth, fontNames, fontChoice(s.settings.Font))
		if chose >= 0 {
			s.settings.Font = fontKeys[chose]
			s.applySettings()
		}
		return dims
	}
}

// fontComboWidth is the merchant editor's, to the dp: the same control in the
// same application should not be a different size.
const fontComboWidth = unit.Dp(180)

// sizeControl is one size: the slider and the number it is on.
//
// There was a picture here - a cell drawn at the chosen size beside every
// track - and it is gone. It showed the size and not the effect: what a player
// is choosing is how a grid of these reads, and one icon on its own says
// nothing about that. The grid itself is one click away and follows the drag
// live, so the real preview was always the other tab.
//
// Settings are stored the moment they change everywhere else on this view. Not
// here: a drag is hundreds of changes, and writing the file on each would be
// hundreds of writes for one decision. The value applies live and is stored
// when the thumb is let go.
func (s *State) sizeControl(th *material.Theme, over sizeRange, slider *widget.Float, size *float32,
	unsaved *bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if slider.Update(gtx) {
			*size = iconSizeFromSlider(slider.Value, over)
			*unsaved = true
		}
		if *unsaved && !slider.Dragging() {
			*unsaved = false
			s.applySettings()
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(sizeSliderWidth), gtx.Dp(sizeSliderWidth)
				style := material.Slider(th, slider)
				style.Color = colorAccent
				return style.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(36))
				number := muted(th, fmt.Sprint(int(*size)))
				number.Alignment = text.End
				return number.Layout(gtx)
			}),
		)
	}
}

// sizeSliderWidth is the track. Long enough that the step between two
// neighbouring sizes is a real distance to drag rather than a pixel.
const sizeSliderWidth = unit.Dp(200)

// fullScreenControl is Full screen: on, the window fills the screen; off, it
// is a window of the default size. Applied at once and kept, so the next start
// opens the same way.
func (s *State) fullScreenControl(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if s.fullScreen.Update(gtx) {
			s.settings.FullScreen = s.fullScreen.Value
			s.applySettings()
			s.applyWindowMode()
		}
		swtch := material.Switch(th, &s.fullScreen, "Full screen")
		swtch.Color.Track, swtch.Color.Disabled, swtch.Color.Enabled = colorControl, colorMuted, colorAccent
		return swtch.Layout(gtx)
	}
}

// allFreeControl is All free: a switch that is on exactly when every Ash of
// the open save is free, and that frees them all, or puts them all back on
// their own price when all are free (setAllFree).
func (s *State) allFreeControl(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch {
		case s.LoadedSave() == nil:
			return disabledButton(gtx, th, "Open a save first")
		case s.BusyMsg() != "":
			return disabledButton(gtx, th, "All free")
		}
		if s.allFreeBox.Update(gtx) {
			s.setAllFree(s.allFreeBox.Value)
		}
		s.allFreeBox.Value = s.allFree()
		swtch := material.Switch(th, &s.allFreeBox, "All free")
		swtch.Color.Track, swtch.Color.Disabled, swtch.Color.Enabled = colorControl, colorMuted, colorAccent
		return swtch.Layout(gtx)
	}
}

// monumentControl is one of the two monument switches: on as the save will
// stand once saved, and moving it stages the change like any other, listed
// under Pending (application.Monuments).
func (s *State) monumentControl(th *material.Theme, box *widget.Bool, name string,
	position func(application.MonumentSwitches) bool, move func(bool) error) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch {
		case s.LoadedSave() == nil:
			return disabledButton(gtx, th, "Open a save first")
		case s.BusyMsg() != "":
			return disabledButton(gtx, th, name)
		}
		if box.Update(gtx) {
			if err := move(box.Value); err != nil {
				s.setNotice(notice{text: err.Error(), bad: true})
			}
		}
		box.Value = position(s.session.Monuments())
		swtch := material.Switch(th, box, name)
		swtch.Color.Track, swtch.Color.Disabled, swtch.Color.Enabled = colorControl, colorMuted, colorAccent
		return swtch.Layout(gtx)
	}
}

// resetControl is Reset to vanilla's button, which stages rather than acts,
// and turns around into the control that drops what it staged.
//
// Staging one re-reads the save, so it takes a worker and the button is inert
// while one is running. Dropping it is a memory operation and happens in the
// frame, which is why only one of the two branches goes through StartReset.
func (s *State) resetControl(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		switch {
		case s.LoadedSave() == nil:
			return disabledButton(gtx, th, "Open a save first")
		case s.resetStaged():
			if s.resetBtn.Clicked(gtx) {
				s.dropReset()
			}
			return button(th, &s.resetBtn, "Drop the reset").Layout(gtx)
		case s.BusyMsg() != "":
			return disabledButton(gtx, th, "Reset")
		}
		if s.resetBtn.Clicked(gtx) {
			s.StartReset()
		}
		return quietButton(th, &s.resetBtn, "Reset").Layout(gtx)
	}
}
