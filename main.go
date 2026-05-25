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

	"golang.org/x/sys/windows"
)

const Version = "0.1.0"

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
	fmt.Println("==================================================")
	fmt.Printf("              PRISM CLI - v%s\n", Version)
	fmt.Println("==================================================")
	fmt.Println("Type your messages. Supported commands:")
	fmt.Println("  /search           - Toggles active web search")
	fmt.Println("  /deep or /research - Toggles deep research")
	fmt.Println("  /youtube <term>   - Opens video directly in browser")
	fmt.Println("  /swarm <goal>     - Executes agents in swarm")
	fmt.Println("  /config           - Reconfigures the API key")
	fmt.Println("  /exit or /quit    - Exits the program")
	fmt.Println("==================================================")

	sessionID := fmt.Sprintf("chat_%d", time.Now().Unix())
	session := ChatSession{
		ID:       sessionID,
		Title:    "Quick Chat",
		Messages: make([]ChatMessage, 0),
	}

	activeSearch := startSearch
	deepResearch := startDeep
	thinkMode := true // Think Mode default enabled for reasoning models

	promptFn := func() string {
		promptSymbol := "\033[1;34mPrism >\033[0m "
		if thinkMode {
			if deepResearch {
				promptSymbol = "\033[1;36mPrism (Deep Research + Thinking) >\033[0m "
			} else if activeSearch {
				promptSymbol = "\033[1;32mPrism (Search + Thinking) >\033[0m "
			} else {
				promptSymbol = "\033[1;33mPrism (Thinking) >\033[0m "
			}
		} else {
			if deepResearch {
				promptSymbol = "\033[1;36mPrism (Deep Research) >\033[0m "
			} else if activeSearch {
				promptSymbol = "\033[1;32mPrism (Search) >\033[0m "
			}
		}
		return promptSymbol
	}

	for {
		input, err := ReadLine(cfg, promptFn, &thinkMode, &activeSearch, &deepResearch)
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
					fmt.Println("✓ Active Search enabled.")
				} else {
					fmt.Println("✗ Active Search disabled.")
				}
				continue
			case "/deep", "/research":
				deepResearch = !deepResearch
				if deepResearch {
					activeSearch = false
					fmt.Println("✓ Extended Search (Deep Research) enabled.")
				} else {
					fmt.Println("✗ Extended Search disabled.")
				}
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
				if thinkMode {
					fmt.Println("✓ Thinking Mode enabled.")
				} else {
					fmt.Println("✗ Thinking Mode disabled.")
				}
				continue
			case "/model":
				if arg == "" {
					fmt.Println("\n=== AVAILABLE MODELS ===")
					for i, m := range SelectableModels {
						friendlyName := ModelFriendlyNames[m]
						active := ""
						if m == cfg.DefaultModel {
							active = " (Active)"
						}
						fmt.Printf(" [%d] %s%s\n", i+1, friendlyName, active)
					}
					fmt.Println("\nTo change the model, use: /model <number or name>")
					fmt.Println("Examples: /model 1   or   /model Prism 5")
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
					fmt.Printf("✓ Default model changed to: %s\n", ModelFriendlyNames[selectedModel])
				} else {
					fmt.Printf("✗ Model '%s' not found. Type '/model' without parameters to see the list.\n", arg)
				}
				continue
			case "/config":
				err := RunSetupWizard(cfg)
				if err != nil {
					fmt.Printf("Configuration error: %v\n", err)
				}
				continue
			case "/help":
				fmt.Println("Available commands:")
				fmt.Println("  /search           - Toggles active web search")
				fmt.Println("  /deep or /research - Toggles deep research")
				fmt.Println("  /think            - Toggles thinking mode")
				fmt.Println("  /model [option]    - Displays or changes the active AI model")
				fmt.Println("  /youtube <term>   - Opens video directly in browser")
				fmt.Println("  /swarm <goal>     - Executes agents in swarm")
				fmt.Println("  /config           - Reconfigures the API key")
				fmt.Println("  /exit             - Exits the program")
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
			fmt.Printf("Erro ao obter resposta da IA: %v\n", err)
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
	WVirtualScanCode   uint16
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
	fmt.Println("\033[1;35m=== SELECT AI MODEL ===\033[0m")
	for i, modelID := range SelectableModels {
		friendlyName := ModelFriendlyNames[modelID]
		marker := "[ ]"
		if modelID == activeModel {
			marker = "\033[32m[➔]\033[0m"
		}
		
		if i == selectedIndex {
			fmt.Printf(" \033[1;36m➔ %s %s\033[0m\n", marker, friendlyName)
		} else {
			fmt.Printf("   %s %s\n", marker, friendlyName)
		}
	}
	fmt.Println("\033[90m(Use ↑/↓ to navigate, Enter to confirm, Esc to cancel)\033[0m")
}

func clearLines(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[A\r\033[K")
	}
}

func runModelSelectionMenu(cfg *Config, promptFn func() string, currentLine []rune) {
	selectedIndex := 0
	for idx, m := range SelectableModels {
		if m == cfg.DefaultModel {
			selectedIndex = idx
			break
		}
	}

	stdin := windows.Handle(os.Stdin.Fd())
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
			clearLines(2 + len(SelectableModels))
			drawModelMenu(selectedIndex, cfg.DefaultModel)

		case 0x28: // VK_DOWN
			selectedIndex++
			if selectedIndex >= len(SelectableModels) {
				selectedIndex = 0
			}
			clearLines(2 + len(SelectableModels))
			drawModelMenu(selectedIndex, cfg.DefaultModel)

		case 0x0D: // VK_RETURN
			cfg.DefaultModel = SelectableModels[selectedIndex]
			_ = SaveConfig(*cfg)
			clearLines(2 + len(SelectableModels))
			fmt.Printf("%s%s", promptFn(), string(currentLine))
			return

		case 0x1B: // VK_ESCAPE
			clearLines(2 + len(SelectableModels))
			fmt.Printf("%s%s", promptFn(), string(currentLine))
			return
		}
	}
}

// ReadLine reads input character by character from the console on Windows in raw mode,
// allowing keyboard shortcuts like Ctrl+T and Ctrl+M to toggle modes and choose models.
func ReadLine(cfg *Config, promptFn func() string, thinkMode *bool, activeSearch *bool, deepResearch *bool) (string, error) {
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
			fmt.Print("\r\033[K")
			fmt.Printf("%s%s", promptFn(), string(line))
			continue
		}

		// 2. Detect Ctrl+M (also fallback to ASCII control code 13, distinguishing from Enter using the Virtual Key Code)
		if (hasCtrl && (vk == 'M' || vk == 'm')) || (char == 13 && (vk == 'M' || vk == 'm')) {
			runModelSelectionMenu(cfg, promptFn, line)
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
