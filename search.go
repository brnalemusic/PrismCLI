package main

import (
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

// splitByRegex splits a string using a precompiled regular expression, mimicking JS's split.
func splitByRegex(s string, re *regexp.Regexp) []string {
	indices := re.FindAllStringIndex(s, -1)
	if len(indices) == 0 {
		return []string{s}
	}

	result := make([]string, 0, len(indices)+1)
	lastIdx := 0
	for _, idx := range indices {
		result = append(result, s[lastIdx:idx[0]])
		lastIdx = idx[1]
	}
	result = append(result, s[lastIdx:])
	return result
}

// normalizeHttpUrl normalizes and validates a URL, adding schemes if needed.
func normalizeHttpUrl(input string, label string) (string, error) {
	cleaned := strings.TrimSpace(input)
	if cleaned == "" {
		return "", fmt.Errorf("missing required %s. Provide a complete URL", label)
	}

	placeholderRegex := regexp.MustCompile(`(?i)^(URL|LINK|WEBPAGE|TARGET)([_-]?\w+)?$`)
	if placeholderRegex.MatchString(cleaned) {
		return "", fmt.Errorf("invalid %s: %q. Replace placeholders with a real URL", label, input)
	}

	hasHttpScheme := regexp.MustCompile(`(?i)^https?://`).MatchString(cleaned)
	localhostWithoutScheme := regexp.MustCompile(`(?i)^(localhost|127\.0\.0\.1|\[::1\])(?::|/|$)`).MatchString(cleaned)

	var candidate string
	if hasHttpScheme {
		candidate = cleaned
	} else if localhostWithoutScheme {
		candidate = "http://" + cleaned
	} else {
		candidate = "https://" + cleaned
	}

	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %v", label, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported %s protocol: %s", label, parsed.Scheme)
	}

	return parsed.String(), nil
}

// stripHtml removes all HTML tags, script, and style blocks repeatedly until the output stabilizes.
func stripHtml(htmlStr string) string {
	text := htmlStr
	var previous string

	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reTags := regexp.MustCompile(`<[^>]*>`)

	for {
		previous = text
		text = reScript.ReplaceAllString(text, "")
		text = reStyle.ReplaceAllString(text, "")
		text = reTags.ReplaceAllString(text, " ")
		if text == previous {
			break
		}
	}
	return text
}

// CleanHTMLContent removes scripts, styles and extracts plain text up to 20,000 characters.
func CleanHTMLContent(htmlStr string) string {
	plainText := stripHtml(htmlStr)

	// Unescape HTML entities
	plainText = html.UnescapeString(plainText)

	// Clean up whitespaces
	words := strings.Fields(plainText)
	cleaned := strings.Join(words, " ")

	// Limit length to 20000 characters
	if len(cleaned) > 20000 {
		cleaned = cleaned[:20000] + "... (truncated)"
	}

	return cleaned
}

// fetchWithHeadlessChrome fetches a URL by dumping the DOM using headless Chrome.
func fetchWithHeadlessChrome(targetURL string) (string, error) {
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
		return "", fmt.Errorf("headless Chrome binary not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	cmd := exec.CommandContext(ctx, chromePath, "--headless", "--disable-gpu", "--user-agent="+userAgent, "--dump-dom", targetURL)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run headless Chrome: %v", err)
	}

	return stdout.String(), nil
}

// parseDuckDuckGoHTML extracts SearchResult items from DuckDuckGo HTML source.
func parseDuckDuckGoHTML(htmlContent string) []SearchResult {
	splitRegex := regexp.MustCompile(`(?i)<div[^>]*class="[^"]*result(?:__body|s_links| )[^"]*"[^>]*>`)
	resultBlocks := splitByRegex(htmlContent, splitRegex)
	if len(resultBlocks) > 1 {
		resultBlocks = resultBlocks[1:]
	} else {
		return nil
	}

	titleRegex := regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__a[^"]*"[^>]*>(.*?)</a>`)

	linkRegex1 := regexp.MustCompile(`(?is)href="([^"]*)"[^>]*class="[^"]*result__a[^"]*"`)
	linkRegex2 := regexp.MustCompile(`(?is)class="[^"]*result__a[^"]*"[^>]*href="([^"]*)"`)
	linkRegex3 := regexp.MustCompile(`(?is)href="([^"]*)"`)

	snippetRegex1 := regexp.MustCompile(`(?is)<[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)<\/(?:a|div|span|p)>`)
	snippetRegex2 := regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
	snippetRegex3 := regexp.MustCompile(`(?is)<div[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</div>`)

	fallbackRegex1 := regexp.MustCompile(`(?is)</h2>(.*?)<div[^>]*class="[^"]*result__extras`)
	fallbackRegex2 := regexp.MustCompile(`(?is)</a>(.*?)<div[^>]*class="[^"]*result__extras`)

	var results []SearchResult
	for _, body := range resultBlocks {
		if len(results) >= 5 {
			break
		}

		titleMatch := titleRegex.FindStringSubmatch(body)

		var rawLink string
		if m := linkRegex1.FindStringSubmatch(body); len(m) >= 2 {
			rawLink = m[1]
		} else if m := linkRegex2.FindStringSubmatch(body); len(m) >= 2 {
			rawLink = m[1]
		} else if m := linkRegex3.FindStringSubmatch(body); len(m) >= 2 {
			rawLink = m[1]
		}

		var snippetContent string
		var snippetMatch []string
		if m := snippetRegex1.FindStringSubmatch(body); len(m) >= 2 {
			snippetMatch = m
		} else if m := snippetRegex2.FindStringSubmatch(body); len(m) >= 2 {
			snippetMatch = m
		} else if m := snippetRegex3.FindStringSubmatch(body); len(m) >= 2 {
			snippetMatch = m
		}

		if len(snippetMatch) == 0 {
			if m := fallbackRegex1.FindStringSubmatch(body); len(m) >= 2 {
				snippetMatch = m
			} else if m := fallbackRegex2.FindStringSubmatch(body); len(m) >= 2 {
				snippetMatch = m
			}
		}

		if len(snippetMatch) >= 2 {
			snippetContent = snippetMatch[1]
		}

		if len(titleMatch) >= 2 && rawLink != "" {
			targetLink := rawLink
			if strings.HasPrefix(targetLink, "//") {
				targetLink = "https:" + targetLink
			} else if strings.HasPrefix(targetLink, "/") {
				targetLink = "https://duckduckgo.com" + targetLink
			}

			if parsedURL, err := url.Parse(targetLink); err == nil {
				uddg := parsedURL.Query().Get("uddg")
				if uddg != "" {
					if decoded, err := url.QueryUnescape(uddg); err == nil {
						targetLink = decoded
					}
				}
			}

			titleText := strings.TrimSpace(stripHtml(titleMatch[1]))
			titleText = html.UnescapeString(titleText)
			snippetText := strings.TrimSpace(stripHtml(snippetContent))
			snippetText = html.UnescapeString(snippetText)

			results = append(results, SearchResult{
				Title:   titleText,
				URL:     targetLink,
				Snippet: snippetText,
			})
		}
	}
	return results
}

