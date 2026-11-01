package character

// Effects saved on a character. The slot keeps one 16-byte record per
// SpEffectParam save category, thirteen of them, right after PlayerGameData,
// and the game restores each record on load. So an effect written here is on the character
// with no item to carry it - the carrier the author chose for the
// summon-anywhere effect (2026-10-26).
//
// A record is the effect id and three words; an empty one is -1 and zeroes.
// Batch 77 wrote (19982, -1.0, 0, 0) and the game loaded it, though the form
// glitched (the three words are not quite what the game writes), so this
// writes the same.

import (
	"encoding/binary"
	"fmt"
	"math"
)

// NoSavedEffect empties a record.
const NoSavedEffect = -1

// SetSavedEffect writes effect into one character's record for a save
// category, or empties it with NoSavedEffect. The file is changed in place.
func SetSavedEffect(file []byte, slot, category int, effect int32) error {
	if category < 0 || category >= savedEffects {
		return fmt.Errorf("save category %d is outside 0..%d", category, savedEffects-1)
	}
	if effect <= 0 && effect != NoSavedEffect {
		return fmt.Errorf("effect %d is not an effect id", effect)
	}
	data, magic, err := characterSlot(file, slot)
	if err != nil {
		return err
	}
	record := data[magic+1+category*savedEffectRecord:][:savedEffectRecord]
	clear(record)
	binary.LittleEndian.PutUint32(record, uint32(effect))
	if effect != NoSavedEffect {
		binary.LittleEndian.PutUint32(record[4:], math.Float32bits(-1))
	}
	return nil
}
