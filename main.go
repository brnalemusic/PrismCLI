package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/mattn/go-runewidth"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

const Version = "0.3.1"

const SystemPrompt = `You are Prism, a highly capable local AI agent operating in command-line (CLI) mode on Windows.
Your core philosophy is direct local action, total privacy, and native integration with the operating system.
You have local tools to:
1. Execute terminal commands (execute_terminal_command).
2. Manipulate files (computer_use_create_file, computer_use_create_directory, computer_use_remove_file, computer_use_remove_directory, computer_use_save_file, computer_use_append_file, computer_use_edit_file, computer_use_copy_file, computer_use_move_file, computer_use_get_file_info, computer_use_list_directory, computer_use_read_file).
3. Locate and launch applications (list_installed_applications, open_application).
4. Perform web searches (web_search, saw_link_from_url) and open links in the browser (open_browser_link).
5. Perform semantic search in past histories (search_chat_history).
6. Execute and coordinate virtual subagent swarms (run_subagents).
7. Configure Prism (configure_prism) or open the main Prism Desktop window (open_main_app).

When interacting:
- Be direct, clear, and action-oriented.
- If the user asks you to create an interactive interface or complex functional visual module (Level 3), generate the code encapsulated in the <mini-app>...</mini-app> tag containing the full HTML structure, CSS styles, and JS logic in a single block. The local interpreter will temporarily save this file and open it in the user's browser.
- If the user requests a web search, you must use the web_search and saw_link_from_url tools.
- Always follow the tools' security safeguards.`

func main() {
	// Set Windows console to UTF-8
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	_, _, _ = setConsoleCP.Call(65001)
	_, _, _ = setConsoleOutputCP.Call(65001)

	// CLI Flags
	searchFlag := flag.Bool("search", false, "Start chat with Active Search enabled")
	deepFlag := flag.Bool("deep", false, "Start chat with Extended Search (Deep Research) enabled")
	versionFlag := flag.Bool("version", false, "Display Prism version")
	configFlag := flag.Bool("config", false, "Open API key configuration wizard")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("Prism CLI - Version %s\n", Version)
		return
	}

	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Check for updates
	if !*versionFlag {
		CheckAndPerformUpdate(&cfg)
	}

	// Trigger Setup Wizard if config flag is passed or API key is missing
	apiKey, keyErr := cfg.GetAPIKey()
	if *configFlag || keyErr != nil || apiKey == "" {
		err := RunSetupWizard(&cfg)
		if err != nil {
			fmt.Printf("Configuration error: %v\n", err)
			os.Exit(1)
		}
		if *configFlag {
			return
		}
		// Reload config after wizard
		cfg, _ = LoadConfig()
	}

	// Check for other commands passed via positional arguments
	args := flag.Args()
	if len(args) > 0 {
		cmd := strings.ToLower(args[0])
		switch cmd {
		case "config":
			err := RunSetupWizard(&cfg)
			if err != nil {
				fmt.Printf("Configuration error: %v\n", err)
			}
			return
		case "list":
			sessions, err := ListChatSessions()
			if err != nil {
				fmt.Printf("Error listing chats: %v\n", err)
				return
			}
			fmt.Println("\n=== SAVED CHAT HISTORY ===")
			for _, s := range sessions {
				t := time.Unix(s.LastModified, 0).Format("2006-01-02 15:04")
				fmt.Printf(" - ID: %s | Title: \"%s\" (Modified: %s)\n", s.ID, s.Title, t)
			}
			fmt.Println()
			return
		}
	}

	// Start Chat Session REPL
	runChatREPL(&cfg, *searchFlag, *deepFlag)
}

