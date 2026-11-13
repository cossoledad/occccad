package thumbnail

import "math"

// Surface illumination is linear radiance. Decode each base channel once per
// triangle, then use a bounded display-transfer table in the raster loop.
// The 12-bit linear table has at most 1 byte of error for the sampled studio
// palette; it avoids a power function for every channel of every covered pixel.
func srgbLinear(value float64) float64 {
	if value <= .04045 {
		return value / 12.92
	}
	return math.Pow((value+.055)/1.055, 2.4)
}
func linearSRGB(value float64) float64 {
	if value <= .0031308 {
		return value * 12.92
	}
	return 1.055*math.Pow(value, 1/2.4) - .055
}

var surfaceLinear = func() [256]float64 {
	var table [256]float64
	for i := range table {
		table[i] = srgbLinear(float64(i)/255) * 4096
	}
	return table
}()
var surfaceDisplay = func() [4097]uint8 {
	var table [4097]uint8
	for i := range table {
		table[i] = uint8(math.Round(linearSRGB(float64(i)/4096) * 255))
	}
	return table
}()
