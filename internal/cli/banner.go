package cli

import (
	"math"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

const compassSize = 36

const (
	colFill   = "#2e2834"
	colFill2  = "#26202c"
	colRingHi = "#f4d6b8"
	colRing   = "#d99a6c"
	colRingMd = "#c4845a"
	colRingLo = "#9e6646"
	colN      = "#e8734a"
	colN2     = "#c45230"
	colN3     = "#a84026"
	colS      = "#c48c74"
	colS2     = "#966858"
	colS3     = "#705046"
	colHub    = "#e8734a"
	colHub2   = "#ffc496"
	colOrange = "#D97A54"
	colMuted  = "#8A8290"
)

var ringPalette = []string{colRingHi, colRing, colRingMd, colRingLo}

const bannerTagline = "The Normatik CLI"

// banner renders the chunky 36×36 compass as truecolor half-blocks, with the
// tagline centred under the mark. Size is even so every pixel row pairs into a
// half-block cell. lipgloss strips colour when stdout is not a colour terminal.
func banner() string {
	rows := renderHalfBlocks(chunkyCompass(compassSize))
	var b strings.Builder
	b.WriteByte('\n')
	for _, row := range rows {
		b.WriteString("  ")
		b.WriteString(row)
		b.WriteByte('\n')
	}
	diamond := lipgloss.NewStyle().Foreground(lipgloss.Color(colOrange)).Bold(true).Render("◆")
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color(colMuted)).Bold(true).Render(bannerTagline)
	plain := "◆  " + bannerTagline
	pad := (compassSize - utf8.RuneCountInString(plain)) / 2
	if pad < 0 {
		pad = 0
	}
	b.WriteByte('\n')
	b.WriteString("  ")
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(diamond)
	b.WriteString("  ")
	b.WriteString(tag)
	b.WriteByte('\n')
	return b.String()
}

// shouldShowBanner: only on a real terminal, only for the root help / bare run,
// never when piped (agents/CI) or when --no-banner is passed.
func shouldShowBanner(args []string) bool {
	if !stdoutIsTTY() {
		return false
	}
	return bannerRequested(args)
}

func bannerRequested(args []string) bool {
	for _, a := range args {
		if a == "--no-banner" {
			return false
		}
	}
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "-h", "--help", "help":
		return true
	}
	return false
}

func renderHalfBlocks(grid [][]string) []string {
	h := len(grid)
	w := len(grid[0])
	even := h
	if even%2 != 0 {
		even++
	}
	rows := make([]string, 0, even/2)
	for y := 0; y < even; y += 2 {
		var line strings.Builder
		for x := 0; x < w; x++ {
			top := grid[y][x]
			bot := ""
			if y+1 < h {
				bot = grid[y+1][x]
			}
			line.WriteString(halfBlockCell(top, bot))
		}
		if s := line.String(); strings.TrimSpace(s) != "" {
			rows = append(rows, s)
		}
	}
	return rows
}

func halfBlockCell(top, bot string) string {
	switch {
	case top == "" && bot == "":
		return " "
	case top != "" && bot != "" && top == bot:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(top)).Render("█")
	case top != "" && bot != "":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(top)).Background(lipgloss.Color(bot)).Render("▀")
	case top != "":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(top)).Render("▀")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(bot)).Render("▄")
	}
}

func chunkyCompass(size int) [][]string {
	g := make([][]string, size)
	for y := range g {
		g[y] = make([]string, size)
	}
	cx := float64(size-1) / 2
	cy := cx
	rOut := float64(size) * 0.47
	thick := math.Max(1.15, float64(size)*0.075)
	rIn := rOut - thick
	rInner := float64(size) * 0.30
	hw := math.Max(1.6, float64(size)*0.175)
	hh := float64(size) * 0.355
	hubR := math.Max(0.85, float64(size)*0.065)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x), float64(y)
			d := math.Hypot(fx-cx, fy-cy)
			nx := (fx - cx) / math.Max(d, 0.001)
			ny := (fy - cy) / math.Max(d, 0.001)
			shade := 0.5 + 0.5*(-0.62*nx-0.55*ny)
			if d >= rIn-0.15 && d <= rOut+0.4 {
				g[y][x] = nearestHex(lerpHex(colRingLo, colRingHi, shade), ringPalette)
			} else if d < rIn {
				if d > rInner {
					g[y][x] = colFill
				} else {
					g[y][x] = colFill2
				}
			}
		}
	}

	setp := func(px, py float64, c string) {
		x := int(math.Round(px))
		y := int(math.Round(py))
		if y >= 0 && y < size && x >= 0 && x < size {
			g[y][x] = c
		}
	}
	for i := 0; i <= 3; i++ {
		setp(cx, cy-rOut+0.2+float64(i)*0.85, colN)
	}
	setp(cx-1, cy-rOut+0.4, colN)
	setp(cx+1, cy-rOut+0.4, colN)
	for _, dir := range [][2]float64{{1, 0}, {-1, 0}, {0, 1}} {
		setp(cx+dir[0]*(rOut-0.15), cy+dir[1]*(rOut-0.15), colRing)
	}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x), float64(y)
			dx := math.Abs(fx - cx)
			dy := math.Abs(fy - cy)
			v := dx/hw + dy/hh
			if v > 1.02 {
				continue
			}
			if math.Hypot(fx-cx, fy-cy) > rIn-0.15 {
				continue
			}
			left := fx < cx
			switch {
			case fy < cy-0.15:
				g[y][x] = pickNeedle(v > 0.82, left, colN, colN2, colN3)
			case fy > cy+0.15:
				g[y][x] = pickNeedle(v > 0.82, left, colS, colS2, colS3)
			}
		}
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)-cx, float64(y)-cy)
			if d <= hubR+0.25 {
				if d < hubR*0.45 {
					g[y][x] = colHub2
				} else {
					g[y][x] = colHub
				}
			}
		}
	}
	return g
}

func pickNeedle(edge, left bool, body, dark, darker string) string {
	if edge {
		if left {
			return dark
		}
		return darker
	}
	if left {
		return body
	}
	return dark
}

func parseHex(h string) (r, g, b int) {
	if len(h) != 7 || h[0] != '#' {
		return 0, 0, 0
	}
	n := 0
	for i := 1; i < 7; i++ {
		c := h[i]
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			return 0, 0, 0
		}
		n = n<<4 | int(v)
	}
	return n >> 16, (n >> 8) & 0xff, n & 0xff
}

func lerpHex(a, b string, t float64) string {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	r := int(math.Round(float64(ar) + (float64(br)-float64(ar))*t))
	g := int(math.Round(float64(ag) + (float64(bg)-float64(ag))*t))
	bl := int(math.Round(float64(ab) + (float64(bb)-float64(ab))*t))
	return hexRGB(r, g, bl)
}

func hexRGB(r, g, b int) string {
	const digits = "0123456789abcdef"
	out := [7]byte{'#'}
	out[1] = digits[r>>4]
	out[2] = digits[r&15]
	out[3] = digits[g>>4]
	out[4] = digits[g&15]
	out[5] = digits[b>>4]
	out[6] = digits[b&15]
	return string(out[:])
}

func nearestHex(c string, keys []string) string {
	cr, cg, cb := parseHex(c)
	best := keys[0]
	bestD := 1 << 30
	for _, k := range keys {
		kr, kg, kb := parseHex(k)
		dr, dg, db := cr-kr, cg-kg, cb-kb
		d := dr*dr + dg*dg + db*db
		if d < bestD {
			bestD = d
			best = k
		}
	}
	return best
}
