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

// RunSetupWizard prompts the user for their Gemini API Key in the terminal.
func RunSetupWizard(cfg *Config) error {
	borderCol := "\033[38;5;129m" // Magenta/Purple
	resetCol := "\033[0m"
	
	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("╔", "═", " PRISM SETUP WIZARD ", 60, "╗") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  No Gemini API key was found or reconfiguration requested.", 60), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  This key is required to connect to the AI models.", 60), borderCol, resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual("  The key will be securely encrypted on your machine.", 60), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╚", "═", 60, "╝") + resetCol)
	fmt.Println()

	fmt.Printf("\033[33m🔑 Please enter your Gemini API Key:\033[0m ")
	
	key, err := readPasswordWindows()
	if err != nil {
		reader := bufio.NewReader(os.Stdin)
		k, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		key = strings.TrimSpace(k)
	}

	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	err = cfg.SetAPIKey(key)
	if err != nil {
		return fmt.Errorf("error saving API key: %v", err)
	}

	fmt.Println()
	fmt.Println("\033[1;32m✔ API Key saved successfully via Windows DPAPI!\033[0m")
	fmt.Println()
	return nil
}

// readPasswordWindows reads console input without echoing it
func readPasswordWindows() (string, error) {
	// Standard Windows syscall to disable echoing
	stdin := windows.Handle(os.Stdin.Fd())
	var mode uint32
	err := windows.GetConsoleMode(stdin, &mode)
	if err != nil {
		return "", err
	}

	// Disable echo and line input
	// ENABLE_ECHO_INPUT = 0x0004
	err = windows.SetConsoleMode(stdin, mode &^ 0x0004)
	if err != nil {
		return "", err
	}
	defer windows.SetConsoleMode(stdin, mode)

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(line), nil
}
