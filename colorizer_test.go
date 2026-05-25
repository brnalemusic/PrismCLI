package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestMarkdownColorizer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // We will check if it contains the correct color codes
	}{
		{
			name:     "Bold",
			input:    "**Hello**",
			expected: ColorBlue + "Hello" + ColorReset,
		},
		{
			name:     "Italic",
			input:    "*Hello*",
			expected: ColorLightCyan + "Hello" + ColorReset,
		},
		{
			name:     "Code",
			input:    "`Hello`",
			expected: ColorPurple + "Hello" + ColorReset,
		},
		{
			name:     "Header 1",
			input:    "# Hello\n",
			expected: ColorRed + "# " + "Hello" + ColorReset + "\n",
		},
		{
			name:     "Quote",
			input:    "> Hello\n",
			expected: ColorLightGray + "> " + "Hello" + ColorReset + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			m := NewMarkdownColorizer()
			m.Print(tc.input)
			m.Flush()

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			io.Copy(&buf, r)
			result := buf.String()

			if !strings.Contains(result, tc.expected) {
				t.Errorf("Expected to contain %q, but got %q", tc.expected, result)
			}
		})
	}
}
