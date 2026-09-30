// example.go draws Example.pdf, an A4 page in OEM12x20, as PDF/A-3a and
// PDF/UA-1: a banner drawn from the font's own pixels, an 80 by 25 text-mode
// screen in the colours of DOS with the 256 characters of code page 437, and
// a sunset at sea in block and shade characters.
//
//	go run .
//
// It builds with PDFjet checked out next to this folder, in ../pdfjet.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	pdfjet "github.com/edragoev1/pdfjet/v9/src"
	"github.com/edragoev1/pdfjet/v9/src/a4"
	"github.com/edragoev1/pdfjet/v9/src/compliance"
)

// Code page 437, as OEM12x20 maps it to Unicode; NUL, the space and 0xFF
// are blank
const cp437 = " ☺☻♥♦♣♠•◘○◙♂♀♪♫☼►◄↕‼¶§▬↨↑↓→←∟↔▲▼" +
	" !\"#$%&'()*+,-./0123456789:;<=>?" +
	"@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_" +
	"`abcdefghijklmnopqrstuvwxyz{|}~⌂" +
	"ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒ" +
	"áíóúñÑªº¿⌐¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐" +
	"└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀" +
	"αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ "

// The 16 colours of the text mode of a VGA card
const (
	black        = 0x000000
	blue         = 0x0000AA
	green        = 0x00AA00
	cyan         = 0x00AAAA
	red          = 0xAA0000
	magenta      = 0xAA00AA
	brown        = 0xAA5500
	lightGray    = 0xAAAAAA
	darkGray     = 0x555555
	lightBlue    = 0x5555FF
	lightGreen   = 0x55FF55
	lightCyan    = 0x55FFFF
	lightRed     = 0xFF5555
	lightMagenta = 0xFF55FF
	yellow       = 0xFFFF55
	white        = 0xFFFFFF

	none  = -1       // no background: the paper shows
	ink   = 0x333333 // the text on the paper
	bezel = 0x2B2B2B // the monitor around the screen
)

// A grid of cells, each a character in a colour on a background
type grid struct {
	cols, rows int
	char       [][]rune
	fg, bg     [][]int32
}

func newGrid(cols, rows int, fg, bg int32) *grid {
	g := &grid{cols: cols, rows: rows}
	for range rows {
		g.char = append(g.char, make([]rune, cols))
		g.fg = append(g.fg, make([]int32, cols))
		g.bg = append(g.bg, make([]int32, cols))
	}
	g.fill(0, 0, cols, rows, fg, bg)
	return g
}

func (g *grid) fill(col, row, w, h int, fg, bg int32) {
	for r := row; r < row+h; r++ {
		for c := col; c < col+w; c++ {
			g.char[r][c], g.fg[r][c], g.bg[r][c] = ' ', fg, bg
		}
	}
}

// Writes text from (col, row) in the colour fg, on bg, or, with bg none, on
// the backgrounds already there. A full block on a background gets one of
// its own colour, so that no hairline shows between two blocks.
func (g *grid) put(col, row int, text string, fg, bg int32) {
	for _, ch := range text {
		if col >= 0 && col < g.cols && row >= 0 && row < g.rows {
			g.char[row][col], g.fg[row][col] = ch, fg
			if bg != none {
				g.bg[row][col] = bg
			}
			if ch == '█' && g.bg[row][col] != none {
				g.bg[row][col] = fg
			}
		}
		col++
	}
}

// A panel with a double frame and its title centred on the top edge
func (g *grid) panel(col, row, w, h int, title string) {
	g.fill(col, row, w, h, lightCyan, blue)
	g.put(col, row, "╔"+strings.Repeat("═", w-2)+"╗", lightCyan, blue)
	for r := row + 1; r < row+h-1; r++ {
		g.put(col, r, "║", lightCyan, blue)
		g.put(col+w-1, r, "║", lightCyan, blue)
	}
	g.put(col, row+h-1, "╚"+strings.Repeat("═", w-2)+"╝", lightCyan, blue)
	title = " " + title + " "
	g.put(col+(w-len([]rune(title)))/2, row, title, white, blue)
}

