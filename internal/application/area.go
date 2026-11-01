package application

// The monument switches, as the session sees them.
//
// Two switches over one table (plan.Area, plan.Rest): Wider summoning area -
// summoning away from a Rebirth Monument, with the sleeping monuments woken -
// and No once-per-rest limit. Each is staged like the strength switch, and
// one manifest part of their own writes them, saying where both stand:
// authoring writes the host's own block and has nothing to say to the
// monument table.

import (
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/savecore"
)

// MonumentSwitches is where the two switches stand.
type MonumentSwitches struct {
	WideArea, NoRestLimit bool
}

// StagedArea is the switches, staged: a field is nil unless that switch was
// moved.
type StagedArea struct {
	WideArea, NoRestLimit *bool
}

// Monuments is where the switches will stand once the staged changes are
// written: as moved, or as the save holds them.
func (session *Session) Monuments() MonumentSwitches {
	if session.loaded == nil {
		return MonumentSwitches{}
	}
	switches := session.monumentBaseline()
	if session.wideArea != nil {
		switches.WideArea = *session.wideArea
	}
	if session.noRestLimit != nil {
		switches.NoRestLimit = *session.noRestLimit
	}
	return switches
}

// monumentBaseline is where the switches stand unless moved: as the loaded
// save holds them, or both off beside a staged reset, which puts them back.
// A half applied to some rows only, or the area without its woken monuments,
// is a state this editor does not write, and reads as off.
func (session *Session) monumentBaseline() MonumentSwitches {
	state := session.loaded.state
	if session.restoration != nil || state.StoneRows == 0 {
		return MonumentSwitches{}
	}
	return MonumentSwitches{
		WideArea: state.WideAreaRows == state.StoneRows &&
			len(state.WokenMonuments) == len(savecore.SleepingMonuments()),
		NoRestLimit: state.NoRestLimitRows == state.StoneRows,
	}
}

// SetWideArea moves Wider summoning area.
func (session *Session) SetWideArea(on bool) error {
	if session.loaded == nil {
		return ErrNoSaveLoaded
	}
	return session.moveSwitch(&session.wideArea, on, session.monumentBaseline().WideArea)
}

// SetNoRestLimit moves No once-per-rest limit.
func (session *Session) SetNoRestLimit(on bool) error {
	if session.loaded == nil {
		return ErrNoSaveLoaded
	}
	return session.moveSwitch(&session.noRestLimit, on, session.monumentBaseline().NoRestLimit)
}

// moveSwitch stages a switch, or drops it when moved back to where the save
// has it. It needs no restaging of anything else: it writes tables no
// formation touches. What it does need, in the off position, is the shipped
// snapshot to put back, and it is refused now, while the caller is still here
// to be told, rather than at commit time.
func (session *Session) moveSwitch(staged **bool, on, baseline bool) error {
	if !on {
		if _, err := shippedVanilla(); err != nil {
			return err
		}
	}
	if on == baseline {
		*staged = nil
		return nil
	}
	*staged = &on
	return nil
}

// stagedArea is the switches as Staged reports them, nil when neither moved.
func (session *Session) stagedArea() *StagedArea {
	if session.wideArea == nil && session.noRestLimit == nil {
		return nil
	}
	return &StagedArea{WideArea: session.wideArea, NoRestLimit: session.noRestLimit}
}

// conversionRecipe is what a conversion writes when the switches were not
// moved: both on, as every conversion always did, the sleeping monuments
// woken with them, so a converted save reads as the switches on rather than as
// a state neither position describes.
func conversionRecipe(state *savecore.State) plan.Recipe {
	return plan.Recipe{
		MonumentOriginRows: plan.MonumentOriginRows1_17,
		Area:               plan.Anywhere,
		Sleeping:           savecore.SleepingMonuments(),
		Woken:              state.WokenMonuments,
	}
}

// areaRecipe is the recipe the switches ask for, or nil when neither was moved
// and there is nothing to write. It says where both stand, so the part writes
// the same rows whichever moved.
func (session *Session) areaRecipe() (*plan.Recipe, error) {
	if session.stagedArea() == nil {
		return nil, nil
	}
	switches := session.Monuments()
	recipe := plan.Recipe{
		MonumentOriginRows: plan.MonumentOriginRows1_17,
		Area:               plan.Monuments,
		Rest:               plan.RestKept,
		Sleeping:           savecore.SleepingMonuments(),
		Woken:              session.loaded.state.WokenMonuments,
	}
	if switches.WideArea {
		recipe.Area = plan.Anywhere
	}
	if switches.NoRestLimit {
		recipe.Rest = plan.RestLifted
	}
	if !switches.WideArea || !switches.NoRestLimit {
		shipped, err := shippedVanilla()
		if err != nil {
			return nil, err
		}
		recipe.Shipped = shipped
		if !switches.WideArea {
			recipe.MonumentOriginRows = session.shippedOrigins(shipped)
		}
	}
	return &recipe, nil
}

// shippedOrigins are the monument-origin rows whose formation the save holds
// as the game ships it. The off position puts the monument flag back on those
// only: a formation authored into one of these blocks (Putrid Corpse, Land
// Squirt, Spider Scorpion) keeps the 0 authoring wrote, so its summons still
// stand where they were placed rather than around a monument.
func (session *Session) shippedOrigins(shipped plan.Vanilla) []int32 {
	authored := map[int32]bool{}
	for trigger, block := range session.loaded.formations.Blocks {
		changed := false
		for _, row := range block.Rows {
			values, ships := shipped.Buddy[row.RowID]
			if !ships {
				changed = true
				break
			}
			now := row.Values(trigger)
			for _, field := range placedFields {
				if now[field] != values[field] {
					changed = true
				}
			}
		}
		if changed {
			for _, row := range block.Rows {
				authored[row.RowID] = true
			}
		}
	}
	var rows []int32
	for _, row := range plan.MonumentOriginRows1_17 {
		if !authored[row] {
			rows = append(rows, row)
		}
	}
	return rows
}

// placedFields are what authoring decides about a formation row: who stands
// there and where. The ladder is left out, because the strength switch writes
// it on shipped formations too.
var placedFields = []string{
	"npcParamId", "npcThinkParamId", "npcPlayerInitParamId", "generateAnimId",
	"x_offset", "z_offset", "y_angle",
}
