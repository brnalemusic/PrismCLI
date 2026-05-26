package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

type ansiState int

const (
	stateNormal ansiState = iota
	stateESC
	stateCSI
	stateOSC
	stateOSCESC
)

// Renderer handles terminal word wrapping, margins, indentation, and ANSI styling.
type Renderer struct {
	termWidth    int
	maxWidth     int
	contentWidth int
	leftPad      string
	col          int // Current visual column on the current line

	wordBuf   bytes.Buffer // Buffers characters for the current word
	wordWidth int          // Visual width of the word in wordBuf

	lineStart   bool   // Is the next character the start of a line?
	ansiActive  string // Tracks active ANSI style codes to re-apply on wrap
	disableWrap bool   // Temporarily disable word wrapping (e.g. for code blocks)
	linePrefix  string // Custom prefix printed right after leftPad (e.g. "│ ")
	linePrefixWidth int // Visual width of linePrefix

	// ANSI parser state
	ansiMode ansiState
	ansiBuf  bytes.Buffer

	indentStack []int
	indentStr   string

	writer *bufio.Writer
}

// NewRenderer creates a new Renderer instance detecting current terminal width.
func NewRenderer() *Renderer {
	return NewRendererWriter(os.Stdout)
}

// NewRendererWriter creates a new Renderer instance writing to a custom io.Writer.
func NewRendererWriter(w io.Writer) *Renderer {
	r := &Renderer{
		maxWidth:    100,
		lineStart:   true,
		writer:      bufio.NewWriter(w),
		indentStack: []int{},
	}
	r.RefreshWidth()
	return r
}

// SetLinePrefix sets a custom line prefix and its visual character width.
func (r *Renderer) SetLinePrefix(prefix string, visualWidth int) {
	r.linePrefix = prefix
	r.linePrefixWidth = visualWidth
}


// RefreshWidth re-detects the terminal width and updates padding/content width.
func (r *Renderer) RefreshWidth() {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		if r.contentWidth > 0 {
			return
		}
		width = 80 // Default fallback
	}
	r.termWidth = width
	r.contentWidth = r.termWidth
	r.leftPad = ""
}

// GetContentWidth returns the current maximum content block width.
func (r *Renderer) GetContentWidth() int {
	return r.contentWidth
}

// GetTerminalWidth returns the raw terminal width columns.
func (r *Renderer) GetTerminalWidth() int {
	return r.termWidth
}

// SetDisableWrap enables or disables word wrapping.
func (r *Renderer) SetDisableWrap(disable bool) {
	r.disableWrap = disable
}

// PushIndent adds a left indentation level.
func (r *Renderer) PushIndent(n int) {
	r.indentStack = append(r.indentStack, n)
	r.updateIndentStr()
}

// PopIndent removes the last left indentation level.
func (r *Renderer) PopIndent() {
	if len(r.indentStack) > 0 {
		r.indentStack = r.indentStack[:len(r.indentStack)-1]
		r.updateIndentStr()
	}
}

func (r *Renderer) updateIndentStr() {
	total := 0
	for _, val := range r.indentStack {
		total += val
	}
	r.indentStr = strings.Repeat(" ", total)
}

