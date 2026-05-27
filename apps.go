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
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path"`
}

type rawAppInfo struct {
	name            string
	version         string
	displayIcon     string
	installLocation string
}

var (
	appCache      []AppInfo
	appCacheTime  time.Time
	cacheMutex    sync.Mutex
	cacheDuration = 5 * time.Minute
)

// ScanApplications returns a list of installed applications on the system.
// It checks the registry (32/64-bit, user/machine) and performs manual scans
// of common Windows program directories, resolving paths to main executables.
// The results are cached for 5 minutes.
func ScanApplications() []AppInfo {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	if len(appCache) > 0 && time.Since(appCacheTime) < cacheDuration {
		return appCache
	}

	var rawApps []rawAppInfo

	// 1. Scan Windows Registry (Machine and User, 32 and 64-bit)
	rawApps = append(rawApps, scanRegistry(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`)...)
	rawApps = append(rawApps, scanRegistry(registry.LOCAL_MACHINE, `SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`)...)
	rawApps = append(rawApps, scanRegistry(registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`)...)
	rawApps = append(rawApps, scanRegistry(registry.CURRENT_USER, `SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`)...)

	// 2. Manual scan of common Windows folders to find unregistered apps/shortcuts
	var manualApps []rawAppInfo
	for _, dir := range getCommonPaths() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			entryPath := filepath.Join(dir, entry.Name())
			nameLower := strings.ToLower(entry.Name())
			if entry.IsDir() {
				manualApps = append(manualApps, rawAppInfo{
					name:            entry.Name(),
					installLocation: entryPath,
				})
			} else if strings.HasSuffix(nameLower, ".lnk") || strings.HasSuffix(nameLower, ".exe") {
				name := entry.Name()
				if idx := strings.LastIndex(nameLower, "."); idx != -1 {
					name = name[:idx]
				}
				manualApps = append(manualApps, rawAppInfo{
					name:        name,
					displayIcon: entryPath,
				})
			}
		}
	}

	// 3. Merge, resolve actual executable paths, and deduplicate by lowercase name
	allApps := append(rawApps, manualApps...)
	seenNames := make(map[string]bool)
	var apps []AppInfo

	for _, app := range allApps {
		if app.name == "" {
			continue
		}
		nameLower := strings.ToLower(app.name)
		if seenNames[nameLower] {
			continue
		}

		exePath := resolveAppExecutable(app.displayIcon, app.installLocation)
		if exePath != "" {
			seenNames[nameLower] = true
			apps = append(apps, AppInfo{
				Name:    app.name,
				Version: app.version,
				Path:    exePath,
			})
		}
	}

	appCache = apps
	appCacheTime = time.Now()
	return apps
}

// scanRegistry queries display name, version, location, and icon from Windows Registry
func scanRegistry(rootKey registry.Key, path string) []rawAppInfo {
	var result []rawAppInfo
	k, err := registry.OpenKey(rootKey, path, registry.READ)
	if err != nil {
		return nil
	}
	defer k.Close()

	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}

	for _, skName := range subkeys {
		sk, err := registry.OpenKey(k, skName, registry.READ)
		if err != nil {
			continue
		}

		displayName, _, _ := sk.GetStringValue("DisplayName")
		displayVersion, _, _ := sk.GetStringValue("DisplayVersion")
		displayIcon, _, _ := sk.GetStringValue("DisplayIcon")
		installLocation, _, _ := sk.GetStringValue("InstallLocation")

		sk.Close()

		if displayName == "" {
			continue
		}

		result = append(result, rawAppInfo{
			name:            displayName,
			version:         displayVersion,
			displayIcon:     displayIcon,
			installLocation: installLocation,
		})
	}
	return result
}

