package routes

import "slices"

// Placing a route group by where it ends up, for what has no rows to drag:
// the command line and agents. MoveGroup is the drag; these say the place.

// MovableOrder are the groups that can move, in the order they apply:
// every group but the default route, which is always last (Listed).
func (p Plan) MovableOrder() []string {
	var out []string
	for _, id := range p.GroupOrder() {
		if Movable(id) {
			out = append(out, id)
		}
	}
	return out
}

// PlaceGroup moves group id to place at (from 0) among MovableOrder, the
// others keeping their order. A place past the end is the last. It reports
// whether anything moved.
func (p *Plan) PlaceGroup(id string, at int) bool {
	order := p.MovableOrder()
	from := slices.Index(order, id)
	if from < 0 || at < 0 {
		return false
	}
	at = min(at, len(order)-1)
	if at == from {
		return false
	}
	return p.MoveGroup(id, order[at]) // the group at its place makes room
}

// PlaceAt is where group id goes among MovableOrder to come just before
// (after false) or just after group next to, -1 when either can't move.
func (p Plan) PlaceAt(id, to string, after bool) int {
	order := p.MovableOrder()
	from, at := slices.Index(order, id), slices.Index(order, to)
	if from < 0 || at < 0 {
		return -1
	}
	if from == at {
		return from
	}
	if after {
		at++
	}
	if from < at { // taking id out moves to up by one
		at--
	}
	return at
}
