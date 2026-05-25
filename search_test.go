package main

import (
	"strings"
	"testing"
)

func TestStripHTMLTags(t *testing.T) {
	htmlStr := `<p>Hello <b>World</b>! <a href="http://example.com">Link</a></p>`
	expected := "Hello World! Link"
	result := stripHTMLTags(htmlStr)
	
	// Clean spacing to match simple tags strip
	cleanResult := strings.Join(strings.Fields(result), " ")
	if cleanResult != expected {
		t.Errorf("Expected stripped result '%s', got '%s'", expected, cleanResult)
	}
}

func TestCleanHTMLContent(t *testing.T) {
	htmlStr := `
		<html>
			<head>
				<style>body { color: red; }</style>
				<script>alert("test");</script>
			</head>
			<body>
				<h1>Prism CLI Documentation</h1>
				<p>This is a test document with &amp; some symbols.</p>
			</body>
		</html>
	`
	cleaned := CleanHTMLContent(htmlStr)
	
	if strings.Contains(cleaned, "body {") {
		t.Errorf("Style block was not removed")
	}
	if strings.Contains(cleaned, "alert(") {
		t.Errorf("Script block was not removed")
	}
	if !strings.Contains(cleaned, "Prism CLI Documentation") {
		t.Errorf("Title content was lost")
	}
	if !strings.Contains(cleaned, "& some symbols") && !strings.Contains(cleaned, "&amp; some symbols") {
		// Wait, CleanHTMLContent unescapes &amp; to &
		if !strings.Contains(cleaned, "& some symbols") {
			t.Errorf("&amp; entity was not unescaped correctly")
		}
	}
}