// Draws the grid with its top left corner at (x, y), in font at size: the
// backgrounds, as runs of one colour, and then the text, as runs of one
// colour. A cell is 0.6 by 1 of the size, the advance and the height of
// OEM12x20, and the baseline is 0.85 of the size below its top.
func (g *grid) drawOn(page *pdfjet.Page, font *pdfjet.Font, size, x, y float32) {
	cellW := size * 0.6
	for r := range g.rows {
		top := y + float32(r)*size
		for c := 0; c < g.cols; {
			end := c
			for end < g.cols && g.bg[r][end] == g.bg[r][c] {
				end++
			}
			if g.bg[r][c] != none {
				// A little taller, so that no hairline shows between rows
				rect := pdfjet.NewRect(x+float32(c)*cellW, top,
					float32(end-c)*cellW, size+0.25)
				rect.SetFillColor(g.bg[r][c])
				rect.DrawOn(page)
			}
			c = end
		}
		for c := 0; c < g.cols; {
			end := c
			for end < g.cols && g.fg[r][end] == g.fg[r][c] {
				end++
			}
			text := string(g.char[r][c:end])
			if strings.TrimSpace(text) != "" {
				line := pdfjet.NewTextLine(font, text)
				line.SetFontSize(size)
				line.SetTextColor(g.fg[r][c])
				line.SetLocation(x+float32(c)*cellW, top+size*0.85)
				line.DrawOn(page)
			}
			c = end
		}
	}
}

// Reads the bitmaps of the BDF font, 20 rows of 12 pixels each, by character
func readBDF(path string) map[rune][]uint16 {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	glyphs := make(map[rune][]uint16)
	var code int
	var rows []uint16
	inBitmap := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "ENCODING "):
			code, _ = strconv.Atoi(line[9:])
		case line == "BITMAP":
			inBitmap, rows = true, nil
		case line == "ENDCHAR":
			inBitmap = false
			glyphs[[]rune(cp437)[code]] = rows
		case inBitmap:
			bits, _ := strconv.ParseUint(line, 16, 16)
			rows = append(rows, uint16(bits))
		}
	}
	return glyphs
}

// The banner: the pixels of each letter, two rows of pixels to a cell, as
// full, upper half and lower half blocks, the blank rows left out, and each
// row of cells a colour of the gradient
func banner(text string, glyphs map[rune][]uint16, gradient []int32) *grid {
	const top, bottom = 2, 18 // the rows of pixels that any letter uses
	letters := []rune(text)
	g := newGrid(12*len(letters), (bottom-top)/2, ink, none)
	for i, ch := range letters {
		bitmap := glyphs[ch]
		for r := 0; r < g.rows; r++ {
			upper, lower := bitmap[top+2*r], bitmap[top+2*r+1]
			for c := range 12 {
				bit := uint16(0x8000) >> c
				cell := " "
				switch {
				case upper&bit != 0 && lower&bit != 0:
					cell = "█"
				case upper&bit != 0:
					cell = "▀"
				case lower&bit != 0:
					cell = "▄"
				}
				g.put(12*i+c, r, cell, gradient[r%len(gradient)], none)
			}
		}
	}
	return g
}

