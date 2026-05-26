package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dllcrypt32  = syscall.NewLazyDLL("crypt32.dll")
	procEncrypt = dllcrypt32.NewProc("CryptProtectData")
	procDecrypt = dllcrypt32.NewProc("CryptUnprotectData")
)

type DATA_BLOB struct {
	cbData uint32
	pbData *byte
}

// Config represents the application settings.
type Config struct {
	LauncherShortcut       string `json:"launcherShortcut"`
	ModelSelectionShortcut string `json:"modelSelectionShortcut"`
	DefaultModel           string `json:"defaultModel"`
	SubagentModel          string `json:"subagentModel"`
	MinimizeToTray         bool   `json:"minimizeToTray"`
	AutoLaunch             bool   `json:"autoLaunch"`
	QuickLauncherMode      string `json:"quickLauncherMode"`
	UserGeminiKey          string `json:"userGeminiKey"` // Hex encoded DPAPI encrypted API key
	Username               string `json:"username"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		LauncherShortcut:       "Ctrl+Space",
		ModelSelectionShortcut: "Ctrl+M",
		DefaultModel:           "gemini-3.5-flash", // Prism 5
		SubagentModel:          "gemma-4-26b-a4b-it", // Prism 4.2
		MinimizeToTray:         false,
		AutoLaunch:             false,
		QuickLauncherMode:      "simple",
		UserGeminiKey:          "",
		Username:               "",
	}
}

// GetConfigDir returns the OS specific AppData directory for PrismCLI
func GetConfigDir() (string, error) {
	appData, err := os.UserConfigDir()
	if err != nil {
		appData = os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("unable to determine User Config Directory")
		}
	}
	dir := filepath.Join(appData, "PrismCLI")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// GetConfigPath returns the path to the config file.
func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig loads the configuration from disk.
func LoadConfig() (Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return DefaultConfig(), err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Save default config
			cfg := DefaultConfig()
			_ = SaveConfig(cfg)
			return cfg, nil
		}
		return DefaultConfig(), err
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return DefaultConfig(), err
	}
	return cfg, nil
}

// SaveConfig saves the configuration to disk.
func SaveConfig(cfg Config) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(cfg)
}

// DPAPI Encryption
func encrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var inBlob, outBlob DATA_BLOB
	inBlob.cbData = uint32(len(data))
	inBlob.pbData = &data[0]

	r1, _, err := procEncrypt.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r1 == 0 {
		return nil, err
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(outBlob.pbData)))

	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

// DPAPI Decryption
func decrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var inBlob, outBlob DATA_BLOB
	inBlob.cbData = uint32(len(data))
	inBlob.pbData = &data[0]

	r1, _, err := procDecrypt.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r1 == 0 {
		return nil, err
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(outBlob.pbData)))

	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

// GetAPIKey returns the decrypted API Key.
func (cfg *Config) GetAPIKey() (string, error) {
	if cfg.UserGeminiKey == "" {
		return "", fmt.Errorf("no API key set")
	}

	encryptedBytes, err := hex.DecodeString(cfg.UserGeminiKey)
	if err != nil {
		return "", err
	}

	decryptedBytes, err := decrypt(encryptedBytes)
	if err != nil {
		return "", err
	}

	return string(decryptedBytes), nil
}

// SetAPIKey encrypts and stores the API Key.
func (cfg *Config) SetAPIKey(key string) error {
	encryptedBytes, err := encrypt([]byte(key))
	if err != nil {
		return err
	}

	cfg.UserGeminiKey = hex.EncodeToString(encryptedBytes)
	return SaveConfig(*cfg)
}

// RunSetupWizard prompts the user for their Gemini API Key in the terminal, showing the current key pre-filled.
func RunSetupWizard(cfg *Config) error {
	borderCol := "\033[38;5;129m" // Magenta/Purple
	resetCol := "\033[0m"
	
	currentKey, _ := cfg.GetAPIKey()
	defaultValue := currentKey

	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("╔", "═", " PRISM SETUP WIZARD ", 70, "╗") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  No Gemini API key was found or reconfiguration requested.", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  This key is required to connect to the AI models.", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  The key will be securely encrypted on your machine.", 70), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("", 70), borderCol, resetCol)
	
	displayKey := currentKey
	if displayKey == "" {
		displayKey = "None"
	}
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(fmt.Sprintf("  Current API Key: %s", displayKey), 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
	fmt.Println()

	fmt.Printf("\033[33m🔑 Please enter your Gemini API Key:\033[0m ")
	
	key, err := ReadLineWithDefault(defaultValue)
	if err != nil {
		return err
	}

	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	err = cfg.SetAPIKey(key)
	if err != nil {
		return fmt.Errorf("error saving API key: %v", err)
	}

	fmt.Println()
	fmt.Printf("\033[1;32m✔ API Key saved successfully via Windows DPAPI: \033[1;33m%s\033[0m\n", key)
	fmt.Println()
	return nil
}

// ReadLineWithDefault reads raw input character-by-character on Windows, pre-filling with a default value.
func ReadLineWithDefault(defaultValue string) (string, error) {
	stdin := windows.Handle(os.Stdin.Fd())
	var mode uint32
	isConsole := windows.GetConsoleMode(stdin, &mode) == nil

	if !isConsole {
		fmt.Print(defaultValue)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			return defaultValue, nil
		}
		return trimmed, nil
	}

	originalMode := mode
	rawMode := originalMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
	_ = windows.SetConsoleMode(stdin, rawMode)
	defer windows.SetConsoleMode(stdin, originalMode)

	var line []rune
	if defaultValue != "" {
		line = []rune(defaultValue)
		fmt.Print(defaultValue)
	}

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

		if vk == 0x0D { // VK_RETURN
			fmt.Println()
			return string(line), nil
		}

		if vk == 0x08 { // VK_BACK
			if len(line) > 0 {
				line = line[:len(line)-1]
				fmt.Print("\b \b")
			}
			continue
		}

		if vk == 0x1B { // VK_ESCAPE
			for i := 0; i < len(line); i++ {
				fmt.Print("\b \b")
			}
			fmt.Print(defaultValue)
			return defaultValue, nil
		}

		if char >= 32 {
			line = append(line, char)
			fmt.Print(string(char))
		}
	}
}
