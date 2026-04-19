# image-converter

Small Go utilities that take **photos or raster PDFs** and build **print-ready A4 PDFs**: images are placed in fixed **millimetre** frames on the page, embedded at **full pixel resolution** (the PDF scales them at print time). Portrait shots are optionally rotated to landscape before layout.

## Requirements

- [Go](https://go.dev/) **1.23** or newer (pdfcpu needs 1.23+)

## Build

```bash
go build -o a4grid ./cmd/a4grid
go build -o ticketpack ./cmd/ticketpack
```

## Tools

### `a4grid` — A6-sized photo sheets

Packs images into **ISO A6 landscape** slots (**148 × 105 mm**) on **A4**. The program tries **A4 portrait and landscape** and picks whichever fits **more** complete tiles (with **zero margin**, landscape A4 usually fits **four** slots in a 2×2 grid).

**Input**

- A **folder** of `.jpg`, `.jpeg`, or `.png` (sorted by file name), or  
- A **PDF**: embedded images are extracted with [pdfcpu](https://github.com/pdfcpu/pdfcpu) in page order (thumbnails and image masks are skipped).

**Example**

```bash
./a4grid -input ./input/set1 -output ./output/photos-a4.pdf
./a4grid -input ./scans.pdf -output ./output/from-pdf.pdf -pdf-pages "1-4"
```

Run `./a4grid -h` for flags (`-margin`, `-rotate-encode`, `-pdf-pages`, etc.).

---

### `ticketpack` — boarding pass layout

Packs images into **airline-style boarding pass** slots (**203 × 82 mm**, wide × tall) on **A4**, again choosing portrait vs landscape to fit **as many full-size tickets** as possible (typically **three** per A4 portrait page at zero margin).

**Input**

- Same as `a4grid` (directory or PDF).  
- Default input directory: **`input/tickets`** (override with `-input`).

**Example**

```bash
./ticketpack -output ./output/boarding-a4.pdf
./ticketpack -input ./input/tickets -output ./output/boarding-a4.pdf
./ticketpack -input ./passes.pdf -output ./output/boarding.pdf -pdf-pages "1-10"
```

Run `./ticketpack -h` for all options.

---

## Shared behaviour

| Topic | Behaviour |
|--------|-----------|
| **Layout** | Each image is **letterboxed** inside its slot (aspect ratio preserved). |
| **Resolution** | JPEG/PNG that are already **landscape** are embedded **without re-encoding**. Portrait sources are decoded, rotated 90°, then encoded again (`-rotate-encode jpeg` at quality 100, or `png` for a lossless intermediate). |
| **PDF pages** | `-pdf-pages` uses **pdfcpu** page selection syntax (e.g. `1-3,5`). Empty means all pages. |
| **Library** | `internal/pack` holds shared loading, image preparation, and A4 packing math. |

## Repository layout

```
cmd/a4grid/        # A6-on-A4 composer
cmd/ticketpack/    # 203×82 mm boarding passes on A4
internal/pack/     # Shared PDF/image helpers
```

## Limits (paper geometry)

Slot sizes are fixed in **millimetres**. How many fit on one A4 sheet follows from **210 × 297 mm** (and the landscape swap); larger **margins** reduce the count. The programs print a one-line **layout summary** to stderr (grid size and slot dimensions).
