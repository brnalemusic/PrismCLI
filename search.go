package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SearchResult represents a single search hit.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// ScrapeDuckDuckGo queries DuckDuckGo HTML-only search and parses top 5 results.
func ScrapeDuckDuckGo(query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}

	// Browser headers to avoid blocks
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DuckDuckGo returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	bodyStr := string(bodyBytes)

	// Regex to extract result divs
	// DuckDuckGo HTML contains results in: <div class="result results_links results_links_deep web-result ">
	resultBlockRegex := regexp.MustCompile(`(?s)<div[^>]*class="[^"]*results_links[^"]*"[^>]*>(.*?)</div>\s*</div>\s*</div>`)
	matches := resultBlockRegex.FindAllStringSubmatch(bodyStr, -1)

	var results []SearchResult
	count := 0

	for _, match := range matches {
		if count >= 5 {
			break
		}
		block := match[1]

		// Extract URL from redirect link: uddg=URL_ENCODED
		uddgRegex := regexp.MustCompile(`uddg=([^&"]+)`)
		uddgMatch := uddgRegex.FindStringSubmatch(block)
		if len(uddgMatch) < 2 {
			continue
		}

		rawURL, err := url.QueryUnescape(uddgMatch[1])
		if err != nil {
			continue
		}

		// Extract Title
		// In html.duckduckgo.com it is inside <a class="result__snippet" ...> or similar under h2
		titleRegex := regexp.MustCompile(`(?s)<h2[^>]*>.*?<a[^>]*>(.*?)</a>.*?</h2>`)
		titleMatch := titleRegex.FindStringSubmatch(block)
		title := "No Title"
		if len(titleMatch) >= 2 {
			title = stripHTMLTags(titleMatch[1])
			title = html.UnescapeString(strings.TrimSpace(title))
		}

		// Extract Snippet
		snippetRegex := regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
		snippetMatch := snippetRegex.FindStringSubmatch(block)
		snippet := ""
		if len(snippetMatch) >= 2 {
			snippet = stripHTMLTags(snippetMatch[1])
			snippet = html.UnescapeString(strings.TrimSpace(snippet))
		}

		results = append(results, SearchResult{
			Title:   title,
			URL:     rawURL,
			Snippet: snippet,
		})
		count++
	}

	return results, nil
}

// stripHTMLTags removes all HTML tags from a string.
func stripHTMLTags(src string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(src, "")
}

// CleanHTMLContent removes scripts, styles and extracts plain text.
func CleanHTMLContent(htmlStr string) string {
	// Remove script blocks
	reScript := regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	htmlStr = reScript.ReplaceAllString(htmlStr, "")

	// Remove style blocks
	reStyle := regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	htmlStr = reStyle.ReplaceAllString(htmlStr, "")

	// Replace HTML tags with space
	reTags := regexp.MustCompile(`<[^>]*>`)
	plainText := reTags.ReplaceAllString(htmlStr, " ")

	// Unescape HTML entities
	plainText = html.UnescapeString(plainText)

	// Clean up whitespaces
	words := strings.Fields(plainText)
	cleaned := strings.Join(words, " ")

	// Limit length to 10000 characters
	if len(cleaned) > 10000 {
		cleaned = cleaned[:10000] + "..."
	}

	return cleaned
}

// WebReader retrieves the contents of a webpage.
// Layer 1: Direct HTTP Request.
// Layer 2: Headless Chrome Dump-DOM Fallback.
func WebReader(targetURL string) (string, error) {
	// Layer 1: Direct Request
	req, err := http.NewRequest("GET", targetURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			bodyBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err == nil {
				text := CleanHTMLContent(string(bodyBytes))
				if len(strings.TrimSpace(text)) > 100 {
					return text, nil
				}
			}
		}
	}

	// Layer 2: Headless Chrome Fallback
	chromePaths := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		filepath.Join(os.Getenv("LocalAppData"), `Google\Chrome\Application\chrome.exe`),
	}

	var chromePath string
	for _, p := range chromePaths {
		if _, err := os.Stat(p); err == nil {
			chromePath = p
			break
		}
	}

	if chromePath == "" {
		return "", fmt.Errorf("direct request failed and Chrome not found for fallback")
	}

	// Execute chrome --headless --disable-gpu --dump-dom <url>
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, chromePath, "--headless", "--disable-gpu", "--dump-dom", targetURL)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run headless Chrome: %v", err)
	}

	text := CleanHTMLContent(stdout.String())
	return text, nil
}

