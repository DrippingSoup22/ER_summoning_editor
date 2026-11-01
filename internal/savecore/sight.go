package savecore

import (
	"encoding/binary"

	"github.com/DrippingSoup22/ER_shared/regulation"
)

// Senses is what a think row notices by itself, in metres from the actor's
// centre: an eye cone Eye metres long and Cone degrees wide (eye_angY, a width
// capped at 180, so a cone never sees behind) whose apex sits Behind metres
// back (eye_BackOffsetDist), and a nose all round to Nose. Zero eye and nose is
// an actor that notices nothing until it is hit.
type Senses struct {
	Eye, Cone, Behind, Nose float64
}

// readSenses reads every NpcThinkParam row's own senses.
func readSenses(bnd4 []byte) (map[int32]Senses, error) {
	table, err := regulation.OpenTable(bnd4, npcThinkLayout)
	if err != nil {
		return nil, err
	}
	senses := make(map[int32]Senses, len(table.Rows))
	for _, row := range table.Rows {
		data := table.Data[row.DataOffset : row.DataOffset+npcThinkLayout.RowSize]
		senses[row.ID] = Senses{
			Eye:    float64(binary.LittleEndian.Uint16(data[thinkEyeDistOffset:])),
			Cone:   float64(data[thinkEyeAngYOffset]),
			Behind: eyeOrigin(data),
			Nose:   float64(binary.LittleEndian.Uint16(data[thinkFields["nose_dist"].offset:])),
		}
	}
	return senses, nil
}

// LevelSight is what a clone notices once a perception level is written over
// it: the writer's own profile (ladderProfile) for a body of that radius and
// front, with whatever the level leaves alone kept from the row's own senses -
// all of them for the Loner, and for a row kept as held.
func LevelSight(level string, body, front float64, own Senses, noTurn bool) Senses {
	profile := ladderProfile(level, body, front, make([]byte, npcThinkLayout.RowSize), noTurn)
	sight := own
	if value, written := profile["eye_dist"]; written {
		sight.Eye = value
	}
	if value, written := profile["eye_angY"]; written {
		sight.Cone = value
	}
	if value, written := profile["eye_BackOffsetDist"]; written {
		sight.Behind = value
	}
	if value, written := profile["nose_dist"]; written {
		sight.Nose = value
	}
	return sight
}