func runChatREPL(cfg *Config, startSearch, startDeep bool) {
	sessionID := fmt.Sprintf("chat_%d", time.Now().Unix())
	session := ChatSession{
		ID:       sessionID,
		Title:    "Quick Chat",
		Messages: make([]ChatMessage, 0),
	}

	activeSearch := startSearch
	deepResearch := startDeep
	thinkMode := true // Think Mode default enabled for reasoning models

	drawWelcomeScreen(cfg, thinkMode, activeSearch, deepResearch)

	promptFn := func() string {
		userName := "User"
		if u := os.Getenv("USERNAME"); u != "" {
			userName = u
		}

		promptSymbol := fmt.Sprintf("\033[1;34m%s >\033[0m ", userName)
		if thinkMode {
			if deepResearch {
				promptSymbol = fmt.Sprintf("\033[1;36m%s (Deep Research + Thinking) >\033[0m ", userName)
			} else if activeSearch {
				promptSymbol = fmt.Sprintf("\033[1;32m%s (Search + Thinking) >\033[0m ", userName)
			} else {
				promptSymbol = fmt.Sprintf("\033[1;33m%s (Thinking) >\033[0m ", userName)
			}
		} else {
			if deepResearch {
				promptSymbol = fmt.Sprintf("\033[1;36m%s (Deep Research) >\033[0m ", userName)
			} else if activeSearch {
				promptSymbol = fmt.Sprintf("\033[1;32m%s (Search) >\033[0m ", userName)
			}
		}
		return promptSymbol
	}

	for {
		fmt.Println()
		input, err := ReadLine(cfg, promptFn, &thinkMode, &activeSearch, &deepResearch, len(session.Messages) == 0)
		if err != nil {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Process Slash Commands
		if strings.HasPrefix(input, "/") {
			parts := strings.SplitN(input, " ", 2)
			cmd := strings.ToLower(parts[0])
			var arg string
			if len(parts) > 1 {
				arg = parts[1]
			}

			switch cmd {
			case "/exit", "/quit":
				fmt.Println("Exiting Prism CLI. Goodbye!")
				return
			case "/search":
				activeSearch = !activeSearch
				if activeSearch {
					deepResearch = false // search and deep are mutually exclusive
				}
				updateStateAndRedraw(cfg, len(session.Messages) == 0, thinkMode, activeSearch, deepResearch, promptFn)
				continue
			case "/deep", "/research":
				deepResearch = !deepResearch
				if deepResearch {
					activeSearch = false
				}
				updateStateAndRedraw(cfg, len(session.Messages) == 0, thinkMode, activeSearch, deepResearch, promptFn)
				continue
			case "/youtube":
				if arg == "" {
					fmt.Println("Error: Provide search terms for the video. E.g.: /youtube lo-fi hip hop")
					continue
				}
				err := YoutubeCommandSearch(arg)
				if err != nil {
					fmt.Printf("YouTube command error: %v\n", err)
				}
				continue
			case "/swarm":
				if arg == "" {
					fmt.Println("Error: Provide the global goal for the swarm. E.g.: /swarm create backup script")
					continue
				}
				RunSwarmTask(context.Background(), arg)
				continue
			case "/think", "/thinking":
				thinkMode = !thinkMode
				updateStateAndRedraw(cfg, len(session.Messages) == 0, thinkMode, activeSearch, deepResearch, promptFn)
				continue
			case "/model":
				if arg == "" {
					selectedModel, ok := selectModelInteractively(cfg)
					if ok {
						cfg.DefaultModel = selectedModel
						_ = SaveConfig(*cfg)
					}
					updateStateAndRedraw(cfg, len(session.Messages) == 0, thinkMode, activeSearch, deepResearch, promptFn)
					continue
				}
				var selectedModel string
				var found bool
				var index int
				_, err := fmt.Sscanf(arg, "%d", &index)
				if err == nil && index >= 1 && index <= len(SelectableModels) {
					selectedModel = SelectableModels[index-1]
					found = true
				} else {
					cleanedArg := strings.TrimSpace(strings.ToLower(arg))
					for _, m := range SelectableModels {
						friendly := strings.ToLower(ModelFriendlyNames[m])
						if m == cleanedArg || friendly == cleanedArg || strings.Contains(friendly, cleanedArg) {
							selectedModel = m
							found = true
							break
						}
					}
				}

				if found {
					cfg.DefaultModel = selectedModel
					_ = SaveConfig(*cfg)
					updateStateAndRedraw(cfg, len(session.Messages) == 0, thinkMode, activeSearch, deepResearch, promptFn)
				} else {
					fmt.Print("\033[1A\r\033[K")
					fmt.Printf("✗ Model '%s' not found. Type '/model' to choose from list.\n", arg)
					fmt.Print(promptFn())
				}
				continue
			case "/config":
				err := RunSetupWizard(cfg)
				if err != nil {
					fmt.Printf("Configuration error: %v\n", err)
				}
				continue
			case "/clear":
				cmdCls := exec.Command("cmd", "/c", "cls")
				cmdCls.Stdout = os.Stdout
				_ = cmdCls.Run()
				sessionID = fmt.Sprintf("chat_%d", time.Now().Unix())
				session = ChatSession{
					ID:       sessionID,
					Title:    "Quick Chat",
					Messages: make([]ChatMessage, 0),
				}
				drawWelcomeScreen(cfg, thinkMode, activeSearch, deepResearch)
				fmt.Println("\n\033[32m✔ Chat and history cleared successfully.\033[0m")
				continue
			case "/help":
				fmt.Println()
				drawHelpBox()
				continue
			default:
				fmt.Printf("Unknown command: %s. Type /help to see commands.\n", cmd)
				continue
			}
		}

		// Handle Deep Research if enabled as a pre-process
		if deepResearch {
			res, err := DeepResearchProtocol(
				context.Background(),
				input,
				ScrapeDuckDuckGo,
				WebReader,
			)
			if err != nil {
				fmt.Printf("Deep research error: %v\n", err)
				continue
			}
			// Inject results as user context message
			input = fmt.Sprintf("User requested deep investigation on: %s\n\nResults obtained:\n%s", input, res)
			// Reset deep research mode for the next prompt after running once
			deepResearch = false
		}

		// Append user message to history
		userMsg := ChatMessage{
			Role:      "user",
			Content:   input,
			Timestamp: time.Now().Unix(),
		}
		session.Messages = append(session.Messages, userMsg)

		// Set chat title if it's the first message
		if len(session.Messages) == 1 {
			titleLimit := 30
			if len(input) < titleLimit {
				titleLimit = len(input)
			}
			session.Title = input[:titleLimit] + "..."
		}

		// Generate response from AI
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		modelResp, err := GenerateResponse(ctx, cfg, session.Messages, SystemPrompt, thinkMode, activeSearch, deepResearch)
		cancel()

		if err != nil {
			fmt.Printf("Error getting response from AI: %v\n", err)
			continue
		}

		// Append model response to history
		modelMsg := ChatMessage{
			Role:      "model",
			Content:   modelResp,
			Model:     cfg.DefaultModel,
			Timestamp: time.Now().Unix(),
		}
		session.Messages = append(session.Messages, modelMsg)

		// Persist chat session to disk
		_ = SaveChatSession(session)

		// Detect level 3 Mini Apps inside tags <mini-app>...</mini-app>
		processMiniApp(modelResp)
	}
}

// processMiniApp checks if the generated text contains a <mini-app> tag.
// If found, it extracts it, saves it locally as HTML and opens it in the browser.
func processMiniApp(text string) {
	re := regexp.MustCompile(`(?s)<mini-app>(.*?)</mini-app>`)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return
	}

	content := strings.TrimSpace(match[1])
	if content == "" {
		return
	}

	// Create temp directory for mini-apps
	tempDir := filepath.Join(os.TempDir(), "PrismMiniApps")
	_ = os.MkdirAll(tempDir, 0755)

	filePath := filepath.Join(tempDir, fmt.Sprintf("app_%d.html", time.Now().UnixNano()))
	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		fmt.Printf("\033[31m[Mini-App Error] Failed to write local mini-app: %v\033[0m\n", err)
		return
	}

	fmt.Printf("\n\033[32m[Mini-App] Interactive module generated and saved at: %s\033[0m\n", filePath)
	fmt.Println("\033[32m[Mini-App] Opening in the operating system's default browser...\033[0m")

	// Open the file url
	fileURL := "file:///" + filepath.ToSlash(filePath)
	cmd := exec.Command("cmd.exe", "/c", "start", "", fileURL)
	_ = cmd.Start()
}