// WriteRune processes a single rune through the wrapping and ANSI state machine.
func (r *Renderer) WriteRune(ch rune) {
	// Parse ANSI sequences if we are in an escape sequence or encounter \033
	if r.ansiMode != stateNormal {
		r.ansiBuf.WriteRune(ch)
		switch r.ansiMode {
		case stateESC:
			if ch == '[' {
				r.ansiMode = stateCSI
			} else if ch == ']' {
				r.ansiMode = stateOSC
			} else {
				// Unknown ESC sequence, write as raw ANSI
				r.writeRaw(r.ansiBuf.String())
				r.ansiBuf.Reset()
				r.ansiMode = stateNormal
			}
		case stateCSI:
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				code := r.ansiBuf.String()
				r.writeRaw(code)
				r.trackANSI(code)
				r.ansiBuf.Reset()
				r.ansiMode = stateNormal
			}
		case stateOSC:
			if ch == '\033' {
				r.ansiMode = stateOSCESC
			} else if ch == '\a' { // OSC terminated by Bell
				r.writeRaw(r.ansiBuf.String())
				r.ansiBuf.Reset()
				r.ansiMode = stateNormal
			}
		case stateOSCESC:
			if ch == '\\' { // ESC \ String Terminator
				r.writeRaw(r.ansiBuf.String())
				r.ansiBuf.Reset()
				r.ansiMode = stateNormal
			} else {
				r.ansiMode = stateOSC
			}
		}
		return
	}

	if ch == '\033' {
		r.flushWord()
		r.ansiBuf.WriteRune(ch)
		r.ansiMode = stateESC
		return
	}

	// Handle standard newlines
	if ch == '\n' {
		r.ForceBreak()
		return
	}
	if ch == '\r' {
		// Ignore carriage returns to avoid double newlines
		return
	}

	// If wrapping is disabled, write raw character directly
	if r.disableWrap {
		r.writeRaw(string(ch))
		return
	}

	// Visual width of the current rune
	w := runewidth.RuneWidth(ch)
	if w <= 0 {
		w = 1 // Fallback for safety
	}

	// If character is whitespace, it acts as a word boundary
	if unicode.IsSpace(ch) {
		r.flushWord()
		
		// Print space if it fits on the line
		effectiveLimit := r.contentWidth - len([]rune(r.indentStr)) - r.linePrefixWidth
		if r.col+w <= effectiveLimit {
			r.writeRaw(string(ch))
			r.col += w
		} else {
			// Wrap on space: just break line and ignore the leading space
			r.ForceBreak()
		}
		return
	}

	// Accumulate in word buffer
	r.wordBuf.WriteRune(ch)
	r.wordWidth += w

	// If a single word is wider than the terminal content limit itself, force break it
	effectiveLimit := r.contentWidth - len([]rune(r.indentStr)) - r.linePrefixWidth
	if r.wordWidth > effectiveLimit {
		r.flushWord()
		r.ForceBreak()
	}
}

// WriteString writes a string by splitting into runes.
func (r *Renderer) WriteString(s string) {
	for _, ch := range s {
		r.WriteRune(ch)
	}
}

// WriteANSI writes a styling ANSI code directly without wrapping checks.
func (r *Renderer) WriteANSI(code string) {
	r.flushWord()
	r.writeRaw(code)
	r.trackANSI(code)
}

// ForceBreak breaks the line, rendering margins and padding on the next line.
func (r *Renderer) ForceBreak() {
	r.RefreshWidth()
	r.flushWord()
	_, _ = r.writer.WriteString("\n")
	r.lineStart = true
	r.col = 0
}

// Flush flushes all remaining text in the word buffer and stdout writer.
func (r *Renderer) Flush() {
	r.flushWord()
	_ = r.writer.Flush()
}

// FlushWriter flushes only the underlying buffered writer to output.
func (r *Renderer) FlushWriter() {
	_ = r.writer.Flush()
}

// ResetColumn clears the current column tracker. Useful after direct prints.
func (r *Renderer) ResetColumn() {
	r.col = 0
	r.lineStart = true
}

func (r *Renderer) flushWord() {
	if r.wordBuf.Len() == 0 {
		return
	}

	wordStr := r.wordBuf.String()
	r.wordBuf.Reset()
	w := r.wordWidth
	r.wordWidth = 0

	r.RefreshWidth()
	effectiveLimit := r.contentWidth - len([]rune(r.indentStr)) - r.linePrefixWidth
	// If the word doesn't fit on this line, wrap to next line first
	if r.col+w > effectiveLimit && r.col > 0 {
		_, _ = r.writer.WriteString("\n")
		r.lineStart = true
		r.col = 0
	}

	r.writeRaw(wordStr)
	r.col += w
}

func (r *Renderer) writeRaw(str string) {
	if r.lineStart {
		// Output left margins
		_, _ = r.writer.WriteString(r.leftPad)
		if r.linePrefix != "" {
			_, _ = r.writer.WriteString(r.linePrefix)
		}
		_, _ = r.writer.WriteString(r.indentStr)
		// Re-apply styles
		if r.ansiActive != "" {
			_, _ = r.writer.WriteString(r.ansiActive)
		}
		r.lineStart = false
	}
	_, _ = r.writer.WriteString(str)
}

func (r *Renderer) trackANSI(code string) {
	if strings.HasPrefix(code, "\033[") && strings.HasSuffix(code, "m") {
		inner := code[2 : len(code)-1]
		isStyle := true
		for _, ch := range inner {
			if !unicode.IsDigit(ch) && ch != ';' {
				isStyle = false
				break
			}
		}
		if isStyle {
			if code == "\033[0m" {
				r.ansiActive = ""
			} else {
				r.ansiActive += code
			}
		}
	}
}
