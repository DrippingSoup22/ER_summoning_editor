package catalog

// Each creature model's body on the ground.
//
// A body is the creature's collision radius - the one most of the rows the
// game places carry - and its look seen from above: its collision circle, or,
// for a creature research lists, how wide it looks and how far its head and
// its tail reach in the standing pose (body-shapes.csv). The placement grid lays the look
// out, and the perception ladder takes the radius for the body's width and
// counts a level's reach past the look's front: counted from the radius
// alone, the Flying Dragon saw only under its head.
//
// Not the skeleton: a model's bind pose holds the parts the game shows only in
// an attack, as the Crucible Knight's tail, and helper bones that are not
// flesh, which ran one dragon 105 m behind itself.

import (
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/DrippingSoup22/ER_summoning_editor/reference"
)

// Shape is a body in metres from its centre: its collision Radius, and its
// look - Side either way, Front ahead and Back behind, each never less than
// Side.
type Shape struct{ Radius, Side, Front, Back float64 }

// DecodeBodies reads the body table: one shape per model.
func DecodeBodies(source io.Reader) (map[string]Shape, error) {
	records, err := readTable(source, "body table", []string{"model", "radius", "side", "front", "back"})
	if err != nil {
		return nil, err
	}
	shapes := make(map[string]Shape, len(records))
	for _, record := range records {
		var values [4]float64
		for at, column := range [...]string{"radius", "side", "front", "back"} {
			value, err := strconv.ParseFloat(record[column], 64)
			if err != nil || value < 0 {
				return nil, fmt.Errorf("body table: %s %s %q is not a length", record["model"], column, record[column])
			}
			values[at] = value
		}
		shapes[record["model"]] = Shape{Radius: values[0], Side: values[1], Front: values[2], Back: values[3]}
	}
	return shapes, nil
}

var bodies = sync.OnceValues(func() (map[string]Shape, error) {
	return DecodeBodies(reference.BodyCatalog())
})

// BodyOf is a model's body, and whether the table knows the model.
func BodyOf(model string) (Shape, bool, error) {
	shapes, err := bodies()
	if err != nil {
		return Shape{}, false, err
	}
	shape, known := shapes[model]
	return shape, known, nil
}