type KEY_EVENT_RECORD struct {
	BKeyDown          int32
	WRepeatCount      uint16
	WVirtualKeyCode   uint16
	WVirtualScanCode  uint16
	UnicodeChar       uint16
	DwControlKeyState uint32
}

type INPUT_RECORD struct {
	EventType uint16
	Padding   uint16
	Event     [16]byte
}

var SelectableModels = []string{
	"gemini-3.5-flash",       // Prism 5
	"gemma-4-31b-it",         // Prism 4.3
	"gemma-4-26b-a4b-it",     // Prism 4.2
	"gemini-3-flash-preview", // Prism 4.1
	"gemini-3.1-flash-lite",  // Prism 4
}

func drawModelMenu(selectedIndex int, activeModel string) {
	fmt.Print("\r\033[K")
	borderCol := "\033[38;5;129m" // Magenta/Purple
	resetCol := "\033[0m"
	width := getBoxWidth()

	fmt.Println(borderCol + drawBoxHeader("╔", "═", " SELECT AI MODEL ", width, "╗") + resetCol)
	for i, modelID := range SelectableModels {
		friendlyName := ModelFriendlyNames[modelID]
		marker := "[ ]"
		if modelID == activeModel {
			marker = "\033[32m[✔]\033[0m"
		}

		var line string
		if i == selectedIndex {
			line = fmt.Sprintf("  \033[1;36m❯ %s %s (%s)\033[0m", marker, friendlyName, modelID)
		} else {
			line = fmt.Sprintf("    %s %s (%s)", marker, friendlyName, modelID)
		}
		fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(line, width), borderCol, resetCol)
	}
	fmt.Println(borderCol + drawBoxLine("╠", "═", width, "╣") + resetCol)
	hintLine := "  Use ↑/↓ to navigate, Enter to confirm, Esc to cancel"
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(hintLine, width), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", width, "╝") + resetCol)
}