// getCommonPaths returns a slice of paths for scanning typical software installation folders
func getCommonPaths() []string {
	var paths []string

	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	paths = append(paths, pf)

	pf86 := os.Getenv("ProgramFiles(x86)")
	if pf86 == "" {
		pf86 = `C:\Program Files (x86)`
	}
	paths = append(paths, pf86)

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		paths = append(paths, filepath.Join(localAppData, "Programs"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, `AppData\Local\Programs`))
	}

	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	paths = append(paths, filepath.Join(pd, `Microsoft\Windows\Start Menu\Programs`))

	appData := os.Getenv("APPDATA")
	if appData != "" {
		paths = append(paths, filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs`))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, `AppData\Roaming\Microsoft\Windows\Start Menu\Programs`))
	}

	return paths
}

// findMainExecutable searches a folder recursively up to depth 4 for a primary executable (.exe)
func findMainExecutable(folderPath string, depth int) (string, error) {
	if depth > 4 {
		return "", fmt.Errorf("maximum depth exceeded")
	}
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return "", err
	}

	var exeFiles []string
	var subDirs []string

	for _, entry := range entries {
		fullPath := filepath.Join(folderPath, entry.Name())
		nameLower := strings.ToLower(entry.Name())

		if !entry.IsDir() && strings.HasSuffix(nameLower, ".exe") {
			if !strings.Contains(nameLower, "unins") &&
				!strings.Contains(nameLower, "uninstall") &&
				!strings.Contains(nameLower, "setup") &&
				!strings.Contains(nameLower, "helper") &&
				!strings.Contains(nameLower, "crash") &&
				!strings.Contains(nameLower, "update") &&
				!strings.Contains(nameLower, "elevate") {
				exeFiles = append(exeFiles, fullPath)
			}
		} else if entry.IsDir() && nameLower != "node_modules" && !strings.HasPrefix(nameLower, ".") {
			subDirs = append(subDirs, fullPath)
		}
	}

	if len(exeFiles) > 0 {
		folderNameLower := strings.ToLower(filepath.Base(folderPath))
		bestMatch := ""
		for _, exe := range exeFiles {
			exeNameLower := strings.ToLower(filepath.Base(exe))
			if strings.Contains(exeNameLower, folderNameLower) ||
				strings.Contains(folderNameLower, strings.TrimSuffix(exeNameLower, ".exe")) {
				bestMatch = exe
				break
			}
		}
		if bestMatch != "" {
			return bestMatch, nil
		}
		return exeFiles[0], nil
	}

	for _, subDir := range subDirs {
		exe, err := findMainExecutable(subDir, depth+1)
		if err == nil && exe != "" {
			return exe, nil
		}
	}

	return "", fmt.Errorf("no main executable found in folder: %s", folderPath)
}

// getExecutablePath resolves shortcuts or directories to a literal .exe path
func getExecutablePath(appPath string) (string, error) {
	if appPath == "" {
		return "", fmt.Errorf("empty path")
	}
	stats, err := os.Stat(appPath)
	if err == nil {
		if !stats.IsDir() {
			if strings.HasSuffix(strings.ToLower(appPath), ".lnk") {
				target, err := resolveShortcutHeuristic(appPath)
				if err == nil && target != "" {
					return getExecutablePath(target)
				}
			}
			if strings.HasSuffix(strings.ToLower(appPath), ".exe") {
				return appPath, nil
			}
			return "", fmt.Errorf("not an executable")
		} else {
			return findMainExecutable(appPath, 0)
		}
	} else {
		if strings.HasSuffix(strings.ToLower(appPath), ".exe") {
			return appPath, nil
		}
	}
	return "", fmt.Errorf("could not resolve path: %s", appPath)
}

// cleanDisplayIcon removes surrounding quotes and any trailing icon indexes (e.g. "path.exe,0")
func cleanDisplayIcon(iconPath string) string {
	cleaned := strings.TrimSpace(iconPath)
	if strings.HasPrefix(cleaned, `"`) && strings.HasSuffix(cleaned, `"`) {
		cleaned = cleaned[1 : len(cleaned)-1]
	}
	commaIndex := strings.LastIndex(cleaned, ",")
	if commaIndex != -1 {
		suffix := strings.TrimSpace(cleaned[commaIndex+1:])
		isNum := true
		if len(suffix) > 0 {
			start := 0
			if suffix[0] == '-' {
				start = 1
			}
			if start == len(suffix) {
				isNum = false
			}
			for i := start; i < len(suffix); i++ {
				if suffix[i] < '0' || suffix[i] > '9' {
					isNum = false
					break
				}
			}
		} else {
			isNum = false
		}
		if isNum {
			cleaned = strings.TrimSpace(cleaned[:commaIndex])
		}
	}
	return cleaned
}

// resolveAppExecutable tries multiple techniques to find a valid main executable path
func resolveAppExecutable(displayIcon string, installLocation string) string {
	isExe := func(p string) bool { return strings.HasSuffix(strings.ToLower(p), ".exe") }
	isLnk := func(p string) bool { return strings.HasSuffix(strings.ToLower(p), ".lnk") }
	isUninstaller := func(p string) bool {
		low := strings.ToLower(p)
		return strings.Contains(low, "unins") || strings.Contains(low, "uninstall") || strings.Contains(low, "setup")
	}

	// Try 1: DisplayIcon direct resolution if it ends with .exe or .lnk
	if displayIcon != "" {
		cleanedIcon := cleanDisplayIcon(displayIcon)
		if cleanedIcon != "" {
			if isLnk(cleanedIcon) || isExe(cleanedIcon) {
				resolved, err := getExecutablePath(cleanedIcon)
				if err == nil && resolved != "" {
					if !isUninstaller(resolved) {
						return resolved
					} else {
						// It's an uninstaller (e.g. Steam). Search parent folder for a main executable.
						parentDir := filepath.Dir(resolved)
						mainExe, err := findMainExecutable(parentDir, 0)
						if err == nil && mainExe != "" {
							return mainExe
						}
					}
				}
			}
		}
	}

	// Try 2: InstallLocation directory lookup
	if installLocation != "" {
		stats, err := os.Stat(installLocation)
		if err == nil && stats.IsDir() {
			mainExe, err := findMainExecutable(installLocation, 0)
			if err == nil && mainExe != "" {
				return mainExe
			}
		}
	}

	// Try 3: Fallback directory lookup if DisplayIcon is a file but not a .exe (e.g., .ico)
	if displayIcon != "" {
		cleanedIcon := cleanDisplayIcon(displayIcon)
		if cleanedIcon != "" {
			stats, err := os.Stat(cleanedIcon)
			if err == nil && !stats.IsDir() {
				parentDir := filepath.Dir(cleanedIcon)
				parentLower := strings.ToLower(parentDir)
				// Avoid scanning generic Steam games or Riot Games metadata folders
				if !strings.Contains(parentLower, `steam\steam\games`) &&
					!strings.Contains(parentLower, `riot games\metadata`) {
					mainExe, err := findMainExecutable(parentDir, 0)
					if err == nil && mainExe != "" {
						return mainExe
					}
				}
			}
		}
	}

	return ""
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
// It launches the process in a detached state after resolving the path first.
func LaunchApplication(appPath string) error {
	resolved, err := getExecutablePath(appPath)
	if err == nil && resolved != "" {
		appPath = resolved
	}
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
