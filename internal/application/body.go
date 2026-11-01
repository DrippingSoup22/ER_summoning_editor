package application

import "github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"

// Body is a creature's footprint on the ground, in metres from its centre: how
// far it reaches in front and behind, and to either side. A frontend laying a
// formation out uses it to show where two summons would stand in each other.
type Body struct{ Front, Back, Side float64 }

// Bodies is every family's footprint, keyed by model: its look seen from
// above, a circle or a creature clearly longer than wide from where its tail
// ends to where its head ends (shapeOf).
func (session *Session) Bodies() (map[string]Body, error) {
	bodies := make(map[string]Body, len(session.service.families))
	for _, family := range session.service.families {
		shape, err := shapeOf(family)
		if err != nil {
			return nil, err
		}
		bodies[family.Model] = Body{Front: shape.Front, Back: shape.Back, Side: shape.Side}
	}
	return bodies, nil
}

// shapeOf is a family's body: research's (catalog.BodyOf) - the collision
// radius most of its placed rows carry, and its look - or, for a model
// research has no body for, a circle of the family's largest radius.
func shapeOf(family catalog.Family) (catalog.Shape, error) {
	shape, known, err := catalog.BodyOf(family.Model)
	if err != nil || known {
		return shape, err
	}
	radius := family.HitRadius
	return catalog.Shape{Radius: radius, Side: radius, Front: radius, Back: radius}, nil
}
