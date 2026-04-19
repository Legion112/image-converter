package pack

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// FileExts lists raster extensions accepted from directories.
var FileExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true}

// SourceImage is one raster to place (from a file path or extracted from a PDF).
type SourceImage struct {
	Label string
	Raw   []byte
	Ext   string // lowercase, includes dot, e.g. ".jpg"
}

// LoadSources reads images from a directory (sorted by name) or extracts them from a PDF.
func LoadSources(inputPath, pdfPages string) ([]SourceImage, error) {
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

func sourcesFromDir(dir string) ([]SourceImage, error) {
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
		if !FileExts[ext] {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make([]SourceImage, 0, len(names))
	for _, n := range names {
		p := filepath.Join(dir, n)
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, SourceImage{
			Label: p,
			Raw:   raw,
			Ext:   strings.ToLower(filepath.Ext(n)),
		})
	}
	return out, nil
}

func sourcesFromPDF(pdfPath string, pageSelection []string) ([]SourceImage, error) {
	f, err := os.Open(pdfPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	base := filepath.Base(pdfPath)
	var out []SourceImage
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
		out = append(out, SourceImage{Label: label, Raw: data, Ext: ext})
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
