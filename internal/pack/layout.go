package pack

import "math"

// A4 dimensions (portrait: width × height in mm).
const (
	A4PortraitWMM = 210.0
	A4PortraitHMM = 297.0
)

// BestA4Pack picks A4 portrait or landscape to maximize a grid of slotW×slotH mm tiles inside symmetric margins.
func BestA4Pack(margin, slotWMM, slotHMM float64) (orient string, cols, rows int, pageW, pageH float64) {
	type trial struct {
		o      string
		pw, ph float64
	}
	trials := []trial{
		{"L", A4PortraitHMM, A4PortraitWMM}, // 297×210
		{"P", A4PortraitWMM, A4PortraitHMM}, // 210×297
	}
	bestN := -1
	for _, t := range trials {
		iw := t.pw - 2*margin
		ih := t.ph - 2*margin
		if iw < slotWMM || ih < slotHMM {
			continue
		}
		c := int(math.Floor(iw / slotWMM))
		r := int(math.Floor(ih / slotHMM))
		n := c * r
		if n > bestN {
			bestN = n
			orient = t.o
			cols, rows = c, r
			pageW, pageH = t.pw, t.ph
		}
	}
	if bestN < 0 {
		return "L", 0, 0, A4PortraitHMM, A4PortraitWMM
	}
	return orient, cols, rows, pageW, pageH
}
