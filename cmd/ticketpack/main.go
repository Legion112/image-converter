// ticketpack lays out airline-style boarding passes (203×82 mm) on A4.
// It picks A4 portrait vs landscape to fit as many full-size tickets as possible
// and embeds rasters at full resolution (contain inside each ticket slot).
//
// Input: directory of JPEG/PNG (default input/tickets) or a PDF of embedded images.
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

// IATA-style boarding pass print size (wide × tall in mm).
const (
	boardingPassWMM = 203.0
	boardingPassHMM = 82.0
)

func main() {
	inputPath := flag.String("input", "input/tickets",
		"directory of JPEG/PNG images, or a .pdf file (default: input/tickets)")
	outputPath := flag.String("output", "", "output PDF path (required)")
	marginMM := flag.Float64("margin", 0,
		"symmetric page margin in mm; larger margins may reduce how many 203×82 mm tickets fit")
	rotateEncode := flag.String("rotate-encode", "jpeg",
		"for portrait files after rotation: jpeg (quality 100) or png (lossless from decoded pixels)")
	pdfPages := flag.String("pdf-pages", "",
		"when -input is a PDF: optional page selection (pdfcpu syntax), e.g. 1-3,5; default all pages")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -output <file.pdf> [-input <dir|file.pdf>]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr,
			"Packs boarding-pass images into fixed 203×82 mm slots on A4 (as many as fit).\n"+
				"Chooses A4 orientation to maximize count. Does not downsample pixels.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *outputPath == "" {
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

	orient, cols, rows, pageW, pageH := pack.BestA4Pack(*marginMM, boardingPassWMM, boardingPassHMM)
	if cols < 1 || rows < 1 {
		fmt.Fprintf(os.Stderr, "margin %.1f mm is too large: no full 203×82 mm ticket fits on A4\n", *marginMM)
		os.Exit(1)
	}
	perPage := cols * rows
	fmt.Fprintf(os.Stderr, "layout: A4 %s, %d×%d = %d boarding pass slots (203×82 mm) per page, margin %.1f mm; %d source image(s)\n",
		map[string]string{"L": "landscape", "P": "portrait"}[orient], cols, rows, perPage, *marginMM, len(sources))

	pdf := fpdf.New(orient, "mm", "A4", "")

	innerW := pageW - 2*(*marginMM)
	innerH := pageH - 2*(*marginMM)
	totalW := float64(cols) * boardingPassWMM
	totalH := float64(rows) * boardingPassHMM
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
			frameX := offX + float64(col)*boardingPassWMM
			frameY := offY + float64(row)*boardingPassHMM

			name := fmt.Sprintf("ticket_%d", i)
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
			arFrame := boardingPassWMM / boardingPassHMM
			var drawW, drawH float64
			if arImg > arFrame {
				drawW = boardingPassWMM
				drawH = boardingPassWMM / arImg
			} else {
				drawH = boardingPassHMM
				drawW = boardingPassHMM * arImg
			}
			x := frameX + (boardingPassWMM-drawW)/2
			y := frameY + (boardingPassHMM-drawH)/2

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
