package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// generateTrayIcon creates a 64x64 PNG icon for the system tray.
// A dark blue circle with a cyan "S" curve — matches the app's dark theme.
func generateTrayIcon() []byte {
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	center := float64(size) / 2
	radius := float64(size)/2 - 2

	bg := color.RGBA{15, 23, 42, 255}    // #0f172a (app background)
	fg := color.RGBA{56, 189, 248, 255}   // #38bdf8 (cyan accent)
	ring := color.RGBA{30, 41, 59, 255}   // #1e293b (darker ring)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(x) - center + 0.5
			fy := float64(y) - center + 0.5
			dist := math.Sqrt(fx*fx + fy*fy)

			if dist > radius+1 {
				continue // transparent outside
			}

			// Anti-aliased circle edge
			if dist > radius {
				alpha := uint8(255 * (radius + 1 - dist))
				img.Set(x, y, color.RGBA{bg.R, bg.G, bg.B, alpha})
				continue
			}

			// Outer ring
			if dist > radius-3 {
				img.Set(x, y, ring)
				continue
			}

			// Background
			img.Set(x, y, bg)

			// Draw "S" shape using two offset arcs
			// Top arc: center at (center-4, center-10), radius 14
			// Bottom arc: center at (center+4, center+10), radius 14
			topCx, topCy := center-4, center-10
			botCx, botCy := center+4, center+10
			arcR := 14.0
			strokeW := 5.0

			topDist := math.Sqrt((fx-(topCx-center))*(fx-(topCx-center)) + (fy-(topCy-center))*(fy-(topCy-center)))
			botDist := math.Sqrt((fx-(botCx-center))*(fx-(botCx-center)) + (fy-(botCy-center))*(fy-(botCy-center)))

			topStroke := math.Abs(topDist-arcR) < strokeW
			botStroke := math.Abs(botDist-arcR) < strokeW

			// Top arc: right half (x > center offset)
			if topStroke && float64(x) > topCx-2 && float64(y) < center+2 {
				img.Set(x, y, fg)
			}
			// Bottom arc: left half
			if botStroke && float64(x) < botCx+2 && float64(y) > center-2 {
				img.Set(x, y, fg)
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