func clearLines(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[A\r\033[K")
	}
}

func selectModelInteractively(cfg *Config) (string, bool) {
	selectedIndex := 0
	for idx, m := range SelectableModels {
		if m == cfg.DefaultModel {
			selectedIndex = idx
			break
		}
	}

	stdin := windows.Handle(os.Stdin.Fd())
	var mode uint32
	isConsole := windows.GetConsoleMode(stdin, &mode) == nil

	if isConsole {
		originalMode := mode
		// Disable line input (0x0002) and echo input (0x0004)
		// keep processed input (0x0001) so Ctrl+C is still handled by Go signal runtime
		rawMode := originalMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
		_ = windows.SetConsoleMode(stdin, rawMode)
		defer windows.SetConsoleMode(stdin, originalMode)
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procReadConsoleInputW := kernel32.NewProc("ReadConsoleInputW")

	drawModelMenu(selectedIndex, cfg.DefaultModel)

	for {
		var record INPUT_RECORD
		var read uint32
		r, _, _ := procReadConsoleInputW.Call(
			uintptr(stdin),
			uintptr(unsafe.Pointer(&record)),
			1,
			uintptr(unsafe.Pointer(&read)),
		)
		if r == 0 || read == 0 {
			continue
		}
		if record.EventType != 1 {
			continue
		}
		keyEvent := (*KEY_EVENT_RECORD)(unsafe.Pointer(&record.Event[0]))
		if keyEvent.BKeyDown == 0 {
			continue
		}

		vk := keyEvent.WVirtualKeyCode

		switch vk {
		case 0x26: // VK_UP
			selectedIndex--
			if selectedIndex < 0 {
				selectedIndex = len(SelectableModels) - 1
			}
			clearLines(4 + len(SelectableModels))
			drawModelMenu(selectedIndex, cfg.DefaultModel)

		case 0x28: // VK_DOWN
			selectedIndex++
			if selectedIndex >= len(SelectableModels) {
				selectedIndex = 0
			}
			clearLines(4 + len(SelectableModels))
			drawModelMenu(selectedIndex, cfg.DefaultModel)

		case 0x0D: // VK_RETURN
			clearLines(4 + len(SelectableModels))
			return SelectableModels[selectedIndex], true

		case 0x1B: // VK_ESCAPE
			clearLines(4 + len(SelectableModels))
			return "", false
		}
	}
}

func runModelSelectionMenu(cfg *Config, promptFn func() string, currentLine []rune, thinkMode, activeSearch, deepResearch bool, isWelcomeScreen bool) {
	selectedModel, ok := selectModelInteractively(cfg)
	if ok {
		cfg.DefaultModel = selectedModel
		_ = SaveConfig(*cfg)
		if isWelcomeScreen {
			// Save cursor position
			fmt.Print("\033[s")
			// Move up 17 lines to the start of the configuration box
			fmt.Print("\033[17A\r")
			// Redraw active state box
			drawActiveStateBox(cfg, thinkMode, activeSearch, deepResearch)
			// Restore cursor position
			fmt.Print("\033[u")
		}
	}
	fmt.Printf("%s%s", promptFn(), string(currentLine))
}

// ReadLine reads input character by character from the console on Windows in raw mode,
// allowing keyboard shortcuts like Ctrl+T and Ctrl+M to toggle modes and choose models.
func ReadLine(cfg *Config, promptFn func() string, thinkMode *bool, activeSearch *bool, deepResearch *bool, isWelcomeScreen bool) (string, error) {
	stdin := windows.Handle(os.Stdin.Fd())
	var mode uint32
	isConsole := windows.GetConsoleMode(stdin, &mode) == nil

	if isConsole {
		originalMode := mode
		// Disable line input (0x0002) and echo input (0x0004)
		// keep processed input (0x0001) so Ctrl+C is still handled by Go signal runtime
		rawMode := originalMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
		_ = windows.SetConsoleMode(stdin, rawMode)
		defer windows.SetConsoleMode(stdin, originalMode)
	}

	if !isConsole {
		fmt.Print(promptFn())
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	}

	var line []rune
	fmt.Print(promptFn())

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procReadConsoleInputW := kernel32.NewProc("ReadConsoleInputW")

	for {
		var record INPUT_RECORD
		var read uint32
		r, _, err := procReadConsoleInputW.Call(
			uintptr(stdin),
			uintptr(unsafe.Pointer(&record)),
			1,
			uintptr(unsafe.Pointer(&read)),
		)
		if r == 0 {
			return "", fmt.Errorf("ReadConsoleInputW failed: %v", err)
		}
		if read == 0 {
			continue
		}

		if record.EventType != 1 {
			continue
		}

		keyEvent := (*KEY_EVENT_RECORD)(unsafe.Pointer(&record.Event[0]))
		if keyEvent.BKeyDown == 0 {
			continue
		}

		vk := keyEvent.WVirtualKeyCode
		char := rune(keyEvent.UnicodeChar)
		ctrl := keyEvent.DwControlKeyState
		hasCtrl := (ctrl&0x0008) != 0 || (ctrl&0x0004) != 0 // LEFT_CTRL_PRESSED or RIGHT_CTRL_PRESSED

		// 1. Detect Ctrl+T (also fallback to ASCII control code 20)
		if (hasCtrl && (vk == 'T' || vk == 't')) || char == 20 {
			*thinkMode = !*thinkMode
			if isWelcomeScreen {
				// Save cursor position
				fmt.Print("\033[s")
				// Move up 17 lines to the start of the configuration box
				fmt.Print("\033[17A\r")
				// Redraw active state box
				drawActiveStateBox(cfg, *thinkMode, *activeSearch, *deepResearch)
				// Restore cursor position
				fmt.Print("\033[u")
			}
			fmt.Print("\r\033[K")
			fmt.Printf("%s%s", promptFn(), string(line))
			continue
		}

		// 2. Detect Ctrl+M (also fallback to ASCII control code 13, distinguishing from Enter using the Virtual Key Code)
		if (hasCtrl && (vk == 'M' || vk == 'm')) || (char == 13 && (vk == 'M' || vk == 'm')) {
			runModelSelectionMenu(cfg, promptFn, line, *thinkMode, *activeSearch, *deepResearch, isWelcomeScreen)
			continue
		}

		// 3. Detect Enter
		if vk == 0x0D { // VK_RETURN
			fmt.Println()
			return string(line), nil
		}

		// 4. Detect Backspace
		if vk == 0x08 { // VK_BACK
			if len(line) > 0 {
				line = line[:len(line)-1]
				fmt.Print("\b \b")
			}
			continue
		}

		// 5. Detect Escape (VK_ESCAPE)
		if vk == 0x1B {
			fmt.Println()
			return "", nil
		}

		// 6. Normal character input
		if char >= 32 {
			line = append(line, char)
			fmt.Print(string(char))
		}
	}
}

// padVisual pads a string with spaces up to the target visual width, ignoring ANSI escape codes.
func padVisual(str string, length int) string {
	re := regexp.MustCompile(`\033\[[0-9;]*[a-zA-Z]`)
	plain := re.ReplaceAllString(str, "")
	visualLen := runewidth.StringWidth(plain)
	if visualLen >= length {
		return str
	}
	return str + strings.Repeat(" ", length-visualLen)
}

func getBoxWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = 80
	}
	boxW := width - 4
	if boxW > 80 {
		boxW = 80
	}
	if boxW < 40 {
		boxW = 40
	}
	return boxW
}

