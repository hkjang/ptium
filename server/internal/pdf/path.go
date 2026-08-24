package pdf

import (
	"strconv"
	"strings"
)

// flattenPath walks the path data the drawing writes and gives back the outline
// as points.
//
// Only what is drawn is understood: move, line, the horizontal and vertical
// shorthands, quarter-circle arcs on the corners of a panel, and close. An arc
// becomes eight straight steps, which at the size a rounded corner is drawn is
// a curve to anyone looking at it.
func flattenPath(data string) []Position {
	tokens := tokenizePath(data)
	var points []Position
	var at Position
	index := 0
	next := func() Point {
		if index >= len(tokens) {
			return 0
		}
		value, _ := strconv.ParseFloat(tokens[index], 64)
		index++
		return value
	}
	for index < len(tokens) {
		command := tokens[index]
		if len(command) != 1 || (command[0] >= '0' && command[0] <= '9') || command[0] == '-' || command[0] == '.' {
			// A repeated command: the previous one continues.
			index++
			continue
		}
		index++
		switch command {
		case "M", "L":
			at = Position{X: next(), Y: next()}
			points = append(points, at)
		case "m", "l":
			at = Position{X: at.X + next(), Y: at.Y + next()}
			points = append(points, at)
		case "H":
			at.X = next()
			points = append(points, at)
		case "h":
			at.X += next()
			points = append(points, at)
		case "V":
			at.Y = next()
			points = append(points, at)
		case "v":
			at.Y += next()
			points = append(points, at)
		case "A", "a":
			radiusX, radiusY := next(), next()
			next() // rotation
			next() // large arc
			sweep := next()
			end := Position{X: next(), Y: next()}
			if command == "a" {
				end = Position{X: at.X + end.X, Y: at.Y + end.Y}
			}
			points = append(points, arcSteps(at, end, radiusX, radiusY, sweep == 1)...)
			at = end
		case "Z", "z":
			if len(points) > 0 {
				at = points[0]
			}
		}
	}
	return points
}

// arcSteps walks a quarter-circle corner from one point to another. The centre
// is the corner the two points share, which is what a rounded rectangle's arc
// always is in the drawings this reads.
func arcSteps(from, to Position, radiusX, radiusY Point, sweep bool) []Position {
	if radiusX <= 0 || radiusY <= 0 {
		return []Position{to}
	}
	centre := Position{X: from.X, Y: to.Y}
	if sweep == (from.X < to.X) == (from.Y < to.Y) {
		centre = Position{X: to.X, Y: from.Y}
	}
	const steps = 8
	var out []Position
	startX, startY := from.X-centre.X, from.Y-centre.Y
	endX, endY := to.X-centre.X, to.Y-centre.Y
	for step := 1; step <= steps; step++ {
		ratio := float64(step) / steps
		// A straight walk in the corner's own square, pushed out to the radius.
		x := startX + (endX-startX)*ratio
		y := startY + (endY-startY)*ratio
		length := (x*x/(radiusX*radiusX) + y*y/(radiusY*radiusY))
		if length > 0 {
			scale := 1 / sqrt(length)
			x, y = x*scale, y*scale
		}
		out = append(out, Position{X: centre.X + x, Y: centre.Y + y})
	}
	return out
}

func sqrt(value float64) float64 {
	if value <= 0 {
		return 0
	}
	guess := value
	for range 20 {
		guess = (guess + value/guess) / 2
	}
	return guess
}

func tokenizePath(data string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for index := 0; index < len(data); index++ {
		character := data[index]
		switch {
		case character == ' ' || character == ',' || character == '\n' || character == '\t':
			flush()
		case (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z'):
			flush()
			tokens = append(tokens, string(character))
		case character == '-' && current.Len() > 0 && current.String() != "e":
			flush()
			current.WriteByte(character)
		default:
			current.WriteByte(character)
		}
	}
	flush()
	return tokens
}
