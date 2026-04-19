package pack

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"strings"

	"github.com/disintegration/imaging"

	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// PreparePayload returns fpdf image type, bytes to embed, pixel dimensions after optional portrait→landscape rotation.
// JPEG/PNG landscape rasters pass through without re-encoding when possible.
func PreparePayload(raw []byte, ext string, rotateLosslessPNG bool) (imgType string, data []byte, w, h int, err error) {
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	switch ext {
	case ".jpg", ".jpeg":
		cfg, jerr := jpeg.DecodeConfig(bytes.NewReader(raw))
		if jerr != nil {
			return "", nil, 0, 0, jerr
		}
		if cfg.Height <= cfg.Width {
			return "jpg", raw, cfg.Width, cfg.Height, nil
		}
		img, jerr := jpeg.Decode(bytes.NewReader(raw))
		if jerr != nil {
			return "", nil, 0, 0, jerr
		}
		return EncodeRotatedLandscape(img, rotateLosslessPNG)

	case ".png":
		cfg, perr := png.DecodeConfig(bytes.NewReader(raw))
		if perr != nil {
			return "", nil, 0, 0, perr
		}
		if cfg.Height <= cfg.Width {
			return "png", raw, cfg.Width, cfg.Height, nil
		}
		img, perr := png.Decode(bytes.NewReader(raw))
		if perr != nil {
			return "", nil, 0, 0, perr
		}
		return EncodeRotatedLandscape(img, rotateLosslessPNG)

	default:
		img, _, derr := image.Decode(bytes.NewReader(raw))
		if derr != nil {
			return "", nil, 0, 0, fmt.Errorf("unsupported or corrupt format %s: %w", ext, derr)
		}
		b := img.Bounds()
		pw, ph := b.Dx(), b.Dy()
		if ph <= pw {
			return EncodeBitmapForPDF(img, rotateLosslessPNG)
		}
		return EncodeRotatedLandscape(img, rotateLosslessPNG)
	}
}

// EncodeBitmapForPDF encodes a decoded image for embedding (used for non-JPEG/PNG after decode).
func EncodeBitmapForPDF(img image.Image, usePNG bool) (imgType string, data []byte, w, h int, err error) {
	b := img.Bounds()
	w, h = b.Dx(), b.Dy()
	var buf bytes.Buffer
	if usePNG {
		if err := png.Encode(&buf, img); err != nil {
			return "", nil, 0, 0, err
		}
		return "png", buf.Bytes(), w, h, nil
	}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		return "", nil, 0, 0, err
	}
	return "jpg", buf.Bytes(), w, h, nil
}

// EncodeRotatedLandscape rotates 90° CCW then encodes.
func EncodeRotatedLandscape(img image.Image, usePNG bool) (imgType string, data []byte, w, h int, err error) {
	rot := imaging.Rotate90(img)
	b := rot.Bounds()
	w, h = b.Dx(), b.Dy()
	var buf bytes.Buffer
	if usePNG {
		if err := png.Encode(&buf, rot); err != nil {
			return "", nil, 0, 0, err
		}
		return "png", buf.Bytes(), w, h, nil
	}
	if err := jpeg.Encode(&buf, rot, &jpeg.Options{Quality: 100}); err != nil {
		return "", nil, 0, 0, err
	}
	return "jpg", buf.Bytes(), w, h, nil
}