// The screen: a menu bar, two panels, the files of this folder and the 256
// characters of code page 437, a command line and the function keys
func screen() *grid {
	s := newGrid(80, 25, lightGray, black)

	s.fill(0, 0, 80, 1, black, cyan)
	s.put(2, 0, "Left     Files     Commands     Options     Right", black, cyan)
	s.put(74, 0, "12:20", black, cyan)

	s.panel(0, 1, 40, 22, `C:\FONTS\OEM12X20`)
	s.put(1, 2, "    Name    │  Size   │  Date   │Time ", yellow, blue)
	files := [][4]string{
		{"..", "►UP--DIR◄", "29-09-26", "12:20"},
		{"BUILD-OT PY", "6,861", "29-09-26", "14:44"},
		{"EXAMPLE  GO", "13,995", "29-09-26", "15:10"},
		{"EXAMPLE  PDF", "78,226", "29-09-26", "15:10"},
		{"LICENSE", "1,072", "29-09-26", "13:55"},
		{"OEM12X20 BDF", "48,165", "29-09-26", "13:55"},
		{"OEM12X20 OTF", "16,780", "29-09-26", "14:44"},
		{"OEM12X20 PNG", "2,657", "29-09-26", "13:55"},
		{"OEM12X20 STR", "6,226", "29-09-26", "14:46"},
	}
	for i, f := range files {
		line := fmt.Sprintf("%-12s│%9s│%9s│%5s", f[0], f[1], f[2], f[3])
		fg, bg := int32(lightCyan), int32(blue)
		if f[0] == "OEM12X20 OTF" {
			fg, bg = black, cyan // the cursor
		}
		s.put(1, 3+i, line, fg, bg)
	}
	for r := 3 + len(files); r < 20; r++ {
		s.put(1, r, "            │         │         │     ", lightCyan, blue)
	}
	s.put(0, 20, "╟"+strings.Repeat("─", 38)+"╢", lightCyan, blue)
	s.put(1, 21, "OEM12X20 OTF     16,780  29-09-26 14:44", lightCyan, blue)

	s.panel(40, 1, 40, 22, "Code page 437")
	s.put(44, 3, "  0 1 2 3 4 5 6 7 8 9 A B C D E F", yellow, blue)
	chars := []rune(cp437)
	for r := range 16 {
		s.put(44, 4+r, fmt.Sprintf("%X_", r), yellow, blue)
		for c := range 16 {
			s.put(47+2*c, 4+r, string(chars[16*r+c]), white, blue)
		}
	}
	s.put(44, 21, "12 by 20 pixels · 256 characters", lightCyan, blue)

	s.put(0, 23, `C:\FONTS\OEM12X20>pdfjet example.pdf`, lightGray, black)
	s.put(36, 23, "▄", lightGray, black) // the cursor
	keys := []string{"Help", "Menu", "View", "Edit", "Copy", "RenMov", "Mkdir",
		"Delete", "PullDn", "Quit"}
	col := 0
	for i, k := range keys {
		n := strconv.Itoa(i + 1)
		s.put(col, 24, n, lightGray, black)
		col += len(n)
		s.put(col, 24, fmt.Sprintf("%-6s", k), black, cyan)
		col += 6 + 1
	}
	return s
}

// The sunset: a sky that fades from night to red in dithered bands, a sun on
// the horizon and its reflection in the waves, a sailboat, birds and an
// island with a palm tree
func sunset() *grid {
	const cols, rows, horizon = 80, 24, 12
	g := newGrid(cols, rows, white, black)

	// The sky, in bands, each ending in a row of shade characters in the
	// colour of the next band
	bands := []struct {
		from, to int
		color    int32
	}{
		{0, 3, black}, {3, 6, blue}, {6, 9, magenta}, {9, 11, red}, {11, 12, lightRed},
	}
	for i, b := range bands {
		g.fill(0, b.from, cols, b.to-b.from, white, b.color)
		if i+1 < len(bands) {
			g.put(0, b.to-1, strings.Repeat("░", cols), bands[i+1].color, b.color)
		}
	}

	// The stars, in the night at the top
	stars := []struct {
		col, row int
		star     string
		color    int32
	}{
		{4, 0, "·", white}, {11, 1, "*", yellow}, {19, 0, "∙", lightGray},
		{27, 1, "·", white}, {33, 0, "☼", yellow}, {45, 1, "·", lightGray},
		{52, 0, "*", white}, {60, 1, "∙", yellow}, {68, 0, "·", white},
		{75, 1, "*", lightGray}, {8, 3, "·", lightGray}, {39, 3, "∙", white},
		{71, 3, "·", lightGray}, {23, 4, "·", white}, {57, 4, "∙", lightGray},
	}
	for _, s := range stars {
		g.put(s.col, s.row, s.star, s.color, none)
	}

	// The sun, its top rounded with half blocks, half set behind the sea
	sun := []string{
		"  ▄▄████▄▄  ",
		" ██████████ ",
		"████████████",
		"████████████",
	}
	for i, row := range sun {
		for j, ch := range []rune(row) {
			if ch != ' ' {
				g.put(34+j, horizon-len(sun)+i, string(ch), yellow, none)
			}
		}
	}

	// The sea, with waves, and the sun's reflection narrowing towards us
	g.fill(0, horizon, cols, rows-horizon, lightCyan, blue)
	for r := horizon; r < rows; r++ {
		for c := (r * 7) % 9; c < cols; c += 9 + r%4 {
			wave := "~"
			if (c+r)%3 == 0 {
				wave = "≈"
			}
			color := int32(cyan)
			if r%2 == 0 {
				color = lightCyan
			}
			g.put(c, r, wave, color, none)
		}
	}
	for i, w := range []int{14, 12, 11, 9, 8, 6, 5, 4, 3, 2} {
		r := horizon + i
		line := strings.Repeat("▬", w)
		if i%2 == 1 {
			line = strings.Repeat("─", w)
		}
		color := int32(yellow)
		if i > 4 {
			color = lightRed
		}
		g.put(40-w/2, r, line, color, none)
	}

	// The sailboat: a white sail, a mast and a brown hull on the water
	sail := []string{"▌", "█▌", "██▌", "███▌", "████▌", "█████▌", "██████▌"}
	for i, row := range sail {
		g.put(14, 7+i, "│", lightGray, none)
		g.put(15, 7+i, row, white, none)
	}
	g.put(14, 6, "►", lightRed, none) // the flag
	g.put(9, 14, "▀██████████████▀", brown, none)
	g.put(11, 15, "▀▀▀▀▀▀▀▀▀▀▀▀", darkGray, none)

	// The birds
	for _, b := range [][2]int{{52, 5}, {55, 6}, {58, 5}, {24, 7}, {27, 8}} {
		g.put(b[0], b[1], "v", black, none)
	}

	// The island, black against the sunset, with a palm tree
	g.put(66, 5, "♣♣♣", black, none)
	g.put(65, 6, "♣", black, none)
	g.put(69, 6, "♣", black, none)
	for r := 6; r < 10; r++ {
		g.put(67+(r-6)/2, r, "│", black, none)
	}
	g.put(63, 10, "▄▄████▄▄", black, none)
	g.put(59, 11, "▄▄████████████████▄▄", black, none)

	// A message in a bottle, and the signature
	g.put(70, 18, "◘", lightGreen, none)
	g.put(62, 22, "PDFjet ♥ 437", white, none)
	return g
}

