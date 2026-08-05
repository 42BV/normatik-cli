package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// menuItem is one selectable row: a short title plus a hint describing what
// choosing it does (shown dimmed under the title).
type menuItem struct {
	title string
	hint  string
}

// menuKey is a decoded keypress relevant to the arrow-selector.
type menuKey int

const (
	keyOther menuKey = iota
	keyUp
	keyDown
	keyEnter
	keyDigit
	keyCancel
)

// errMenuCancelled is returned by runInteractiveMenu when the user aborts the
// selector (Esc, q or Ctrl-C). Callers treat it like the URL-prompt EOF: abort
// the login rather than silently starting a side-effecting flow.
var errMenuCancelled = errors.New("menu selection cancelled")

// renderMenuLines builds the styled menu as individual terminal lines (no line
// terminator). The active row gets an orange pointer + bold title; the others
// align under it. Pure and deterministic so it can be unit-tested.
func renderMenuLines(prompt string, items []menuItem, cursor int) []string {
	lines := []string{styleStrong.Render(prompt), ""}
	for i, it := range items {
		if i == cursor {
			lines = append(lines, "  "+styleAccent.Render("❯ "+it.title))
		} else {
			lines = append(lines, "    "+it.title)
		}
		if it.hint != "" {
			lines = append(lines, "       "+styleMuted.Render(it.hint))
		}
	}
	lines = append(lines, "", "  "+styleMuted.Render("↑/↓ move · enter select · esc cancel"))
	return lines
}

// decodeMenuKey reads one keypress from br and maps it to a menuKey. It handles
// arrow-key escape sequences (ESC [ A/B and the ESC O variant), vim-style j/k,
// Ctrl-P/Ctrl-N, digit quick-select, and Enter/Esc/q/Ctrl-C. A lone Esc (no
// buffered follow-up bytes) counts as cancel. Testable with a bytes reader.
func decodeMenuKey(br *bufio.Reader) (menuKey, int, error) {
	b, err := br.ReadByte()
	if err != nil {
		return keyOther, 0, err
	}
	switch b {
	case '\r', '\n':
		return keyEnter, 0, nil
	case 0x03, 'q', 'Q': // Ctrl-C / q
		return keyCancel, 0, nil
	case 'k', 'K', 0x10: // up / Ctrl-P
		return keyUp, 0, nil
	case 'j', 'J', 0x0e: // down / Ctrl-N
		return keyDown, 0, nil
	case 0x1b: // Esc, alone or as the start of an arrow escape sequence
		if br.Buffered() == 0 {
			return keyCancel, 0, nil
		}
		b2, err := br.ReadByte()
		if err != nil || (b2 != '[' && b2 != 'O') {
			return keyOther, 0, nil
		}
		b3, err := br.ReadByte()
		if err != nil {
			return keyOther, 0, nil
		}
		switch b3 {
		case 'A':
			return keyUp, 0, nil
		case 'B':
			return keyDown, 0, nil
		default:
			return keyOther, 0, nil
		}
	default:
		if b >= '1' && b <= '9' {
			return keyDigit, int(b - '0'), nil
		}
		return keyOther, 0, nil
	}
}

// runInteractiveMenu draws the selector to out, then loops on key events from in
// until the user confirms (Enter, or a digit that maps to a row) or cancels
// (Esc/q/Ctrl-C). It returns the chosen index, or errMenuCancelled / the read
// error. The terminal must already be in raw mode (the caller owns that); in and
// out are parameters so the whole loop is testable with a scripted byte reader
// and a buffer. Lines are terminated with CRLF because raw mode does not
// translate \n to a carriage return.
func runInteractiveMenu(in io.Reader, out io.Writer, prompt string, items []menuItem, start int) (int, error) {
	if len(items) == 0 {
		return -1, errMenuCancelled
	}
	br := bufio.NewReader(in)
	cursor := start
	if cursor < 0 || cursor >= len(items) {
		cursor = 0
	}

	rows := 0
	draw := func() {
		lines := renderMenuLines(prompt, items, cursor)
		fmt.Fprint(out, strings.Join(lines, "\r\n")+"\r\n")
		rows = len(lines)
	}
	// Move back to the top of the previously drawn block and clear to end of
	// screen, so a redraw overwrites in place instead of scrolling.
	rewind := func() { fmt.Fprintf(out, "\x1b[%dA\r\x1b[0J", rows) }

	fmt.Fprint(out, "\x1b[?25l") // hide cursor while navigating
	showCursor := func() { fmt.Fprint(out, "\x1b[?25h") }
	draw()

	// collapse replaces the multi-line menu with a single confirmation line.
	collapse := func() {
		rewind()
		showCursor()
		fmt.Fprint(out, "  "+styleAccent.Render("▸ "+items[cursor].title)+"\r\n")
	}

	for {
		key, digit, err := decodeMenuKey(br)
		if err != nil {
			showCursor()
			return cursor, err
		}
		switch key {
		case keyUp:
			cursor = (cursor - 1 + len(items)) % len(items)
			rewind()
			draw()
		case keyDown:
			cursor = (cursor + 1) % len(items)
			rewind()
			draw()
		case keyDigit:
			if digit >= 1 && digit <= len(items) {
				cursor = digit - 1
				collapse()
				return cursor, nil
			}
		case keyEnter:
			collapse()
			return cursor, nil
		case keyCancel:
			rewind()
			showCursor()
			return -1, errMenuCancelled
		}
	}
}