// drawBoxLine creates a simple filled box line.
func drawBoxLine(left, fill string, length int, right string) string {
	return left + strings.Repeat(fill, length) + right
}

// drawBoxHeader creates a box line with centered header text.
func drawBoxHeader(left, fill string, text string, length int, right string) string {
	fillCount := length - len(text)
	if fillCount < 0 {
		fillCount = 0
	}
	leftFill := fillCount / 2
	rightFill := fillCount - leftFill
	return left + strings.Repeat(fill, leftFill) + text + strings.Repeat(fill, rightFill) + right
}

// drawHelpBox renders the commands and utilities help box.
func drawHelpBox() {
	borderCol := "\033[38;5;244m" // Gray
	resetCol := "\033[0m"
	width := getBoxWidth()
	fmt.Println(borderCol + drawBoxHeader("┌", "─", " COMMANDS & UTILITIES ", width, "┐") + resetCol)

	commandsList := []struct {
		cmd  string
		desc string
	}{
		{"/search", "Toggles active web search"},
		{"/deep or /research", "Toggles deep research (exhaustive search)"},
		{"/think", "Toggles model reasoning/thinking mode"},
		{"/model", "Interactively select Gemini model"},
		{"/youtube <term>", "Opens search terms directly in browser"},
		{"/swarm <goal>", "Executes parallel agents swarm on a task"},
		{"/config", "Reconfigures your Gemini API key"},
		{"/clear", "Clears console screen & resets chat history"},
		{"/exit or /quit", "Exits the application"},
	}

	for _, item := range commandsList {
		line := fmt.Sprintf("  \033[1;34m%-18s\033[0m - %s", item.cmd, item.desc)
		fmt.Printf("%s│%s%s%s│%s\n", borderCol, resetCol, padVisual(line, width), borderCol, resetCol)
	}
	fmt.Println(borderCol + drawBoxLine("└", "─", width, "┘") + resetCol)
}

