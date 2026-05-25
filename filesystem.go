package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePath checks if a path is safe and free of placeholders.
func ValidatePath(path string, writeOp bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// 1. Check for placeholders
	placeholders := []string{"...", "<path>", "[path]", "TODO", "YOUR_CODE_HERE", "<key>", "<api_key>"}
	lowerPath := strings.ToLower(path)
	for _, ph := range placeholders {
		if strings.Contains(lowerPath, strings.ToLower(ph)) {
			return "", fmt.Errorf("path rejected: contains placeholder/temporary marking '%s'", ph)
		}
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %v", err)
	}

	// 2. Safeguard root directory actions
	volName := filepath.VolumeName(absPath)
	cleanPath := filepath.Clean(absPath)
	rootPath := volName + string(filepath.Separator)

	if writeOp {
		// Reject if writing directly to root (e.g. C:\ or C:\file.txt)
		if cleanPath == volName || cleanPath == rootPath {
			return "", fmt.Errorf("path rejected: writing operations to the root directory (%s) are not allowed", rootPath)
		}

		// Reject direct child of root (e.g. C:\file.txt)
		parent := filepath.Dir(cleanPath)
		if parent == volName || parent == rootPath {
			return "", fmt.Errorf("path rejected: writing files directly under the root directory (%s) is not allowed", parent)
		}

		// Reject writing to critical OS directories
		forbiddenDirs := []string{
			strings.ToLower(filepath.Join(rootPath, "Windows")),
			strings.ToLower(filepath.Join(rootPath, "ProgramData")),
			strings.ToLower(filepath.Join(rootPath, "System Volume Information")),
			strings.ToLower(filepath.Join(rootPath, "$Recycle.Bin")),
		}
		lowerCleanPath := strings.ToLower(cleanPath)
		for _, fDir := range forbiddenDirs {
			if lowerCleanPath == fDir || strings.HasPrefix(lowerCleanPath, fDir+string(filepath.Separator)) {
				return "", fmt.Errorf("path rejected: modifying system directories (%s) is not allowed", fDir)
			}
		}
	}

	return cleanPath, nil
}

// FileCreate writes a new file containing text.
// It auto-creates directories if needed, but fails if the file already exists.
func FileCreate(path string, content string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}

	// Check if file exists
	if _, err := os.Stat(cleanPath); err == nil {
		return fmt.Errorf("file already exists at the specified path: %s", cleanPath)
	}

	// Create directories if needed
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating intermediate folders: %v", err)
	}

	return os.WriteFile(cleanPath, []byte(content), 0644)
}

// FileEdit performs targeted replacement of text block.
// It fails if the old text block is not found.
func FileEdit(path string, targetContent string, replacementContent string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}

	// Read current content
	contentBytes, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("error reading file for editing: %v", err)
	}
	contentStr := string(contentBytes)

	// Check for target
	if !strings.Contains(contentStr, targetContent) {
		return fmt.Errorf("the specified target text block was not found in the file")
	}

	// Replace
	newContentStr := strings.Replace(contentStr, targetContent, replacementContent, 1) // Replace 1 occurrence
	return os.WriteFile(cleanPath, []byte(newContentStr), 0644)
}

// FileWrite overwrites or creates a file in the disk.
func FileWrite(path string, content string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}

	// Create directories if needed
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating intermediate folders: %v", err)
	}

	return os.WriteFile(cleanPath, []byte(content), 0644)
}

// FileAppend appends text to the end of an existing file.
func FileAppend(path string, content string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}

	file, err := os.OpenFile(cleanPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

// FileMoveCopy moves or copies files or folders.
func FileMoveCopy(srcPath string, destPath string, op string, overwrite bool) error {
	cleanSrc, err := ValidatePath(srcPath, false)
	if err != nil {
		return err
	}
	cleanDest, err := ValidatePath(destPath, true)
	if err != nil {
		return err
	}

	// Check if dest exists
	if _, err := os.Stat(cleanDest); err == nil && !overwrite {
		return fmt.Errorf("destination already exists and overwrite flag is disabled: %s", cleanDest)
	}

	if strings.ToLower(op) == "move" {
		// Check if source exists
		if _, err := os.Stat(cleanSrc); err != nil {
			return fmt.Errorf("source file not found: %v", err)
		}
		// Move/Rename
		return os.Rename(cleanSrc, cleanDest)
	} else if strings.ToLower(op) == "copy" {
		// Copy file or directory recursively
		return copyRecursive(cleanSrc, cleanDest)
	}

	return fmt.Errorf("unknown operation: %s (use 'move' or 'copy')", op)
}

func copyRecursive(src string, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := os.MkdirAll(dest, info.Mode()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			srcPath := filepath.Join(src, entry.Name())
			destPath := filepath.Join(dest, entry.Name())
			if err := copyRecursive(srcPath, destPath); err != nil {
				return err
			}
		}
		return nil
	}

	// Copy file
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return err
}

