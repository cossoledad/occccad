package thumbnail

import (
	"context"

	"github.com/occccad/occccad/internal/visual"
	"image/color"
	"math"
)

type lineProfile struct {
	sharp, suppressed [2]float64
	explicitColor     bool
}

var studioEdgeColor = color.RGBA{52, 67, 78, 255}
var studioEdgeInk = func() [257]color.RGBA {
	var result [257]color.RGBA
	for i := range result {
		convert := func(c uint8) uint8 {
			v := linearSRGB(srgbLinear(float64(c)/255) * float64(i) / 256)
			return uint8(math.Round(v * 255))
		}
		result[i] = color.RGBA{convert(studioEdgeColor.R), convert(studioEdgeColor.G), convert(studioEdgeColor.B), 255}
	}
	return result
}()

func prepareEdgeStyles(ctx context.Context, profile *visual.EdgeProfile) ([]lineProfile, error) {
	if len(profile.Metadata) == 0 || len(profile.Positions) != len(profile.Metadata)*2 || len(profile.NormalA) != len(profile.Positions) || len(profile.NormalB) != len(profile.Positions) {
		return nil, nil
	}
	result := make([]lineProfile, len(profile.Metadata))
	lo, hi := math.Cos(35*math.Pi/180), math.Cos(5*math.Pi/180)
	for i, m := range profile.Metadata {
		if i%512 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		kind, quality := m[3]&255, m[3]>>8
		for end := 0; end < 2; end++ {
			result[i].sharp[end] = 1
			a, b := unit(profile.NormalA[i*2+end]), unit(profile.NormalB[i*2+end])
			if kind < 3 || kind > 5 || quality&1 != 0 || a == (vec{}) || b == (vec{}) {
				continue
			}
			cosine := max(-1, min(1, dot(a, b)))
			t := max(0, min(1, (cosine-lo)/(hi-lo)))
			result[i].sharp[end] = 1 - t*t*(3-2*t)
			if (kind == 4 || kind == 5) && cosine > .999998 {
				result[i].suppressed[end] = 1
			}
		}
	}
	return result, nil
}
