// a4grid packs images onto A4 using as many physical ISO A6 landscape frames
// (148 mm × 105 mm) as fit. Page orientation (portrait vs landscape) is chosen
// automatically to maximize the count (typically 4 on A4 landscape with no margin).
//
// Input is either a directory of JPEG/PNG files or a single PDF; for PDFs, embedded
// images are extracted (pdfcpu) in page order, then the same layout rules apply.
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
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/go-pdf/fpdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var fileExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true}

// ISO 216 A6 landscape: long edge horizontal.
const (
	a6LandscapeWMM = 148.0
	a6LandscapeHMM = 105.0
	a4PortraitWMM  = 210.0
	a4PortraitHMM  = 297.0
)

// sourceImage is one raster to place (from a file path or extracted from a PDF).
type sourceImage struct {
	label string // path or "file.pdf (page 2, Im1)"
	raw   []byte
	ext   string // lowercase, includes dot, e.g. ".jpg"
}

func main() {
	inputPath := flag.String("input", "", "directory of JPEG/PNG images, or a .pdf file (required)")
	outputPath := flag.String("output", "", "output PDF path (required)")
	marginMM := flag.Float64("margin", 0,
		"symmetric page margin in mm; larger margins reduce how many physical A6 (148×105) tiles fit")
	rotateEncode := flag.String("rotate-encode", "jpeg",
		"for portrait files after rotation: jpeg (quality 100) or png (lossless from decoded pixels)")
	pdfPages := flag.String("pdf-pages", "",
		"when -input is a PDF: optional page selection (pdfcpu syntax), e.g. 1-3,5; default all pages")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -input <dir|file.pdf> -output <file.pdf>\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr,
			"Packs images into physical A6 landscape slots (148×105 mm) on A4, as many as fit.\n"+
				"Input may be a folder of images or a PDF (embedded images extracted in page order).\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *inputPath == "" || *outputPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	re := strings.ToLower(strings.TrimSpace(*rotateEncode))
	if re != "jpeg" && re != "jpg" && re != "png" {
		fmt.Fprintln(os.Stderr, "-rotate-encode must be jpeg or png")
		os.Exit(2)
	}
	usePNGRotate := re == "png"

	sources, err := loadSources(*inputPath, *pdfPages)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "no images to process from", *inputPath)
		os.Exit(1)
	}

	orient, cols, rows, pageW, pageH := bestA4PackA6Landscape(*marginMM)
	if cols < 1 || rows < 1 {
		fmt.Fprintf(os.Stderr, "margin %.1f mm is too large: no full A6 landscape (148×105 mm) tile fits on A4\n", *marginMM)
		os.Exit(1)
	}
	perPage := cols * rows
	fmt.Fprintf(os.Stderr, "layout: A4 %s, %d×%d = %d A6 landscape slots (148×105 mm) per page, margin %.1f mm; %d source image(s)\n",
		map[string]string{"L": "landscape", "P": "portrait"}[orient], cols, rows, perPage, *marginMM, len(sources))

	pdf := fpdf.New(orient, "mm", "A4", "")

	innerW := pageW - 2*(*marginMM)
	innerH := pageH - 2*(*marginMM)
	totalW := float64(cols) * a6LandscapeWMM
	totalH := float64(rows) * a6LandscapeHMM
	offX := *marginMM + (innerW-totalW)/2
	offY := *marginMM + (innerH-totalH)/2

	for start := 0; start < len(sources); start += perPage {
		pdf.AddPage()
		end := start + perPage
		if end > len(sources) {
			end = len(sources)
		}
		for i := start; i < end; i++ {
			slot := i - start
			col := slot % cols
			row := slot / cols
			frameX := offX + float64(col)*a6LandscapeWMM
			frameY := offY + float64(row)*a6LandscapeHMM

			name := fmt.Sprintf("img_%d", i)
			imgType, payload, pw, ph, err := preparePayload(sources[i].raw, sources[i].ext, usePNGRotate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", sources[i].label, err)
				os.Exit(1)
			}

			opt := fpdf.ImageOptions{ImageType: imgType, ReadDpi: false}
			pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(payload))
			if err := pdf.Error(); err != nil {
				fmt.Fprintf(os.Stderr, "register %s: %v\n", sources[i].label, err)
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
				fmt.Fprintf(os.Stderr, "place %s: %v\n", sources[i].label, err)
				os.Exit(1)
			}
		}
	}

	if err := pdf.OutputFileAndClose(*outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func loadSources(inputPath, pdfPages string) ([]sourceImage, error) {
	fi, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return sourcesFromDir(inputPath)
	}
	if strings.EqualFold(filepath.Ext(inputPath), ".pdf") {
		var sel []string
		if strings.TrimSpace(pdfPages) != "" {
			sel = []string{strings.TrimSpace(pdfPages)}
		}
		return sourcesFromPDF(inputPath, sel)
	}
	return nil, fmt.Errorf("-input must be a directory or a .pdf file, got %q", inputPath)
}

func sourcesFromDir(dir string) ([]sourceImage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !fileExts[ext] {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make([]sourceImage, 0, len(names))
	for _, n := range names {
		p := filepath.Join(dir, n)
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, sourceImage{
			label: p,
			raw:   raw,
			ext:   strings.ToLower(filepath.Ext(n)),
		})
	}
	return out, nil
}

func sourcesFromPDF(pdfPath string, pageSelection []string) ([]sourceImage, error) {
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	base := filepath.Base(pdfPath)
	var out []sourceImage
	conf := model.NewDefaultConfiguration()
	err = api.ExtractImages(f, pageSelection, func(img model.Image, _ bool, _ int) error {
		if img.Reader == nil {
			return nil
		}
		if img.Thumb || img.IsImgMask {
			return nil
		}
		data, err := io.ReadAll(img.Reader)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return nil
		}
		ext := strings.ToLower(img.FileType)
		if ext == "" {
			ext = ".bin"
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		label := fmt.Sprintf("%s (page %d, obj %d, %s)", base, img.PageNr, img.ObjNr, img.Name)
		out = append(out, sourceImage{label: label, raw: data, ext: ext})
		return nil
	}, conf)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no suitable embedded images found in %s (masks/thumbnails skipped)", pdfPath)
	}
	return out, nil
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

// preparePayload returns fpdf image type, bytes to embed, pixel dimensions after optional portrait→landscape rotation.
// JPEG/PNG landscape rasters pass through without re-encoding when possible.
func preparePayload(raw []byte, ext string, rotateLosslessPNG bool) (imgType string, data []byte, w, h int, err error) {
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
		img, _, derr := image.Decode(bytes.NewReader(raw))
		if derr != nil {
			return "", nil, 0, 0, fmt.Errorf("unsupported or corrupt format %s: %w", ext, derr)
		}
		b := img.Bounds()
		pw, ph := b.Dx(), b.Dy()
		if ph <= pw {
			return encodeBitmapForPDF(img, rotateLosslessPNG)
		}
		return encodeRotatedLandscape(img, rotateLosslessPNG)
	}
}

func encodeBitmapForPDF(img image.Image, usePNG bool) (imgType string, data []byte, w, h int, err error) {
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