// FileMetadataResult returns detailed metadata about a file or directory.
type FileMetadataResult struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        int64  `json:"sizeBytes"`
	IsDir       bool   `json:"isDir"`
	ModTime     string `json:"modTime"`
	Permissions string `json:"permissions"`
}

// GetFileMetadata returns file/folder details.
func GetFileMetadata(path string) (FileMetadataResult, error) {
	cleanPath, err := ValidatePath(path, false)
	if err != nil {
		return FileMetadataResult{}, err
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return FileMetadataResult{}, err
	}

	return FileMetadataResult{
		Name:        info.Name(),
		Path:        cleanPath,
		Size:        info.Size(),
		IsDir:       info.IsDir(),
		ModTime:     info.ModTime().Format("2006-01-02 15:04:05"),
		Permissions: info.Mode().String(),
	}, nil
}

// FileListResult contains entries of a directory listing.
type FileListResult struct {
	Path    string               `json:"path"`
	Entries []FileMetadataResult `json:"entries"`
}

// FileListRead reads a file's string content or lists a directory's contents.
func FileListRead(path string) (interface{}, error) {
	cleanPath, err := ValidatePath(path, false)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		entries, err := os.ReadDir(cleanPath)
		if err != nil {
			return nil, err
		}

		var list []FileMetadataResult
		for _, entry := range entries {
			eInfo, err := entry.Info()
			if err != nil {
				continue
			}
			list = append(list, FileMetadataResult{
				Name:        entry.Name(),
				Path:        filepath.Join(cleanPath, entry.Name()),
				Size:        eInfo.Size(),
				IsDir:       entry.IsDir(),
				ModTime:     eInfo.ModTime().Format("2006-01-02 15:04:05"),
				Permissions: eInfo.Mode().String(),
			})
		}
		return FileListResult{Path: cleanPath, Entries: list}, nil
	}

	// Read file content
	contentBytes, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, err
	}

	return string(contentBytes), nil
}

// DirectoryCreate creates a new directory recursively.
func DirectoryCreate(path string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}
	return os.MkdirAll(cleanPath, 0755)
}

// FileDelete deletes a file from the system.
func FileDelete(path string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}
	// Verify it's a file, not a directory
	info, err := os.Stat(cleanPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("the specified path is a directory, not a file: %s", cleanPath)
	}
	return os.Remove(cleanPath)
}

// DirectoryDelete recursively deletes an existing directory and its contents.
func DirectoryDelete(path string) error {
	cleanPath, err := ValidatePath(path, true)
	if err != nil {
		return err
	}
	// Verify it's a directory, not a file
	info, err := os.Stat(cleanPath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("the specified path is a file, not a directory: %s", cleanPath)
	}
	return os.RemoveAll(cleanPath)
}

// FileCopy copies a file or directory recursively to a destination path.
func FileCopy(srcPath string, destPath string, overwrite bool) error {
	cleanSrc, err := ValidatePath(srcPath, false)
	if err != nil {
		return err
	}
	cleanDest, err := ValidatePath(destPath, true)
	if err != nil {
		return err
	}

	// Check if dest exists
	if _, err := os.Stat(cleanDest); err == nil && !overwrite {
		return fmt.Errorf("destination already exists and overwrite flag is disabled: %s", cleanDest)
	}

	// Ensure destination parent directory exists
	dir := filepath.Dir(cleanDest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating destination directories: %v", err)
	}

	return copyRecursive(cleanSrc, cleanDest)
}

// FileMove moves or renames a file or directory to a destination path.
func FileMove(srcPath string, destPath string, overwrite bool) error {
	cleanSrc, err := ValidatePath(srcPath, false)
	if err != nil {
		return err
	}
	cleanDest, err := ValidatePath(destPath, true)
	if err != nil {
		return err
	}

	// Check if dest exists
	if _, err := os.Stat(cleanDest); err == nil {
		if !overwrite {
			return fmt.Errorf("destination already exists and overwrite flag is disabled: %s", cleanDest)
		}
		if err := os.RemoveAll(cleanDest); err != nil {
			return fmt.Errorf("error removing existing destination: %v", err)
		}
	}

	// Ensure destination parent directory exists
	dir := filepath.Dir(cleanDest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating destination directories: %v", err)
	}

	return os.Rename(cleanSrc, cleanDest)
}

// ListDirectory lists the contents of a directory.
func ListDirectory(path string) (string, error) {
	cleanPath, err := ValidatePath(path, false)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return "", err
	}

	var list []string
	for _, entry := range entries {
		prefix := "[FILE]"
		if entry.IsDir() {
			prefix = "[DIR]"
		}
		list = append(list, fmt.Sprintf("%s %s", prefix, entry.Name()))
	}

	if len(list) == 0 {
		return "Directory is empty.", nil
	}

	return strings.Join(list, "\n"), nil
}

// FileRead reads the text content of a file.
func FileRead(path string) (string, error) {
	cleanPath, err := ValidatePath(path, false)
	if err != nil {
		return "", err
	}

	// Verify it's a file
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("the specified path is a directory, not a file: %s", cleanPath)
	}

	contentBytes, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}

	return string(contentBytes), nil
}

