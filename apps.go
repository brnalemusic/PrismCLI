package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

// AppInfo represents an installed application.
type AppInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

var (
	appCache      []AppInfo
	appCacheTime  time.Time
	cacheMutex    sync.Mutex
	cacheDuration = 5 * time.Minute
)

// ScanApplications returns a list of installed applications on the system.
// It checks the registry (32/64-bit, user/machine) and Start Menu shortcuts.
// The results are cached for 5 minutes.
func ScanApplications() []AppInfo {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if len(appCache) > 0 && time.Since(appCacheTime) < cacheDuration {
		return appCache
	}

	appsMap := make(map[string]string) // Name -> Path mapping to deduplicate

	// 1. Scan Windows Registry
	scanRegistry(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, appsMap)
	scanRegistry(registry.LOCAL_MACHINE, `SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, appsMap)
	scanRegistry(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, appsMap)

	// 2. Scan Common Start Menu and User Start Menu folders
	programData := os.Getenv("ProgramData")
	if programData != "" {
		scanDirectoryForShortcuts(filepath.Join(programData, `Microsoft\Windows\Start Menu\Programs`), appsMap)
	}

	appData := os.Getenv("APPDATA")
	if appData != "" {
		scanDirectoryForShortcuts(filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs`), appsMap)
	}

	// 3. Convert map to slice and apply filtering heuristics
	var apps []AppInfo
	for name, path := range appsMap {
		if path == "" {
			continue
		}

		// Filter out non-executables
		if !strings.HasSuffix(strings.ToLower(path), ".exe") {
			continue
		}

		// Anti-Uninstaller and utilities heuristics
		pathLower := strings.ToLower(path)
		nameLower := strings.ToLower(name)
		isUtility := false
		ignoredKeywords := []string{
			"uninstall", "unins000", "unins", "helper", "installer", "setup",
			"crashreporter", "updater", "update", "elevate", "diagnostics",
			"regist", "remove", "troubleshoot",
		}
		for _, kw := range ignoredKeywords {
			if strings.Contains(pathLower, kw) || strings.Contains(nameLower, kw) {
				isUtility = true
				break
			}
		}

		if isUtility {
			continue
		}

		apps = append(apps, AppInfo{
			Name: name,
			Path: path,
		})
	}

	appCache = apps
	appCacheTime = time.Now()
	return apps
}

func scanRegistry(rootKey registry.Key, path string, appsMap map[string]string) {
	k, err := registry.OpenKey(rootKey, path, registry.READ)
	if err != nil {
		return
	}
	defer k.Close()

	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return
	}

	for _, skName := range subkeys {
		sk, err := registry.OpenKey(k, skName, registry.READ)
		if err != nil {
			continue
		}

		displayName, _, _ := sk.GetStringValue("DisplayName")
		displayIcon, _, _ := sk.GetStringValue("DisplayIcon")
		installLocation, _, _ := sk.GetStringValue("InstallLocation")

		sk.Close()

		if displayName == "" {
			continue
		}

		// Try to resolve path
		appPath := ""
		if displayIcon != "" {
			// Extract path from icon string (often contains comma and index, like "path.exe,0")
			if idx := strings.Index(displayIcon, ","); idx != -1 {
				appPath = strings.Trim(displayIcon[:idx], `"` )
			} else {
				appPath = strings.Trim(displayIcon, `"` )
			}
		}

		if (appPath == "" || !strings.HasSuffix(strings.ToLower(appPath), ".exe")) && installLocation != "" {
			// Search for exe in install location
			files, err := os.ReadDir(installLocation)
			if err == nil {
				for _, f := range files {
					if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".exe") {
						// Heuristic: check if name matches display name or is main
						if strings.Contains(strings.ToLower(f.Name()), "main") ||
							strings.Contains(strings.ToLower(displayName), strings.ToLower(f.Name()[:len(f.Name())-4])) {
							appPath = filepath.Join(installLocation, f.Name())
							break
						}
					}
				}
				// Default to first exe found if no match
				if appPath == "" {
					for _, f := range files {
						if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".exe") {
							appPath = filepath.Join(installLocation, f.Name())
							break
						}
					}
				}
			}
		}

		if appPath != "" {
			appsMap[displayName] = appPath
		}
	}
}

func scanDirectoryForShortcuts(dir string, appsMap map[string]string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".lnk") {
			target, err := resolveShortcutHeuristic(path)
			if err == nil && target != "" {
				name := info.Name()
				name = name[:len(name)-4] // remove .lnk
				appsMap[name] = target
			}
		}
		return nil
	})
}

// resolveShortcutHeuristic reads a .lnk file and extracts the executable target path
// using rapid binary string heuristics for C:\...\*.exe in ASCII and UTF-16LE
func resolveShortcutHeuristic(lnkPath string) (string, error) {
	data, err := os.ReadFile(lnkPath)
	if err != nil {
		return "", err
	}

	// 1. Scan for UTF-16LE paths starting with "C:\" (or other letters)
	// Look for: [Drive Letter] : \
	// In UTF-16LE, "C:\" is `C\x00:\x00\\\x00`
	for i := 0; i < len(data)-10; i++ {
		// Check for drive letter followed by : and \
		if isDriveLetter(data[i]) && data[i+1] == 0 &&
			data[i+2] == ':' && data[i+3] == 0 &&
			data[i+4] == '\\' && data[i+5] == 0 {
			// Extract UTF-16 string until null or non-printable character
			var u16 []uint16
			j := i
			for j < len(data)-1 {
				val := uint16(data[j]) | (uint16(data[j+1]) << 8)
				if val == 0 || val < 32 || val > 126 {
					break
				}
				u16 = append(u16, val)
				j += 2
			}
			pathStr := string(utf16.Decode(u16))
			if strings.HasSuffix(strings.ToLower(pathStr), ".exe") {
				// Verify path exists
				if _, err := os.Stat(pathStr); err == nil {
					return pathStr, nil
				}
			}
		}
	}

	// 2. Scan for ASCII paths
	for i := 0; i < len(data)-5; i++ {
		if isDriveLetter(data[i]) && data[i+1] == ':' && data[i+2] == '\\' {
			j := i
			for j < len(data) && data[j] >= 32 && data[j] <= 126 {
				j++
			}
			pathStr := string(data[i:j])
			if strings.HasSuffix(strings.ToLower(pathStr), ".exe") {
				if _, err := os.Stat(pathStr); err == nil {
					return pathStr, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not resolve shortcut target via heuristics")
}

func isDriveLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// LaunchApplication opens an application given its executable path.
// It launches the process in a detached state.
func LaunchApplication(appPath string) error {
	cmd := exec.Command("cmd.exe", "/c", "start", "", appPath)
	return cmd.Start()
}

// OpenBrowserLink opens a URL in the default system browser.
func OpenBrowserLink(urlStr string) error {
	cmd := exec.Command("cmd.exe", "/c", "start", "", urlStr)
	return cmd.Start()
}

// OpenMainApp launches the main Prism Electron application.
func OpenMainApp() error {
	path := `c:\Users\brnalemusic\Documents\Code\Prism\dist\win-unpacked\Prism.exe`
	if _, err := os.Stat(path); err != nil {
		// Fallback to relative path from PrismCLI
		path = `..\Prism\dist\win-unpacked\Prism.exe`
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("Prism application not found in the default build path dist/win-unpacked")
		}
	}
	cmd := exec.Command("cmd.exe", "/c", "start", "", path)
	return cmd.Start()
}