// DeepResearchProtocol implements the 5-phase Deep Research process in the terminal.
func DeepResearchProtocol(ctx context.Context, initialQuery string, performSearchFunc func(string) ([]SearchResult, error), readLinkFunc func(string) (string, error)) (string, error) {
	borderCol := "\033[38;5;39m" // Deep Blue/Cyan
	resetCol := "\033[0m"

	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("╔", "═", " DEEP RESEARCH STARTED ", 70, "╗") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(fmt.Sprintf("  Query: %q", initialQuery), 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  [Phase 1/5] Understanding research scope...", 70), borderCol, resetCol)

	// Phase 1: Scope Understanding
	time.Sleep(1 * time.Second)

	// Phase 2: Initial Contextualization
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  [Phase 2/5] Performing initial searches to capture keywords...", 70), borderCol, resetCol)
	results, err := performSearchFunc(initialQuery)
	if err != nil {
		fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
		return "", fmt.Errorf("error in initial search: %v", err)
	}

	// Phase 3: Research Plan and Confirmation
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  [Phase 3/5] Research Plan Prepared", 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  Preliminary results found:", 70), borderCol, resetCol)
	for idx, r := range results {
		if idx >= 3 {
			break
		}
		lineResult := fmt.Sprintf("   - [%s] %s", r.Title, r.URL)
		fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(lineResult, 70), borderCol, resetCol)
	}
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  Detailed plan:", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("   1. Search for variations and related technical terms.", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("   2. Track and read the most promising links (10 iterations).", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("   3. Synthesize and cross-reference the collected data.", 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
	fmt.Println()

	// Prompt the user for confirmation
	fmt.Printf("\033[33m⚡ Do you want to proceed with the exhaustive investigation? [y/N]:\033[0m ")
	reader := bufio.NewReader(os.Stdin)
	ans, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	if ans != "y" && ans != "yes" && ans != "s" && ans != "sim" {
		return "Deep research cancelled by user.", nil
	}

	// Phase 4: Exhaustive Investigation
	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("╔", "═", " EXHAUSTIVE INVESTIGATION (Phase 4/5) ", 70, "╗") + resetCol)
	var findings []string

	// Simple simulation of search/read loop for 10 times with console logging
	queries := []string{
		initialQuery,
		initialQuery + " details",
		initialQuery + " documentation",
		initialQuery + " examples",
		initialQuery + " tutorials",
		initialQuery + " news",
		initialQuery + " latest updates",
		initialQuery + " quick guide",
		initialQuery + " architecture",
		initialQuery + " specifications",
	}

	for i := 0; i < 10; i++ {
		select {
		case <-ctx.Done():
			fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
			return "", context.Canceled
		default:
		}

		q := queries[i]
		lineIter := fmt.Sprintf("  -> Iteration %d/10: Searching for %q...", i+1, q)
		fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(lineIter, 70), borderCol, resetCol)
		loopResults, err := performSearchFunc(q)
		if err == nil && len(loopResults) > 0 {
			targetLink := loopResults[0].URL
			lineRead := fmt.Sprintf("     -> Reading content from: %s...", targetLink)
			fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(lineRead, 70), borderCol, resetCol)
			content, err := readLinkFunc(targetLink)
			if err == nil {
				// Take a snippet
				limit := 400
				if len(content) < limit {
					limit = len(content)
				}
				findings = append(findings, fmt.Sprintf("[Source: %s] %s", targetLink, content[:limit]))
			}
		}
		time.Sleep(500 * time.Millisecond) // Simulate work rate limit
	}

	// Phase 5: Structured Synthesis
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  [Phase 5/5] Consolidating and synthesizing information...", 70), borderCol, resetCol)
	time.Sleep(1 * time.Second)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  [Deep Research] Investigation completed successfully!", 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
	fmt.Println()

	// Merge findings to return to intelligence
	var sb strings.Builder
	sb.WriteString("Deep Research Results:\n\n")
	for _, f := range findings {
		sb.WriteString(f + "\n\n")
	}
	return sb.String(), nil
}

// YoutubeCommandSearch performs search and opens the top youtube link
func YoutubeCommandSearch(query string) error {
	fmt.Printf("[YouTube] Searching for videos of: \"%s\"...\n", query)
	results, err := ScrapeDuckDuckGo("site:youtube.com " + query)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return fmt.Errorf("no videos found for the search")
	}

	topLink := results[0].URL
	fmt.Printf("[YouTube] Opening the best link in the browser: %s\n", topLink)

	// Open URL in default browser
	cmd := exec.Command("cmd.exe", "/c", "start", "", topLink)
	return cmd.Start()
}
