package main

import (
	"context"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// TerminalResult represents the output of a command execution.
type TerminalResult struct {
	Output    string
	ExitCode  int
	Truncated bool
	Error     error
}

// ExecuteCommand runs a command in the Windows terminal with UTF-8 codepage.
// It limits the output to 50,000 characters and supports cancellation via context.
func ExecuteCommand(ctx context.Context, command string) TerminalResult {
	// Adjust code page to UTF-8 on Windows
	// We run it through cmd.exe /c "chcp 65001 > nul && command"
	cmd := exec.CommandContext(ctx, "cmd.exe", "/c", "chcp 65001 > nul && "+command)

	// Combine Stdout and Stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return TerminalResult{Error: err, ExitCode: -1}
	}
	cmd.Stderr = cmd.Stdout // Redirect stderr to stdout

	// Set SysProcAttr to hide window and run cleanly
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := cmd.Start(); err != nil {
		return TerminalResult{Error: err, ExitCode: -1}
	}

	// Read output in a separate goroutine up to 50,000 characters
	outputChan := make(chan TerminalResult, 1)
	go func() {
		buf := make([]byte, 1024)
		var totalChars int
		var outBytes []byte
		truncated := false

		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				totalChars += n
				if totalChars > 50000 {
					// Read up to limit and flag truncation
					limit := 50000 - len(outBytes)
					if limit > 0 {
						outBytes = append(outBytes, buf[:limit]...)
					}
					truncated = true
					// Terminate process group if it exceeds the limit to stop endless loops
					_ = cmd.Process.Kill()
					break
				}
				outBytes = append(outBytes, buf[:n]...)
			}
			if err != nil {
				if err != io.EOF {
					// Handle read error if any
				}
				break
			}
		}

		err = cmd.Wait()
		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				} else {
					exitCode = 1
				}
			} else {
				exitCode = 1
			}
		}

		outputStr := string(outBytes)
		if truncated {
			outputStr += "\n\n--- [LOG TRUNCATED - EXCEEDED 50,000 CHARACTERS LIMIT] ---"
		}

		outputChan <- TerminalResult{
			Output:    outputStr,
			ExitCode:  exitCode,
			Truncated: truncated,
			Error:     err,
		}
	}()

	// Wait for command completion, timeout or context cancellation
	select {
	case res := <-outputChan:
		return res
	case <-ctx.Done():
		// Kill the process group to ensure children are also terminated
		// Generate Console Ctrl Event is sent on Windows if running in a process group
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return TerminalResult{
			Output:   "Command execution cancelled by user.",
			ExitCode: -1,
			Error:    context.Canceled,
		}
	}
}

// ExecuteCommandWithTimeout runs ExecuteCommand with a default timeout
func ExecuteCommandWithTimeout(command string, timeout time.Duration) TerminalResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return ExecuteCommand(ctx, command)
}