// updateStateAndRedraw clears the typed command on the current line and updates the active state box.
func updateStateAndRedraw(cfg *Config, isWelcomeScreen bool, thinkMode, activeSearch, deepResearch bool, promptFn func() string) {
	// Move up 1 line to the prompt line where the command was typed, and clear it
	fmt.Print("\033[1A\r\033[K")



	if isWelcomeScreen {
		// Move up 17 lines to the start of the configuration box
		fmt.Print("\033[17A\r")
		// Redraw active state box (which prints 4 lines, leaving cursor 13 lines above prompt)
		drawActiveStateBox(cfg, thinkMode, activeSearch, deepResearch)
		// Move down 13 lines to return to prompt line
		fmt.Print("\033[13B\r")
	}

	// Move up 1 line to compensate for the fmt.Println() at the top of the REPL loop
	fmt.Print("\033[1A")
}

// drawActiveStateBox draws only the active configuration and mode state box.
func drawActiveStateBox(cfg *Config, thinkMode, activeSearch, deepResearch bool) {
	borderCol := "\033[38;5;129m" // Magenta/Purple
	resetCol := "\033[0m"
	width := getBoxWidth()

	thinkStatus := "\033[1;31m[OFF]\033[0m"
	if thinkMode {
		thinkStatus = "\033[1;32m[ON]\033[0m"
	}
	searchStatus := "\033[1;31m[OFF]\033[0m"
	if activeSearch {
		searchStatus = "\033[1;32m[ON]\033[0m"
	}
	deepStatus := "\033[1;31m[OFF]\033[0m"
	if deepResearch {
		deepStatus = "\033[1;32m[ON]\033[0m"
	}

	modelName := ModelFriendlyNames[cfg.DefaultModel]
	if modelName == "" {
		modelName = cfg.DefaultModel
	}

	fmt.Println(borderCol + drawBoxHeader("╔", "═", " CONFIGURATION & ACTIVE STATE ", width, "╗") + resetCol)
	modelLine := fmt.Sprintf("  Model: \033[1;36m%s\033[0m", modelName)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(modelLine, width), borderCol, resetCol)

	modesLine := fmt.Sprintf("  Modes: Thinking: %s   Search: %s   Deep Research: %s", thinkStatus, searchStatus, deepStatus)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(modesLine, width), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", width, "╝") + resetCol)
}

// drawWelcomeScreen draws the ASCII logo, active settings box, and help box.
func drawWelcomeScreen(cfg *Config, thinkMode, activeSearch, deepResearch bool) {
	fmt.Println()
	// ASCII Art banner with a sleek magenta-to-cyan theme
	fmt.Println("  \033[1;35m    ____       _               \033[0m")
	fmt.Println("  \033[1;35m   / __ \\____(_)____ ____ _____ \033[0m")
	fmt.Println("  \033[1;36m  / /_/ / ___/ / ___// __ `__  /\033[0m")
	fmt.Println("  \033[1;36m / ____/ /  / (__  )/ / / / / /\033[0m")
	fmt.Println("  \033[1;34m/_/   /_/  /_/____//_/ /_/ /_/  \033[1;30mv" + Version + "\033[0m")
	fmt.Println()

	// Configuration & Active State Box
	drawActiveStateBox(cfg, thinkMode, activeSearch, deepResearch)
	fmt.Println()

	// Help box
	drawHelpBox()
}






