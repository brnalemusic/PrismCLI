package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

// GitHubRelease represents the structure of GitHub's release response.
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int    `json:"size"`
	} `json:"assets"`
}

// CompareVersions compares two version strings (e.g., "0.2.0" and "0.1.1").
// Returns 1 if v1 > v2, -1 if v1 < v2, and 0 if they are equal.
func CompareVersions(v1, v2 string) int {
	v1 = strings.TrimLeft(v1, "vV ")
	v2 = strings.TrimLeft(v2, "vV ")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < 3; i++ {
		var p1, p2 string
		if i < len(parts1) {
			p1 = parts1[i]
		} else {
			p1 = "0"
		}
		if i < len(parts2) {
			p2 = parts2[i]
		} else {
			p2 = "0"
		}

		var n1, n2 int
		_, _ = fmt.Sscanf(p1, "%d", &n1)
		_, _ = fmt.Sscanf(p2, "%d", &n2)

		if n1 > n2 {
			return 1
		} else if n1 < n2 {
			return -1
		}
	}
	return 0
}

// cleanupOldVersions removes any old backup binary left over from previous updates.
func cleanupOldVersions() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	bakPath := exePath + ".old"
	if _, err := os.Stat(bakPath); err == nil {
		_ = os.Remove(bakPath)
	}
}

// CheckAndPerformUpdate checks GitHub for a newer release and updates if the user accepts.
func CheckAndPerformUpdate(cfg *Config) {
	// First, try cleaning up any old updates
	cleanupOldVersions()

	// Check latest release from GitHub (with 3-second timeout so it doesn't block slow startups)
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://api.github.com/repos/brnalemusic/PrismCLI/releases/latest", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "PrismCLI-Updater/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return // Silent return on connection issues
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return // Silent return on API limits/errors
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}

	latestVersion := release.TagName
	if CompareVersions(latestVersion, Version) <= 0 {
		return // We are on the latest version or newer (e.g. dev version)
	}

	// Find the windows binary asset
	var downloadURL string
	var totalSize int
	for _, asset := range release.Assets {
		if asset.Name == "prism.exe" {
			downloadURL = asset.BrowserDownloadURL
			totalSize = asset.Size
			break
		}
	}

	if downloadURL == "" {
		return // No windows binary found in assets
	}

	// Alert the user that a new version is available
	pterm.Warning.Prefix = pterm.Prefix{
		Text:  "UPDATE",
		Style: pterm.NewStyle(pterm.BgYellow, pterm.FgBlack),
	}
	pterm.Warning.Printf("A new version of Prism CLI is available: v%s (Current: v%s)\n", latestVersion, Version)

	pterm.Print("Would you like to update now? (y/N): ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		pterm.Info.Println("Skipping update. Starting Prism CLI...")
		return
	}

	pterm.Info.Println("Downloading update...")
	exePath, err := os.Executable()
	if err != nil {
		pterm.Error.Printf("Failed to resolve executable path: %v\n", err)
		return
	}

	dir := filepath.Dir(exePath)
	tmpPath := filepath.Join(dir, "prism_new.exe")
	bakPath := exePath + ".old"

	// Download the new binary
	err = downloadFile(downloadURL, tmpPath, totalSize)
	if err != nil {
		pterm.Error.Printf("Failed to download update: %v\n", err)
		_ = os.Remove(tmpPath)
		return
	}

	// Rename current exe to .old, and rename tmp to current exe
	err = os.Rename(exePath, bakPath)
	if err != nil {
		pterm.Error.Printf("Failed to rename current binary: %v\n", err)
		_ = os.Remove(tmpPath)
		return
	}

	err = os.Rename(tmpPath, exePath)
	if err != nil {
		pterm.Error.Printf("Failed to install update: %v\n", err)
		// Try to restore old binary
		_ = os.Rename(bakPath, exePath)
		return
	}

	pterm.Success.Println("Update completed successfully! Restarting Prism CLI...")
	
	// Restart
	cmd := exec.Command(exePath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	_ = cmd.Run()
	os.Exit(0)
}

type progressWriter struct {
	writer io.Writer
	pb     *pterm.ProgressbarPrinter
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err != nil {
		return n, err
	}
	if pw.pb != nil {
		pw.pb.Add(n)
	}
	return n, nil
}

func downloadFile(url string, targetPath string, totalSize int) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Initialize progress bar
	pb, _ := pterm.DefaultProgressbar.WithTotal(totalSize).WithTitle("Downloading Prism CLI").Start()
	
	pw := &progressWriter{
		writer: out,
		pb:     pb,
	}

	_, err = io.Copy(pw, resp.Body)
	if err != nil {
		return err
	}

	if pb != nil {
		_, _ = pb.Stop()
	}

	return nil
}