func main() {
	pdf, err := pdfjet.NewPDFFile("Example.pdf")
	if err != nil {
		log.Fatal(err)
	}
	pdf.SetCompliance(compliance.PDF_A_3A_UA_1)
	pdf.SetTitle("OEM12x20, a font of code page 437")
	pdf.SetLanguage("en-US")

	font := pdfjet.NewFontFromFile(pdf, "OEM12x20.otf.stream")
	page := pdfjet.NewPage(pdf, a4.Portrait())
	pageWidth := page.GetWidth()

	// Each part is 80 cells wide at a size of 10: 480 points
	const size = 10
	x := (pageWidth - 80*size*0.6) / 2

	// The banner, 96 cells wide at a size of 8.3: also 478 points
	title := banner("OEM12x20", readBDF("OEM12x20.BDF"),
		[]int32{blue, blue, lightBlue, cyan, cyan, lightCyan, lightBlue, blue})
	title.drawOn(page, font, 8.3, (pageWidth-96*8.3*0.6)/2, 40)

	sub := pdfjet.NewTextLine(font,
		"A 12 by 20 pixel font of code page 437, as OpenType")
	sub.SetTextColor(ink)
	sub.SetFontSize(12)
	sub.SetLocation((pageWidth-sub.GetWidth())/2, 130)
	sub.DrawOn(page)

	y := float32(160)
	for _, part := range []*grid{screen(), sunset()} {
		w, h := float32(part.cols)*size*0.6, float32(part.rows)*size
		monitor := pdfjet.NewRect(x-12, y-12, w+24, h+24)
		monitor.SetFillColor(bezel)
		monitor.SetCornerRadius(10)
		monitor.DrawOn(page)
		part.drawOn(page, font, size, x, y)
		y += h + 44
	}

	// A progress bar, in the spirit of an installer of the time
	progress := newGrid(80, 1, ink, none)
	progress.put(0, 0, "Embedding OEM12X20.OTF.STREAM", ink, none)
	progress.put(32, 0, "["+strings.Repeat("█", 30)+strings.Repeat("░", 10)+"]", blue, none)
	progress.put(76, 0, "75%", ink, none)
	progress.drawOn(page, font, size, x, y-14)

	caption := pdfjet.NewTextLine(font,
		"MIT License · Copyright (c) 2023-2026 PDFjet Software · github.com/edragoev1/OEM12x20")
	caption.SetTextColor(ink)
	caption.SetFontSize(8)
	caption.SetLocation((pageWidth-caption.GetWidth())/2, page.GetHeight()-36)
	caption.DrawOn(page)

	if err := pdf.Complete(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Example.pdf")
}
