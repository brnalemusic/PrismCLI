package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePathplaceholders(t *testing.T) {
	invalidPaths := []string{
		`C:\Code\...\file.go`,
		`C:\Code\<path>\file.go`,
		`C:\Code\TODO\file.go`,
		`C:\Code\YOUR_CODE_HERE\file.go`,
	}

	for _, p := range invalidPaths {
		_, err := ValidatePath(p, true)
		if err == nil {
			t.Errorf("Path containing placeholders was not rejected: %s", p)
		} else if !strings.Contains(strings.ToLower(err.Error()), "rejected") {
			t.Errorf("Unexpected error message for placeholder validation: %v", err)
		}
	}
}

func TestValidatePathRootSafeguard(t *testing.T) {
	// Root paths should fail when writeOp is true
	rootPaths := []string{
		`C:\`,
		`C:\file.txt`,
		`C:/system.sys`,
		`C:\Windows`,
		`C:\Windows\System32\cmd.exe`,
	}

	for _, p := range rootPaths {
		_, err := ValidatePath(p, true)
		if err == nil {
			t.Errorf("Dangerous root/system write path was not rejected: %s", p)
		} else if !strings.Contains(strings.ToLower(err.Error()), "rejected") {
			t.Errorf("Unexpected error message for root validation: %v", err)
		}
	}
}

func TestFileCreationAndCleanup(t *testing.T) {
	tempFile := filepath.Join(os.TempDir(), "prism_test_dir", "test_file.txt")
	defer os.RemoveAll(filepath.Dir(tempFile))

	// Validate path is ok for writes
	cleanPath, err := ValidatePath(tempFile, true)
	if err != nil {
		t.Fatalf("Valid temp path rejected: %v", err)
	}

	// Create
	content := "Prism CLI Temp Content Test"
	err = FileCreate(cleanPath, content)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Read
	readVal, err := FileListRead(cleanPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	readStr, ok := readVal.(string)
	if !ok {
		t.Fatalf("FileListRead did not return string content")
	}

	if readStr != content {
		t.Errorf("Expected content %s, got %s", content, readStr)
	}

	// Edit
	err = FileEdit(cleanPath, "Temp Content", "New Replaced Content")
	if err != nil {
		t.Fatalf("Failed to edit file: %v", err)
	}

	readValNew, _ := FileListRead(cleanPath)
	readStrNew := readValNew.(string)
	if !strings.Contains(readStrNew, "New Replaced Content") {
		t.Errorf("File contents did not update correctly after edit: %s", readStrNew)
	}
}
