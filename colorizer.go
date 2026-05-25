package main

import (
	"fmt"
)

type MarkdownColorizer struct {
	bold   bool
	italic bool
	code   bool

	lineStart bool
	header    int
	quote     bool

	buf string // short buffer for ambiguous prefixes
}

func NewMarkdownColorizer() *MarkdownColorizer {
	return &MarkdownColorizer{
		lineStart: true,
	}
}

const (
	ColorReset      = "\033[0m"
	ColorBlue       = "\033[34m"
	ColorLightCyan  = "\033[96m"
	ColorPurple     = "\033[35m"
	ColorRed        = "\033[31m"
	ColorDarkOrange = "\033[38;5;208m"
	ColorOrange     = "\033[38;5;214m"
	ColorDarkYellow = "\033[33m"
	ColorLightGray  = "\033[38;5;244m"
)

func (m *MarkdownColorizer) Print(chunk string) {
	for _, ch := range chunk {
		m.processChar(ch)
	}
}

func (m *MarkdownColorizer) processChar(ch rune) {
	strCh := string(ch)

	// If we are accumulating a potential token at the start of a line
	if m.lineStart {
		if ch == '#' {
			m.buf += strCh
			return
		}
		if ch == '>' {
			m.buf += strCh
			return
		}
		if ch == ' ' {
			if len(m.buf) > 0 {
				if m.buf[0] == '#' {
					m.header = len(m.buf)
					if m.header > 4 {
						m.header = 4
					}
					m.applyHeaderColor()
					fmt.Print(m.buf + " ")
					m.buf = ""
					m.lineStart = false
					return
				} else if m.buf[0] == '>' {
					m.quote = true
					fmt.Print(ColorLightGray + m.buf + " ")
					m.buf = ""
					m.lineStart = false
					return
				}
			}
		}
		// If it's anything else and we had something in the buffer, it wasn't a valid header/quote (or missing space)
		if len(m.buf) > 0 {
			if m.buf[0] == '#' || m.buf[0] == '>' {
				fmt.Print(m.buf)
				m.buf = ""
			}
		}
		if ch != ' ' && ch != '\n' && ch != '\r' {
			m.lineStart = false
		}
	}

	if ch == '\n' {
		// Reset line state
		if m.header > 0 || m.quote {
			fmt.Print(ColorReset)
		}
		m.header = 0
		m.quote = false
		m.lineStart = true
		fmt.Print("\n")
		// Re-apply inline styles if they are active across lines
		if m.bold { fmt.Print(ColorBlue) }
		if m.italic { fmt.Print(ColorLightCyan) }
		if m.code { fmt.Print(ColorPurple) }
		return
	}

	// Ambiguous `*`
	if ch == '*' {
		m.buf += strCh
		return
	}
	if len(m.buf) > 0 && m.buf[0] == '*' {
		if len(m.buf) == 1 {
			m.toggleItalic()
		} else if len(m.buf) == 2 {
			m.toggleBold()
		} else {
			fmt.Print(m.buf)
		}
		m.buf = ""
	}

	if ch == '`' {
		m.toggleCode()
		return
	}

	fmt.Print(strCh)
}

func (m *MarkdownColorizer) applyHeaderColor() {
	switch m.header {
	case 1:
		fmt.Print(ColorRed)
	case 2:
		fmt.Print(ColorDarkOrange)
	case 3:
		fmt.Print(ColorOrange)
	case 4:
		fmt.Print(ColorDarkYellow)
	}
}

func (m *MarkdownColorizer) toggleBold() {
	m.bold = !m.bold
	if m.bold {
		fmt.Print(ColorBlue)
	} else {
		m.resetInline()
	}
}

func (m *MarkdownColorizer) toggleItalic() {
	m.italic = !m.italic
	if m.italic {
		fmt.Print(ColorLightCyan)
	} else {
		m.resetInline()
	}
}

func (m *MarkdownColorizer) toggleCode() {
	m.code = !m.code
	if m.code {
		fmt.Print(ColorPurple)
	} else {
		m.resetInline()
	}
}

func (m *MarkdownColorizer) resetInline() {
	fmt.Print(ColorReset)
	// re-apply
	if m.header > 0 {
		m.applyHeaderColor()
	} else if m.quote {
		fmt.Print(ColorLightGray)
	}
	
	if m.bold {
		fmt.Print(ColorBlue)
	}
	if m.italic {
		fmt.Print(ColorLightCyan)
	}
	if m.code {
		fmt.Print(ColorPurple)
	}
}

func (m *MarkdownColorizer) Flush() {
	if len(m.buf) > 0 {
		if m.buf[0] == '*' {
			if len(m.buf) == 1 {
				m.toggleItalic()
			} else if len(m.buf) == 2 {
				m.toggleBold()
			}
		} else {
			fmt.Print(m.buf)
		}
		m.buf = ""
	}
}