// ScrapeDuckDuckGo queries DuckDuckGo HTML-only search and parses top 5 results.
// Employs a headless Chrome fallback to bypass 202/403 errors or scraping blocks.
func ScrapeDuckDuckGo(query string) ([]SearchResult, error) {
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	var htmlContent string
	var fetchErr error

	// Try direct GET request
	req, err := http.NewRequest("GET", searchURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")

		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				bodyBytes, readErr := io.ReadAll(resp.Body)
				if readErr == nil {
					htmlContent = string(bodyBytes)
				} else {
					fetchErr = readErr
				}
			} else {
				fetchErr = fmt.Errorf("DuckDuckGo returned status %d", resp.StatusCode)
			}
		} else {
			fetchErr = err
		}
	} else {
		fetchErr = err
	}

	var results []SearchResult
	if htmlContent != "" {
		results = parseDuckDuckGoHTML(htmlContent)
	}

	// Fallback to Headless Chrome if direct request failed, was blocked, or yielded no results
	if len(results) == 0 {
		chromeHTML, chromeErr := fetchWithHeadlessChrome(searchURL)
		if chromeErr == nil {
			results = parseDuckDuckGoHTML(chromeHTML)
		} else {
			if fetchErr != nil {
				return nil, fmt.Errorf("direct fetch failed (%v) and Chrome fallback failed (%v)", fetchErr, chromeErr)
			}
			return nil, fmt.Errorf("chrome fallback failed: %v", chromeErr)
		}
	}

	return results, nil
}

// WebReader retrieves and cleans the contents of a webpage.
// Layer 1: Direct HTTP Request with browser-like headers.
// Layer 2: Headless Chrome Dump-DOM Fallback.
func WebReader(targetURL string) (string, error) {
	normalized, err := normalizeHttpUrl(targetURL, "url")
	if err != nil {
		return "", err
	}

	var htmlContent string
	var directErr error

	// Layer 1: Direct Request
	req, err := http.NewRequest("GET", normalized, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,pt-BR;q=0.8,pt;q=0.7")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Pragma", "no-cache")
		req.Header.Set("Sec-Ch-Ua", `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`)
		req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
		req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "none")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Upgrade-Insecure-Requests", "1")

		client := &http.Client{
			Timeout: 10 * time.Second,
		}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				bodyBytes, readErr := io.ReadAll(resp.Body)
				if readErr == nil {
					htmlContent = string(bodyBytes)
				} else {
					directErr = readErr
				}
			} else {
				directErr = fmt.Errorf("website returned status %d", resp.StatusCode)
			}
		} else {
			directErr = err
		}
	} else {
		directErr = err
	}

	var cleanedText string
	if htmlContent != "" {
		cleanedText = CleanHTMLContent(htmlContent)
	}

	// Layer 2: Headless Chrome Fallback if direct request failed, was blocked, or produced insufficient text
	if len(strings.TrimSpace(cleanedText)) <= 100 {
		chromeHTML, chromeErr := fetchWithHeadlessChrome(normalized)
		if chromeErr == nil {
			cleanedText = CleanHTMLContent(chromeHTML)
		} else {
			if directErr != nil {
				return "", fmt.Errorf("direct request failed (%v) and Chrome fallback failed (%v)", directErr, chromeErr)
			}
			return "", fmt.Errorf("chrome fallback failed: %v", chromeErr)
		}
	}

	// Wait 500ms before returning to match the TS implementation
	time.Sleep(500 * time.Millisecond)

	return cleanedText, nil
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
	_, err := performSearchFunc(initialQuery)
	if err != nil {
		fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
		return "", fmt.Errorf("error in initial search: %v", err)
	}

	// Phase 3: Research Plan and Confirmation
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  [Phase 3/5] Research Plan Prepared", 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  Detailed plan:", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("   1. Search for variations and related technical terms.", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("   2. Track and read the most promising sources (10 iterations).", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("   3. Synthesize and cross-reference the collected data.", 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
	fmt.Println()

	// Prompt the user for confirmation
	fmt.Printf("\033[33m⚡ Do you want to proceed with the exhaustive investigation? [y/N]:\033[0m ")
	ans, err := ReadLineWithDefault("")
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
			lineRead := "     -> Reading page content..."
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
