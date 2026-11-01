package catalog

import (
	"strings"
	"testing"
)

// A body table reads every model's four lengths, and a length that is not a
// number, or is below nought, refuses the table.
func TestTheBodyTableReadsOnlyLengths(t *testing.T) {
	good := "model,radius,side,front,back\nc5000,1.10,1.40,4.70,5.00\n"
	shapes, err := DecodeBodies(strings.NewReader(good))
	if err != nil {
		t.Fatal(err)
	}
	if got := shapes["c5000"]; got != (Shape{Radius: 1.1, Side: 1.4, Front: 4.7, Back: 5}) {
		t.Errorf("c5000 = %+v", got)
	}
	for _, bad := range []string{"c1,0.5,far,1,1", "c1,0.5,0.5,-1,1"} {
		if _, err := DecodeBodies(strings.NewReader("model,radius,side,front,back\n" + bad + "\n")); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// The shipped table keeps a horse longer than wide and the Crucible Knight a
// circle: its tail is out only in its attacks.
func TestTheShippedBodiesAreCirclesButTheLongCreatures(t *testing.T) {
	for model, want := range map[string]Shape{
		"c4361": {Radius: 0.7, Side: 0.7, Front: 1.9, Back: 2.5},
		"c2500": {Radius: 0.5, Side: 0.5, Front: 0.5, Back: 0.5},
	} {
		got, known, err := BodyOf(model)
		if err != nil || !known || got != want {
			t.Errorf("%s = %+v, %v, %v; want %+v", model, got, known, err, want)
		}
	}
}
