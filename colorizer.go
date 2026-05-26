package main

import (
	"os"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

type MarkdownColorizer struct {
	renderer *Renderer

	bold   bool
	italic bool
	code   bool

	lineStart bool
	header    int
	quote     bool

	buf string // unused in new lookahead parser but kept for struct compatibility

	// Code block parsing
	inCodeBlock    bool
	readingLang    bool
	langBuf        strings.Builder
	codeBlockTicks int

	// Link parsing
	linkState int 
	linkText  strings.Builder
	linkURL   strings.Builder

	// List parsing
	inListItem bool
	bulletBuf  strings.Builder
}

func NewMarkdownColorizer(r *Renderer) *MarkdownColorizer {
	return &MarkdownColorizer{
		renderer:  r,
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

var orderedListRegex = regexp.MustCompile(`^\d+\.\s`)

type TokenType int

const (
	TokenText TokenType = iota
	TokenBoldStart
	TokenBoldEnd
	TokenItalicStart
	TokenItalicEnd
	TokenCodeStart
	TokenCodeEnd
	TokenLink
)

type Token struct {
	Type    TokenType
	Text    string
	LinkURL string
}

func parseInline(s string) []Token {
	var tokens []Token
	runes := []rune(s)
	n := len(runes)

	boldActive := false
	italicActive := false
	codeActive := false

	findNextBold := func(start int) int {
		j := start
		for j <= n-2 {
			if runes[j] == '*' && runes[j+1] == '*' {
				return j
			}
			j++
		}
		return -1
	}

	findNextItalic := func(start int) int {
		j := start
		for j < n {
			if j+1 < n && runes[j] == '*' && runes[j+1] == '*' {
				j += 2
				continue
			}
			if runes[j] == '*' {
				return j
			}
			j++
		}
		return -1
	}

	findNextCode := func(start int) int {
		j := start
		for j < n {
			if runes[j] == '`' {
				return j
			}
			j++
		}
		return -1
	}

	parseLink := func(start int) (string, string, int, bool) {
		if runes[start] != '[' {
			return "", "", -1, false
		}
		closeBracket := -1
		for j := start + 1; j < n; j++ {
			if runes[j] == ']' {
				closeBracket = j
				break
			}
		}
		if closeBracket == -1 || closeBracket+1 >= n || runes[closeBracket+1] != '(' {
			return "", "", -1, false
		}
		closeParen := -1
		for j := closeBracket + 2; j < n; j++ {
			if runes[j] == ')' {
				closeParen = j
				break
			}
		}
		if closeParen == -1 {
			return "", "", -1, false
		}
		text := string(runes[start+1 : closeBracket])
		url := string(runes[closeBracket+2 : closeParen])
		return text, url, closeParen, true
	}

	i := 0
	var textBuf strings.Builder

	flushText := func() {
		if textBuf.Len() > 0 {
			tokens = append(tokens, Token{Type: TokenText, Text: textBuf.String()})
			textBuf.Reset()
		}
	}

	for i < n {
		if runes[i] == '\\' && i+1 < n {
			textBuf.WriteRune(runes[i+1])
			i += 2
			continue
		}

		if runes[i] == '[' {
			if text, url, endIdx, ok := parseLink(i); ok {
				flushText()
				tokens = append(tokens, Token{
					Type:    TokenLink,
					Text:    text,
					LinkURL: url,
				})
				i = endIdx + 1
				continue
			}
		}

		if i+1 < n && runes[i] == '*' && runes[i+1] == '*' {
			if boldActive {
				flushText()
				tokens = append(tokens, Token{Type: TokenBoldEnd})
				boldActive = false
				i += 2
				continue
			} else {
				next := findNextBold(i+2)
				if next != -1 {
					flushText()
					tokens = append(tokens, Token{Type: TokenBoldStart})
					boldActive = true
					i += 2
					continue
				} else {
					textBuf.WriteString("**")
					i += 2
					continue
				}
			}
		}

		if runes[i] == '*' {
			if italicActive {
				flushText()
				tokens = append(tokens, Token{Type: TokenItalicEnd})
				italicActive = false
				i++
				continue
			} else {
				next := findNextItalic(i+1)
				if next != -1 {
					flushText()
					tokens = append(tokens, Token{Type: TokenItalicStart})
					italicActive = true
					i++
					continue
				}
			}
		}

		if runes[i] == '`' {
			if codeActive {
				flushText()
				tokens = append(tokens, Token{Type: TokenCodeEnd})
				codeActive = false
				i++
				continue
			} else {
				next := findNextCode(i+1)
				if next != -1 {
					flushText()
					tokens = append(tokens, Token{Type: TokenCodeStart})
					codeActive = true
					i++
					continue
				}
			}
		}

		textBuf.WriteRune(runes[i])
		i++
	}

	flushText()
	return tokens
}

func (m *MarkdownColorizer) Print(chunk string) {
	lines := strings.Split(chunk, "\n")
	for i, line := range lines {
		handledBreak := m.processLine(line)
		if i < len(lines)-1 && !handledBreak {
			m.renderer.ForceBreak()
		}
	}
}

func (m *MarkdownColorizer) processLine(line string) bool {
	if m.inCodeBlock {
		if strings.HasPrefix(line, "```") {
			m.inCodeBlock = false
			m.renderer.SetDisableWrap(false)
			m.drawCodeBlockFooter()
			return true
		} else {
			m.renderer.WriteString(line)
			return false
		}
	}

	if strings.HasPrefix(line, "```") {
		m.inCodeBlock = true
		m.renderer.SetDisableWrap(true)
		lang := strings.TrimPrefix(line, "```")
		m.drawCodeBlockHeader(lang)
		return true
	}

	if line == "---" {
		m.drawHorizontalRule()
		return true
	}

	if strings.HasPrefix(line, "#") {
		runes := []rune(line)
		hCount := 0
		for hCount < len(runes) && runes[hCount] == '#' {
			hCount++
		}
		if hCount > 0 && hCount < len(runes) && runes[hCount] == ' ' {
			m.header = hCount
			if m.header > 4 {
				m.header = 4
			}
			m.applyHeaderColor()
			m.renderer.WriteString(string(runes[:hCount+1]))
			m.processInlineText(string(runes[hCount+1:]))
			m.renderer.WriteANSI(ColorReset)
			m.header = 0
			return false
		}
	}

	if strings.HasPrefix(line, "> ") {
		m.quote = true
		m.renderer.WriteANSI(ColorLightGray)
		m.renderer.WriteString("> ")
		m.processInlineText(line[2:])
		m.renderer.WriteANSI(ColorReset)
		m.quote = false
		return false
	} else if line == ">" {
		m.quote = true
		m.renderer.WriteANSI(ColorLightGray)
		m.renderer.WriteString(">")
		m.renderer.WriteANSI(ColorReset)
		m.quote = false
		return false
	}

	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		m.renderer.WriteANSI(ColorPurple)
		m.renderer.WriteString("• ")
		m.renderer.WriteANSI(ColorReset)
		m.renderer.PushIndent(2)
		m.processInlineText(line[2:])
		m.renderer.PopIndent()
		return false
	}

	if match := orderedListRegex.FindString(line); match != "" {
		m.renderer.WriteANSI(ColorPurple)
		m.renderer.WriteString(match)
		m.renderer.WriteANSI(ColorReset)
		indentWidth := runewidth.StringWidth(match)
		m.renderer.PushIndent(indentWidth)
		m.processInlineText(line[len(match):])
		m.renderer.PopIndent()
		return false
	}

	m.processInlineText(line)
	return false
}

func (m *MarkdownColorizer) processInlineText(text string) {
	tokens := parseInline(text)
	for _, tok := range tokens {
		switch tok.Type {
		case TokenText:
			m.renderer.WriteString(tok.Text)
		case TokenBoldStart:
			m.bold = true
			m.renderer.WriteANSI(ColorBlue)
		case TokenBoldEnd:
			m.bold = false
			m.resetInline()
		case TokenItalicStart:
			m.italic = true
			m.renderer.WriteANSI(ColorLightCyan)
		case TokenItalicEnd:
			m.italic = false
			m.resetInline()
		case TokenCodeStart:
			m.code = true
			m.renderer.WriteANSI(ColorPurple)
		case TokenCodeEnd:
			m.code = false
			m.resetInline()
		case TokenLink:
			m.renderLink(tok.Text, tok.LinkURL)
		}
	}
}

func (m *MarkdownColorizer) renderLink(text, url string) {
	if supportsOSC8() {
		m.renderer.WriteANSI("\033]8;;" + url + "\033\\")
		m.renderer.WriteANSI(ColorLightCyan)
		m.renderer.WriteString(text)
		m.renderer.WriteANSI("\033[0m")
		m.renderer.WriteANSI("\033]8;;\033\\")
	} else {
		m.renderer.WriteANSI(ColorLightCyan)
		m.renderer.WriteString(text)
		m.renderer.WriteANSI("\033[0m")
		m.renderer.WriteString(" (")
		m.renderer.WriteANSI("\033[90m")
		m.renderer.WriteString(url)
		m.renderer.WriteANSI("\033[0m")
		m.renderer.WriteString(")")
	}
}

func (m *MarkdownColorizer) applyHeaderColor() {
	switch m.header {
	case 1:
		m.renderer.WriteANSI(ColorRed)
	case 2:
		m.renderer.WriteANSI(ColorDarkOrange)
	case 3:
		m.renderer.WriteANSI(ColorOrange)
	case 4:
		m.renderer.WriteANSI(ColorDarkYellow)
	}
}

func (m *MarkdownColorizer) toggleBold() {
	m.bold = !m.bold
	if m.bold {
		m.renderer.WriteANSI(ColorBlue)
	} else {
		m.resetInline()
	}
}

func (m *MarkdownColorizer) toggleItalic() {
	m.italic = !m.italic
	if m.italic {
		m.renderer.WriteANSI(ColorLightCyan)
	} else {
		m.resetInline()
	}
}

func (m *MarkdownColorizer) toggleCode() {
	m.code = !m.code
	if m.code {
		m.renderer.WriteANSI(ColorPurple)
	} else {
		m.resetInline()
	}
}

func (m *MarkdownColorizer) resetInline() {
	m.renderer.WriteANSI(ColorReset)
	if m.header > 0 {
		m.applyHeaderColor()
	} else if m.quote {
		m.renderer.WriteANSI(ColorLightGray)
	}

	if m.bold {
		m.renderer.WriteANSI(ColorBlue)
	}
	if m.italic {
		m.renderer.WriteANSI(ColorLightCyan)
	}
	if m.code {
		m.renderer.WriteANSI(ColorPurple)
	}
}

func (m *MarkdownColorizer) Flush() {
	m.renderer.Flush()
}

func (m *MarkdownColorizer) drawCodeBlockHeader(lang string) {
	if lang == "" {
		lang = "Code"
	}
	lang = strings.Title(strings.TrimSpace(lang))

	m.renderer.WriteANSI("\033[1;36m")
	m.renderer.WriteString("[" + lang + "]")
	m.renderer.WriteANSI("\033[0m")
	m.renderer.ForceBreak()

	m.renderer.PushIndent(4)
	m.renderer.WriteANSI(ColorPurple)
}

func (m *MarkdownColorizer) drawCodeBlockFooter() {
	m.renderer.WriteANSI("\033[0m")
	m.renderer.PopIndent()
	m.renderer.ForceBreak()
}

func (m *MarkdownColorizer) drawHorizontalRule() {
	width := m.renderer.GetContentWidth()
	rule := strings.Repeat("─", width)

	m.renderer.WriteANSI("\033[90m")
	m.renderer.WriteString(rule)
	m.renderer.WriteANSI("\033[0m")
	m.renderer.ForceBreak()
}

func supportsOSC8() bool {
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	termProg := os.Getenv("TERM_PROGRAM")
	if termProg == "vscode" || termProg == "iTerm.app" || termProg == "Apple_Terminal" {
		return true
	}
	return false
}
