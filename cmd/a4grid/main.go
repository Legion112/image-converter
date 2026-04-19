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
	"os"
	"path/filepath"
	"strings"

	"github.com/go-pdf/fpdf"

	"github.com/legion112/image-converter/internal/pack"
)

const (
	a6LandscapeWMM = 148.0
	a6LandscapeHMM = 105.0
)

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

	sources, err := pack.LoadSources(*inputPath, *pdfPages)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(sources) == 0 {
		fmt.Fprintln(os.Stderr, "no images to process from", *inputPath)
		os.Exit(1)
	}

	orient, cols, rows, pageW, pageH := pack.BestA4Pack(*marginMM, a6LandscapeWMM, a6LandscapeHMM)
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
			imgType, payload, pw, ph, err := pack.PreparePayload(sources[i].Raw, sources[i].Ext, usePNGRotate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", sources[i].Label, err)
				os.Exit(1)
			}

			opt := fpdf.ImageOptions{ImageType: imgType, ReadDpi: false}
			pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(payload))
			if err := pdf.Error(); err != nil {
				fmt.Fprintf(os.Stderr, "register %s: %v\n", sources[i].Label, err)
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
				fmt.Fprintf(os.Stderr, "place %s: %v\n", sources[i].Label, err)
				os.Exit(1)
			}
		}
	}

	if err := pdf.OutputFileAndClose(*outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
