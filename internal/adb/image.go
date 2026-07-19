package adb

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// pngMagic is the 8-byte PNG signature every PNG starts with.
var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// pngFromScreencap returns the PNG payload from a `screencap -p` stdout buffer,
// dropping any leading bytes before the PNG signature.
//
// On a device with more than one physical display (a foldable's inner + cover
// screens), `screencap -p` with no `-d` prints a multi-line "[Warning] Multiple
// displays were found, but no display id was specified! ..." to STDOUT ahead of
// the PNG, shifting the header by a few hundred bytes so no decoder can read its
// dimensions — the capture looks corrupt or blank though the PNG itself is
// intact, just prefixed. Seeking to the signature recovers it, and it is a
// belt-and-suspenders guard even when `-d` is passed (stray warnings must never
// reach the image buffer). If the signature is absent — a genuine failure, e.g.
// the device printed "Failed to take the screenshot" — the input is returned
// unchanged so the caller still sees whatever came back.
func pngFromScreencap(raw []byte) []byte {
	if i := bytes.Index(raw, pngMagic); i > 0 {
		return raw[i:]
	}
	return raw
}

// isMostlyBlack reports whether a PNG is (near-)entirely black — the signature
// of a failed/blanked screencap (an intermittent bad grab, a FLAG_SECURE
// window, or a sleeping display) rather than a real screen. It samples a grid
// of pixels (full scans are unnecessary and slow on full-res frames) and treats
// the frame as black only when essentially every sample is near-black, so a
// dark-theme UI — which still has icons, text, and chrome — does not trip it.
// A frame it can't decode is reported as not-black (nothing to diagnose).
func isMostlyBlack(data []byte) bool {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return false
	}
	stepX := max(1, w/200)
	stepY := max(1, h/200)
	var sampled, nonBlack int
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, _ := src.At(x, y).RGBA() // 16-bit; >>8 → 0..255
			if r>>8 > 6 || g>>8 > 6 || bl>>8 > 6 {
				nonBlack++
			}
			sampled++
		}
	}
	if sampled == 0 {
		return false
	}
	// Allow a hair of noise but require ~everything to be black — a true failed
	// grab is uniformly (0,0,0).
	return float64(nonBlack)/float64(sampled) < 0.001
}

// downscalePNG shrinks a PNG so its largest dimension is at most maxDim, using a
// box-average filter for readable text. If the image already fits (or maxDim is
// non-positive, or decoding fails) the original bytes are returned unchanged.
// The returned width/height describe the bytes actually returned.
func downscalePNG(data []byte, maxDim int) (out []byte, w, h int) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data, 0, 0
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if maxDim <= 0 || (sw <= maxDim && sh <= maxDim) {
		return data, sw, sh
	}

	scale := float64(maxDim) / float64(max(sw, sh))
	dw := max(1, int(float64(sw)*scale))
	dh := max(1, int(float64(sh)*scale))

	// RGBA (like the values RGBA() returns) is alpha-premultiplied, so averaging
	// the samples and storing them here keeps the color space consistent. (For
	// the opaque screenshots this handles it makes no difference, but it is the
	// correct model should a translucent PNG ever be passed in.)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for dy := range dh {
		sy0 := b.Min.Y + dy*sh/dh
		sy1 := b.Min.Y + (dy+1)*sh/dh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := range dw {
			sx0 := b.Min.X + dx*sw/dw
			sx1 := b.Min.X + (dx+1)*sw/dw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, a, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r += uint64(cr)
					g += uint64(cg)
					bl += uint64(cb)
					a += uint64(ca)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8((r / n) >> 8),
				G: uint8((g / n) >> 8),
				B: uint8((bl / n) >> 8),
				A: uint8((a / n) >> 8),
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return data, sw, sh
	}
	return buf.Bytes(), dw, dh
}
