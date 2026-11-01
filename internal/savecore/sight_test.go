package savecore

import "testing"

// The sight a frontend draws is the profile the writer writes: a ladder level
// sets the cone, the reach past the front and the cone's apex at the centre;
// the Loner and a row kept as held keep the row's own senses; the Statue sees
// nothing.
func TestLevelSightIsWhatTheWriterWrites(t *testing.T) {
	own := Senses{Eye: 40, Cone: 120, Behind: 3, Nose: 8}
	for _, want := range []struct {
		level string
		sight Senses
	}{
		{"guard", Senses{Eye: 14, Cone: 90, Behind: 0, Nose: 2}},
		{"hunter", Senses{Eye: 52, Cone: 180, Behind: 0, Nose: 27}},
		{"loner", own},
		{"as-held", own},
		{"statue", Senses{Eye: 0, Cone: 120, Behind: 3, Nose: 0}},
	} {
		if got := LevelSight(want.level, 1, 1.5, own, false); got != want.sight {
			t.Errorf("%s sees %+v, want %+v", want.level, got, want.sight)
		}
	}
}
