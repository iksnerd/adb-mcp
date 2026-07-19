package adb

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// solidPNG encodes a w×h image filled with c.
func solidPNG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestIsMostlyBlack(t *testing.T) {
	black := color.RGBA{0, 0, 0, 255}

	if !isMostlyBlack(solidPNG(t, 200, 400, black)) {
		t.Error("solid black image should be detected as mostly black")
	}
	if isMostlyBlack(solidPNG(t, 200, 400, color.RGBA{255, 255, 255, 255})) {
		t.Error("solid white image should not be mostly black")
	}
	if isMostlyBlack(solidPNG(t, 200, 400, color.RGBA{20, 20, 20, 255})) {
		t.Error("dark-gray (above the near-black threshold) should not be mostly black")
	}

	// A mostly-black frame with a real chunk of bright content (like a splash
	// logo or dark-theme UI) must NOT be flagged — only near-total black is.
	img := image.NewRGBA(image.Rect(0, 0, 200, 400))
	for y := range 400 {
		for x := range 200 {
			if x < 40 { // 20% of the frame is bright
				img.Set(x, y, color.RGBA{200, 200, 200, 255})
			} else {
				img.Set(x, y, black)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if isMostlyBlack(buf.Bytes()) {
		t.Error("a frame with 20% bright content should not be mostly black")
	}

	// Garbage / undecodable input is reported as not-black (nothing to diagnose).
	if isMostlyBlack([]byte("not a png")) {
		t.Error("undecodable data should be reported as not black")
	}
}
