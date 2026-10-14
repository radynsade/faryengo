package categories

import (
	"errors"
	"slices"
)

//
// Color
//

type Color string

const (
	ColorGreen  Color = "green"
	ColorBlue   Color = "blue"
	ColorOrange Color = "orange"
	ColorRed    Color = "red"
	ColorPurple Color = "purple"
	ColorBrown  Color = "brown"
	ColorGrey   Color = "grey"
	ColorYellow Color = "yellow"
)

var colorCatalog = []Color{
	ColorGreen,
	ColorBlue,
	ColorOrange,
	ColorRed,
	ColorPurple,
	ColorBrown,
	ColorGrey,
	ColorYellow,
}

var ErrColorInvalid = errors.New("invalid color")

func NewColor(value string) (Color, error) {
	var (
		err   error
		color Color
	)

	color = Color(value)
	err = color.Validate()
	return color, err
}

// Colors lists every known color in the order they are offered.

func Colors() []Color {
	return slices.Clone(colorCatalog)
}

func (c Color) Validate() error {
	var err error

	if !slices.Contains(colorCatalog, c) {
		err = ErrColorInvalid
	}

	return err
}
