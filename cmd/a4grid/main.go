// a4grid packs images onto A4 using as many physical ISO A6 landscape frames
// (148 mm × 105 mm) as fit. Page orientation (portrait vs landscape) is chosen
// automatically to maximize the count (typically 4 on A4 landscape with no margin).
//
// Each bitmap is embedded at full resolution; only the PDF placement size in mm
// changes (contain inside the A6 slot), so print resolution comes from the file.
//
// Portrait inputs are rotated to landscape before placement (see -rotate-encode).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/go-pdf/fpdf"
)

var exts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true}

// ISO 216 A6 landscape: long edge horizontal.
const (
	a6LandscapeWMM = 148.0
	a6LandscapeHMM = 105.0
	a4PortraitWMM  = 210.0
	a4PortraitHMM  = 297.0
)

func main() {
	inputDir := flag.String("input", "", "directory containing images (required)")
	outputPath := flag.String("output", "", "output PDF path (required)")
	marginMM := flag.Float64("margin", 0,
		"symmetric page margin in mm; larger margins reduce how many physical A6 (148×105) tiles fit")
	rotateEncode := flag.String("rotate-encode", "jpeg",
		"for portrait files after rotation: jpeg (quality 100) or png (lossless from decoded pixels)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -input <dir> -output <file.pdf>\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr,
			"Packs images into physical A6 landscape slots (148×105 mm) on A4, as many as fit.\n"+
				"Chooses A4 portrait vs landscape to maximize slot count. Does not downsample pixels.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *inputDir == "" || *outputPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	re := strings.ToLower(strings.TrimSpace(*rotateEncode))
	if re != "jpeg" && re != "jpg" && re != "png" {
		fmt.Fprintln(os.Stderr, "-rotate-encode must be jpeg or png")
		os.Exit(2)
	}
	usePNGRotate := re == "png"

	paths, err := listImagePaths(*inputDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no supported images found in", *inputDir)
		os.Exit(1)
	}

	orient, cols, rows, pageW, pageH := bestA4PackA6Landscape(*marginMM)
	if cols < 1 || rows < 1 {
		fmt.Fprintf(os.Stderr, "margin %.1f mm is too large: no full A6 landscape (148×105 mm) tile fits on A4\n", *marginMM)
		os.Exit(1)
	}
	perPage := cols * rows
	fmt.Fprintf(os.Stderr, "layout: A4 %s, %d×%d = %d A6 landscape slots (148×105 mm) per page, margin %.1f mm\n",
		map[string]string{"L": "landscape", "P": "portrait"}[orient], cols, rows, perPage, *marginMM)

	pdf := fpdf.New(orient, "mm", "A4", "")

	innerW := pageW - 2*(*marginMM)
	innerH := pageH - 2*(*marginMM)
	totalW := float64(cols) * a6LandscapeWMM
	totalH := float64(rows) * a6LandscapeHMM
	offX := *marginMM + (innerW-totalW)/2
	offY := *marginMM + (innerH-totalH)/2

	for start := 0; start < len(paths); start += perPage {
		pdf.AddPage()
		end := start + perPage
		if end > len(paths) {
			end = len(paths)
		}
		for i := start; i < end; i++ {
			slot := i - start
			col := slot % cols
			row := slot / cols
			frameX := offX + float64(col)*a6LandscapeWMM
			frameY := offY + float64(row)*a6LandscapeHMM

			name := fmt.Sprintf("img_%d", i)
			imgType, payload, pw, ph, err := prepareForPDF(paths[i], usePNGRotate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", paths[i], err)
				os.Exit(1)
			}

			opt := fpdf.ImageOptions{ImageType: imgType, ReadDpi: false}
			pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(payload))
			if err := pdf.Error(); err != nil {
				fmt.Fprintf(os.Stderr, "register %s: %v\n", paths[i], err)
				os.Exit(1)
			}

			arImg := float64(pw) / float64(ph)
			arFrame := a6LandscapeWMM / a6LandscapeHMM
			var drawW, drawH float64
			if arImg > arFrame {
				drawW = a6LandscapeWMM
				drawH = a6LandscapeWMM / arImg
			} else {
				drawH = a6LandscapeHMM
				drawW = a6LandscapeHMM * arImg
			}
			x := frameX + (a6LandscapeWMM-drawW)/2
			y := frameY + (a6LandscapeHMM-drawH)/2

			pdf.ImageOptions(name, x, y, drawW, drawH, false, opt, 0, "")
			if err := pdf.Error(); err != nil {
				fmt.Fprintf(os.Stderr, "place %s: %v\n", paths[i], err)
				os.Exit(1)
			}
		}
	}

	if err := pdf.OutputFileAndClose(*outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// bestA4PackA6Landscape picks A4 portrait or landscape to maximize non-overlapping
// ISO A6 landscape tiles (148×105 mm) inside margins.
func bestA4PackA6Landscape(margin float64) (orient string, cols, rows int, pageW, pageH float64) {
	type trial struct {
		o      string
		pw, ph float64
	}
	trials := []trial{
		{"L", a4PortraitHMM, a4PortraitWMM}, // 297×210
		{"P", a4PortraitWMM, a4PortraitHMM}, // 210×297
	}
	bestN := -1
	for _, t := range trials {
		iw := t.pw - 2*margin
		ih := t.ph - 2*margin
		if iw < a6LandscapeWMM || ih < a6LandscapeHMM {
			continue
		}
		c := int(math.Floor(iw / a6LandscapeWMM))
		r := int(math.Floor(ih / a6LandscapeHMM))
		n := c * r
		if n > bestN {
			bestN = n
			orient = t.o
			cols, rows = c, r
			pageW, pageH = t.pw, t.ph
		}
	}
	if bestN < 0 {
		return "L", 0, 0, a4PortraitHMM, a4PortraitWMM
	}
	return orient, cols, rows, pageW, pageH
}

func listImagePaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !exts[ext] {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

// prepareForPDF returns image type, bytes, pixel size after optional portrait→landscape rotation.
// Landscape JPEG/PNG pass through as original bytes (no re-encode).
func prepareForPDF(path string, rotateLosslessPNG bool) (imgType string, data []byte, w, h int, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, 0, 0, err
	}
	ext := strings.ToLower(filepath.Ext(path))

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
		return encodeRotatedLandscape(img, rotateLosslessPNG)

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
		return encodeRotatedLandscape(img, rotateLosslessPNG)

	default:
		return "", nil, 0, 0, fmt.Errorf("unsupported extension %q", ext)
	}
}

func encodeRotatedLandscape(img image.Image, usePNG bool) (imgType string, data []byte, w, h int, err error) {
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
