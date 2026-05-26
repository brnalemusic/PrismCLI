package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"golang.org/x/term"
	"google.golang.org/genai"
)

// ModelFallbackSequence is the sequence of models to try in case of failure.
var ModelFallbackSequence = []string{
	"gemini-3.5-flash",
	"gemma-4-31b-it",
	"gemma-4-26b-a4b-it",
	"gemini-3-flash-preview",
	"gemini-3.1-flash-lite",
	"gemini-2.5-flash",
	"gemini-1.5-flash",
}

// ModelFriendlyNames maps real model IDs to user-friendly names.
var ModelFriendlyNames = map[string]string{
	"gemini-3.5-flash":       "Prism 5",
	"gemma-4-31b-it":         "Prism 4.3",
	"gemma-4-26b-a4b-it":     "Prism 4.2",
	"gemini-3-flash-preview": "Prism 4.1",
	"gemini-3.1-flash-lite":  "Prism 4",
	"gemini-2.5-flash":       "Prism 2.5 (Fallback)",
	"gemini-1.5-flash":       "Prism 1.5 (Fallback)",
}

// GetToolDefinitions returns the schema for all tools available to the model.
// GetToolDefinitions returns the schema for all tools available to the model.
func GetToolDefinitions() []*genai.Tool {
	executeTerminalCommand := &genai.FunctionDeclaration{
		Name:        "execute_terminal_command",
		Description: "Executes a command in the Windows terminal (cmd/powershell). Returns output truncated at 50,000 characters.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"command": {Type: "string", Description: "The exact command to be executed in the terminal."},
			},
			Required: []string{"command"},
		},
	}

	computerUseCreateFile := &genai.FunctionDeclaration{
		Name:        "computer_use_create_file",
		Description: "Creates a new file on disk with the specified content. Automatically creates parent directories and fails if the file already exists.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path":    {Type: "string", Description: "Full path of the file to be created."},
				"content": {Type: "string", Description: "Initial textual content of the file."},
			},
			Required: []string{"path", "content"},
		},
	}

	computerUseCreateDirectory := &genai.FunctionDeclaration{
		Name:        "computer_use_create_directory",
		Description: "Creates a new directory recursively.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path": {Type: "string", Description: "Full path of the directory to be created."},
			},
			Required: []string{"path"},
		},
	}

	computerUseRemoveFile := &genai.FunctionDeclaration{
		Name:        "computer_use_remove_file",
		Description: "Deletes a file from the system.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path": {Type: "string", Description: "Full path of the file to be deleted."},
			},
			Required: []string{"path"},
		},
	}

	computerUseRemoveDirectory := &genai.FunctionDeclaration{
		Name:        "computer_use_remove_directory",
		Description: "Recursively deletes an existing directory and all its contents.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path": {Type: "string", Description: "Full path of the directory to be deleted."},
			},
			Required: []string{"path"},
		},
	}

	computerUseSaveFile := &genai.FunctionDeclaration{
		Name:        "computer_use_save_file",
		Description: "Writes or completely overwrites a file with new content. Automatically creates parent directories.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path":    {Type: "string", Description: "Full path of the file to be saved."},
				"content": {Type: "string", Description: "Complete textual content to be written."},
			},
			Required: []string{"path", "content"},
		},
	}

	computerUseAppendFile := &genai.FunctionDeclaration{
		Name:        "computer_use_append_file",
		Description: "Adds text to the end of a file. Creates the file and parent directories if they don't exist.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path":    {Type: "string", Description: "Full path of the file."},
				"content": {Type: "string", Description: "Text to be appended."},
			},
			Required: []string{"path", "content"},
		},
	}

	computerUseEditFile := &genai.FunctionDeclaration{
		Name:        "computer_use_edit_file",
		Description: "Edits a file by replacing the exact old text (oldText) with new text (newText). Used for targeted changes.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path":    {Type: "string", Description: "Full path of the file."},
				"oldText": {Type: "string", Description: "The exact text currently in the file to be replaced."},
				"newText": {Type: "string", Description: "The new replacement text."},
			},
			Required: []string{"path", "oldText", "newText"},
		},
	}

	computerUseReplaceInFile := &genai.FunctionDeclaration{
		Name:        "computer_use_replace_in_file",
		Description: "Backward compatible alias for computer_use_edit_file.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path":    {Type: "string", Description: "Full path of the file."},
				"oldText": {Type: "string", Description: "The exact text currently in the file to be replaced."},
				"newText": {Type: "string", Description: "The new replacement text."},
			},
			Required: []string{"path", "oldText", "newText"},
		},
	}

	computerUseCopyFile := &genai.FunctionDeclaration{
		Name:        "computer_use_copy_file",
		Description: "Copies a file or directory to a destination path.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"sourcePath":      {Type: "string", Description: "Full source path."},
				"destinationPath": {Type: "string", Description: "Full destination path."},
				"overwrite":       {Type: "string", Description: "Optional: 'true' or 'false' (default 'false'). Allows overwriting if the destination already exists."},
			},
			Required: []string{"sourcePath", "destinationPath"},
		},
	}

	computerUseMoveFile := &genai.FunctionDeclaration{
		Name:        "computer_use_move_file",
		Description: "Moves or renames a file or directory to a destination path.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"sourcePath":      {Type: "string", Description: "Full source path."},
				"destinationPath": {Type: "string", Description: "Full destination path."},
				"overwrite":       {Type: "string", Description: "Optional: 'true' or 'false' (default 'false'). Allows overwriting if the destination already exists."},
			},
			Required: []string{"sourcePath", "destinationPath"},
		},
	}

	computerUseGetFileInfo := &genai.FunctionDeclaration{
		Name:        "computer_use_get_file_info",
		Description: "Returns metadata for a file or directory: type, size, timestamps, and permissions.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path": {Type: "string", Description: "Full path of the file or directory."},
			},
			Required: []string{"path"},
		},
	}

	computerUseListDirectory := &genai.FunctionDeclaration{
		Name:        "computer_use_list_directory",
		Description: "Lists the contents of a directory.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path": {Type: "string", Description: "Full path of the directory."},
			},
			Required: []string{"path"},
		},
	}

	computerUseReadFile := &genai.FunctionDeclaration{
		Name:        "computer_use_read_file",
		Description: "Reads the textual content of a file.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"path": {Type: "string", Description: "Full path of the file."},
			},
			Required: []string{"path"},
		},
	}

	listInstalledApplications := &genai.FunctionDeclaration{
		Name:        "list_installed_applications",
		Description: "Lists all applications and games installed on Windows (including Steam, Epic, Valorant, Chrome, etc.) and their actual executable paths. You MUST use this tool instead of running terminal/powershell commands (like Get-ItemProperty or Get-StartApps) to list applications, as it is cached, extremely fast, and much more comprehensive.",
		Parameters: &genai.Schema{
			Type: "object",
		},
	}

	openApplication := &genai.FunctionDeclaration{
		Name:        "open_application",
		Description: "Opens an application using the literal executable path (must end in .exe). You should ALWAYS use this tool to open applications instead of terminal/command-line tools, unless it is impossible to open the application via this tool.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"appPath": {Type: "string", Description: "Literal path of the executable file (must end in .exe)."},
			},
			Required: []string{"appPath"},
		},
	}

	webSearch := &genai.FunctionDeclaration{
		Name:        "web_search",
		Description: "Performs a web search to obtain real-time information.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"query": {Type: "string", Description: "Search keywords."},
			},
			Required: []string{"query"},
		},
	}

	sawLinkFromUrl := &genai.FunctionDeclaration{
		Name:        "saw_link_from_url",
		Description: "Fetches and reads the textual content of a specific URL.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"url": {Type: "string", Description: "Web page URL."},
			},
			Required: []string{"url"},
		},
	}

	openBrowserLink := &genai.FunctionDeclaration{
		Name:        "open_browser_link",
		Description: "Opens a URL in the system's default browser.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"url": {Type: "string", Description: "Destination URL."},
			},
			Required: []string{"url"},
		},
	}

	searchChatHistory := &genai.FunctionDeclaration{
		Name:        "search_chat_history",
		Description: "Searches all previous conversations for specific contexts or preferences. Use comma-separated keywords for better results.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"query": {Type: "string", Description: "Comma-separated keywords for history search (e.g., 'keyword1, keyword2')."},
			},
			Required: []string{"query"},
		},
	}

	openMainApp := &genai.FunctionDeclaration{
		Name:        "open_main_app",
		Description: "Opens the main application window (Prism Desktop), starts a new clean chat session, and sends instructions to be executed using a specific Prism model. Use this tool if you need terminal execution, file system access, subagents, or if you need to generate Rich Markdown dashboards, profile cards, etc.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"instructions":   {Type: "string", Description: "The destination instructions for the main assistant to execute."},
				"model":          {Type: "string", Description: "The model key to be used for the main session (e.g., prism-5, prism-4.3, prism-4.2)."},
				"thinkMode":      {Type: "string", Description: "Optional: Set to 'true' to enable think mode in the main app."},
				"searchEnabled":  {Type: "string", Description: "Optional: Set to 'true' to enable web search in the main app."},
				"extendedSearch": {Type: "string", Description: "Optional: Set to 'true' to enable deep research / extended search in the main app."},
			},
			Required: []string{"instructions"},
		},
	}

	configurePrism := &genai.FunctionDeclaration{
		Name:        "configure_prism",
		Description: "Configures Prism application settings. Any combination of parameters can be specified to customize shortcuts, models, window behaviors, user details, and API keys.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"launcherShortcut":       {Type: "string", Description: "Optional: Global shortcut to open/close the launcher (e.g., CommandOrControl+Space)."},
				"modelSelectionShortcut": {Type: "string", Description: "Optional: Global shortcut to open/close the model selection dialog."},
				"defaultModel":           {Type: "string", Description: "Optional: Default main chat model (prism-5, prism-4.3, prism-4.2, prism-4.1, prism-4)."},
				"subagentModel":          {Type: "string", Description: "Optional: Default subagent model (prism-5, prism-4.3, prism-4.2, prism-4.1, prism-4)."},
				"minimizeToTray":         {Type: "string", Description: "Optional: Minimize window to system tray on close ('true'/'false')."},
				"autoLaunch":             {Type: "string", Description: "Optional: Automatically start the app at system login ('true'/'false')."},
				"quickLauncherMode":      {Type: "string", Description: "Optional: Quick launcher screen mode ('simple'/'advanced')."},
				"userGeminiKey":          {Type: "string", Description: "Optional: User's custom Google Gemini API key."},
				"username":               {Type: "string", Description: "Optional: Custom username for personalization."},
			},
		},
	}

	runSubagents := &genai.FunctionDeclaration{
		Name:        "run_subagents",
		Description: "Starts subagents to execute tasks in parallel. Ideal for complex requests (swarm).",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"quantity": {Type: "string", Description: "Number of agents to start."},
				"prompt:1": {Type: "string", Description: "Detailed prompt for agent 1."},
				"prompt:2": {Type: "string", Description: "Detailed prompt for agent 2 (repeat for X agents)."},
			},
			Required: []string{"quantity", "prompt:1"},
		},
	}

	sendGroupMessage := &genai.FunctionDeclaration{
		Name:        "send_group_message",
		Description: "Sends a message to the group chat. If you want to wait for a response, you MUST also call wait_for_updates in the same turn.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"content": {Type: "string", Description: "The message to be transmitted."},
				"status":  {Type: "string", Description: "Use 'working' to stay active. Use 'done' or 'error' to finalize and terminate."},
			},
			Required: []string{"content", "status"},
		},
	}

	readGroupMessages := &genai.FunctionDeclaration{
		Name:        "read_group_messages",
		Description: "Retrieves previous messages from the group chat history.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"sinceTimestamp": {Type: "string", Description: "Optional: Only get messages after this timestamp."},
				"limit":          {Type: "string", Description: "Optional: Maximum number of messages to return."},
			},
		},
	}

	waitForUpdates := &genai.FunctionDeclaration{
		Name:        "wait_for_updates",
		Description: "Pauses execution until a new message is received. Use this after sending a message to wait for a response.",
		Parameters: &genai.Schema{
			Type: "object",
			Properties: map[string]*genai.Schema{
				"timeoutSeconds": {Type: "string", Description: "Maximum time to wait in seconds (max 180s)."},
			},
		},
	}

	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				executeTerminalCommand, computerUseCreateFile, computerUseCreateDirectory,
				computerUseRemoveFile, computerUseRemoveDirectory, computerUseSaveFile,
				computerUseAppendFile, computerUseEditFile, computerUseReplaceInFile,
				computerUseCopyFile, computerUseMoveFile, computerUseGetFileInfo,
				computerUseListDirectory, computerUseReadFile, listInstalledApplications,
				openApplication, webSearch, sawLinkFromUrl, openBrowserLink,
				searchChatHistory, openMainApp, configurePrism, runSubagents,
				sendGroupMessage, readGroupMessages, waitForUpdates,
			},
		},
	}
}

// CallLocalTool routes function calls requested by the model to local Go implementations.
func CallLocalTool(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	// Backward compatibility/aliases
	case "terminal_execute":
		cmd, _ := args["command"].(string)
		res := ExecuteCommandWithTimeout(cmd, 60*time.Second)
		return map[string]interface{}{
			"output":    res.Output,
			"exitCode":  res.ExitCode,
			"truncated": res.Truncated,
		}, nil

	case "file_create":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		err := FileCreate(path, content)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "File created successfully!"}, nil

	case "file_edit":
		path, _ := args["path"].(string)
		target, _ := args["targetContent"].(string)
		rep, _ := args["replacementContent"].(string)
		err := FileEdit(path, target, rep)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Replacement performed successfully!"}, nil

	case "file_write":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		err := FileWrite(path, content)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "File written successfully!"}, nil

	case "file_append":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		err := FileAppend(path, content)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Data appended successfully!"}, nil

	case "file_move_copy":
		src, _ := args["srcPath"].(string)
		dest, _ := args["destPath"].(string)
		op, _ := args["op"].(string)
		overwrite, _ := args["overwrite"].(bool)
		err := FileMoveCopy(src, dest, op, overwrite)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Move/copy operation performed!"}, nil

	case "file_metadata":
		path, _ := args["path"].(string)
		res, err := GetFileMetadata(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return res, nil

	case "file_list_read":
		path, _ := args["path"].(string)
		res, err := FileListRead(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		if str, ok := res.(string); ok {
			return map[string]interface{}{"content": str}, nil
		}
		return res, nil

	case "web_read":
		urlStr, _ := args["url"].(string)
		res, err := WebReader(urlStr)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"content": res}, nil

	case "app_scan":
		res := ScanApplications()
		return res, nil

	case "app_launch":
		path, _ := args["path"].(string)
		err := LaunchApplication(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Application opened successfully!"}, nil

	case "history_search":
		query, _ := args["query"].(string)
		res := SearchChatHistory(query)
		formatted := FormatSearchResults(res)
		return map[string]interface{}{"context": formatted}, nil

	case "swarm_execute":
		goal, _ := args["goal"].(string)
		RunSwarmTask(context.Background(), goal)
		return map[string]interface{}{"status": "success", "message": "Goal successfully finalized by the agent swarm!"}, nil

	// New tools
	case "execute_terminal_command":
		cmd, _ := args["command"].(string)
		res := ExecuteCommandWithTimeout(cmd, 60*time.Second)
		return map[string]interface{}{
			"output":    res.Output,
			"exitCode":  res.ExitCode,
			"truncated": res.Truncated,
		}, nil

	case "computer_use_create_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		err := FileCreate(path, content)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "File created successfully!"}, nil

	case "computer_use_create_directory":
		path, _ := args["path"].(string)
		err := DirectoryCreate(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Directory created successfully!"}, nil

	case "computer_use_remove_file":
		path, _ := args["path"].(string)
		err := FileDelete(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "File deleted successfully!"}, nil

	case "computer_use_remove_directory":
		path, _ := args["path"].(string)
		err := DirectoryDelete(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Directory deleted successfully!"}, nil

	case "computer_use_save_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		err := FileWrite(path, content)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "File written successfully!"}, nil

	case "computer_use_append_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		err := FileAppend(path, content)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Data appended successfully!"}, nil

	case "computer_use_edit_file", "computer_use_replace_in_file":
		path, _ := args["path"].(string)
		oldText, _ := args["oldText"].(string)
		newText, _ := args["newText"].(string)
		err := FileEdit(path, oldText, newText)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Replacement performed successfully!"}, nil

	case "computer_use_copy_file":
		src, _ := args["sourcePath"].(string)
		dest, _ := args["destinationPath"].(string)
		overwriteStr, _ := args["overwrite"].(string)
		overwrite := parseBoolString(overwriteStr)
		err := FileCopy(src, dest, overwrite)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Copy performed successfully!"}, nil

	case "computer_use_move_file":
		src, _ := args["sourcePath"].(string)
		dest, _ := args["destinationPath"].(string)
		overwriteStr, _ := args["overwrite"].(string)
		overwrite := parseBoolString(overwriteStr)
		err := FileMove(src, dest, overwrite)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Move performed successfully!"}, nil

	case "computer_use_get_file_info":
		path, _ := args["path"].(string)
		res, err := GetFileMetadata(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return res, nil

	case "computer_use_list_directory":
		path, _ := args["path"].(string)
		res, err := ListDirectory(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"output": res}, nil

	case "computer_use_read_file":
		path, _ := args["path"].(string)
		res, err := FileRead(path)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"content": res}, nil

	case "list_installed_applications":
		res := ScanApplications()
		return res, nil

	case "open_application":
		appPath, _ := args["appPath"].(string)
		err := LaunchApplication(appPath)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Application opened successfully!"}, nil

	case "web_search":
		query, _ := args["query"].(string)
		res, err := ScrapeDuckDuckGo(query)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return res, nil

	case "saw_link_from_url":
		urlStr, _ := args["url"].(string)
		res, err := WebReader(urlStr)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"content": res}, nil

	case "open_browser_link":
		urlStr, _ := args["url"].(string)
		err := OpenBrowserLink(urlStr)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Link opened in browser successfully!"}, nil

	case "search_chat_history":
		query, _ := args["query"].(string)
		res := SearchChatHistory(query)
		formatted := FormatSearchResults(res)
		return map[string]interface{}{"context": formatted}, nil

	case "open_main_app":
		err := OpenMainApp()
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}
		return map[string]interface{}{"status": "success", "message": "Prism main window opened successfully!"}, nil

	case "configure_prism":
		config, err := LoadConfig()
		if err != nil {
			return map[string]interface{}{"status": "error", "message": err.Error()}, nil
		}

		changed := []string{}

		if val, ok := args["launcherShortcut"].(string); ok && val != "" {
			config.LauncherShortcut = val
			changed = append(changed, fmt.Sprintf("launcherShortcut: %q", val))
		}
		if val, ok := args["modelSelectionShortcut"].(string); ok && val != "" {
			config.ModelSelectionShortcut = val
			changed = append(changed, fmt.Sprintf("modelSelectionShortcut: %q", val))
		}
		if val, ok := args["defaultModel"].(string); ok && val != "" {
			apiModel := mapModelKeyToAPI(val)
			config.DefaultModel = apiModel
			changed = append(changed, fmt.Sprintf("defaultModel: %q", val))
		}
		if val, ok := args["subagentModel"].(string); ok && val != "" {
			apiModel := mapModelKeyToAPI(val)
			config.SubagentModel = apiModel
			changed = append(changed, fmt.Sprintf("subagentModel: %q", val))
		}
		if valStr, ok := args["minimizeToTray"].(string); ok && valStr != "" {
			val := parseBoolString(valStr)
			config.MinimizeToTray = val
			changed = append(changed, fmt.Sprintf("minimizeToTray: %t", val))
		}
		if valStr, ok := args["autoLaunch"].(string); ok && valStr != "" {
			val := parseBoolString(valStr)
			config.AutoLaunch = val
			changed = append(changed, fmt.Sprintf("autoLaunch: %t", val))
		}
		if val, ok := args["quickLauncherMode"].(string); ok && val != "" {
			if val == "simple" || val == "advanced" {
				config.QuickLauncherMode = val
				changed = append(changed, fmt.Sprintf("quickLauncherMode: %q", val))
			}
		}
		if val, ok := args["userGeminiKey"].(string); ok && val != "" {
			err := config.SetAPIKey(val)
			if err != nil {
				return map[string]interface{}{"status": "error", "message": "error saving API key: " + err.Error()}, nil
			}
			changed = append(changed, "userGeminiKey: [UPDATED]")
		}
		if val, ok := args["username"].(string); ok && val != "" {
			config.Username = val
			changed = append(changed, fmt.Sprintf("username: %q", val))
		}

		if len(changed) == 0 {
			return map[string]interface{}{"status": "success", "message": "No configuration provided for change."}, nil
		}

		err = SaveConfig(config)
		if err != nil {
			return map[string]interface{}{"status": "error", "message": "error saving configuration: " + err.Error()}, nil
		}

		msg := fmt.Sprintf("Prism configuration updated successfully:\n%s", strings.Join(changed, "\n"))
		return map[string]interface{}{"status": "success", "message": msg}, nil

	case "run_subagents":
		quantityStr, _ := args["quantity"].(string)
		var quantity int
		fmt.Sscanf(quantityStr, "%d", &quantity)
		if quantity <= 0 {
			quantity = 1
		}
		var prompts []string
		for i := 1; i <= 20; i++ {
			pKey := fmt.Sprintf("prompt:%d", i)
			if p, ok := args[pKey].(string); ok && p != "" {
				prompts = append(prompts, p)
			}
		}

		RunSubagentsSim(quantity, prompts)
		return map[string]interface{}{"status": "success", "message": "Subagents completed their tasks successfully!"}, nil

	case "send_group_message":
		return nil, fmt.Errorf("error: send_group_message can only be used by subagents")

	case "read_group_messages":
		return nil, fmt.Errorf("error: read_group_messages can only be used by subagents")

	case "wait_for_updates":
		return nil, fmt.Errorf("error: wait_for_updates can only be used by subagents")
	}

	return nil, fmt.Errorf("unknown tool: %s", name)
}

func mapModelKeyToAPI(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "prism-5", "gemini-3.5-flash":
		return "gemini-3.5-flash"
	case "prism-4.3", "gemma-4-31b-it":
		return "gemma-4-31b-it"
	case "prism-4.2", "gemma-4-26b-a4b-it":
		return "gemma-4-26b-a4b-it"
	case "prism-4.1", "gemini-3-flash-preview":
		return "gemini-3-flash-preview"
	case "prism-4", "gemini-3.1-flash-lite":
		return "gemini-3.1-flash-lite"
	default:
		return key
	}
}

func parseBoolString(val string) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	return v == "true" || v == "1" || v == "yes" || v == "y" || v == "sim" || v == "s"
}

// GenerateResponse runs a full multi-turn conversational loop, supporting tool calls,
// streaming, Think Mode, and automatic model fallback.
func GenerateResponse(ctx context.Context, cfg *Config, messages []ChatMessage, systemPrompt string, thinkMode bool, activeSearch bool, deepResearch bool) (string, error) {
	apiKey, err := cfg.GetAPIKey()
	if err != nil {
		return "", err
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", err
	}

	currentModelIdx := 0
	// Try loading default model index
	for idx, m := range ModelFallbackSequence {
		if m == cfg.DefaultModel {
			currentModelIdx = idx
			break
		}
	}

	// Prepare history contents for GenAI v2 SDK
	var contents []*genai.Content
	for _, m := range messages {
		contents = append(contents, &genai.Content{
			Role: m.Role,
			Parts: []*genai.Part{
				{Text: m.Content},
			},
		})
	}

	// Visual indicators for terminal
	hasIndicators := false
	if thinkMode {
		fmt.Print("\033[33m💭 Thinking...\033[0m ")
		hasIndicators = true
	}
	if activeSearch {
		fmt.Print("\033[32m🔍 Active Search...\033[0m ")
		hasIndicators = true
	}
	if deepResearch {
		fmt.Print("\033[36m🧪 Deep Research...\033[0m ")
		hasIndicators = true
	}
	if hasIndicators {
		fmt.Println()
	}

	var finalResponse string

	for {
		modelName := ModelFallbackSequence[currentModelIdx]

		// Set up generation config
		genConfig := &genai.GenerateContentConfig{
			Temperature: float32Ptr(0.7),
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{
					{Text: systemPrompt},
				},
			},
		}

		// Configure Think Mode / Temperature depending on action
		if thinkMode {
			genConfig.ThinkingConfig = &genai.ThinkingConfig{
				IncludeThoughts: false,
				ThinkingLevel:   genai.ThinkingLevel("HIGH"),
			}
		} else {
			genConfig.ThinkingConfig = &genai.ThinkingConfig{
				IncludeThoughts: false,
				ThinkingLevel:   genai.ThinkingLevel("MINIMAL"),
			}
		}

		// Attach tools
		genConfig.Tools = GetToolDefinitions()

		// Execute API call
		// We support streaming responses directly to stdout
		fmt.Printf("\n\033[1;35m🔮 %s\033[0m\n", ModelFriendlyNames[modelName])

		renderer := NewRenderer()
		
		stream := client.Models.GenerateContentStream(ctx, modelName, contents, genConfig)
		
		thoughtWriter := NewThoughtWriter(false, renderer)
		filter := NewStreamFilter(false, thoughtWriter, renderer)
		var accumulatedThoughts strings.Builder
		var lastToolCallPart *genai.Part
		var apiError error
		writingToolCallPrinted := false

		for result, err := range stream {
			if err != nil {
				apiError = err
				break
			}
			
			// Process parts
			for _, candidate := range result.Candidates {
				if candidate.Content != nil {
					for _, part := range candidate.Content.Parts {
						if part.FunctionCall != nil {
							lastToolCallPart = part
							if !writingToolCallPrinted {
								PrintWritingToolCallScreen()
								writingToolCallPrinted = true
							}
						} else if part.Thought {
							accumulatedThoughts.WriteString(part.Text)
							thoughtWriter.Write(part.Text)
						} else if part.Text != "" {
							filter.Feed(part.Text)
						}
					}
				}
			}
			renderer.FlushWriter()
		}
		filter.Flush()
		thoughtWriter.Close()
		renderer.ResetColumn()

		// Fallback mechanism
		if apiError != nil {
			fmt.Printf("\n\033[31m[Technical Error with %s: %v]\033[0m\n", ModelFriendlyNames[modelName], apiError)
			currentModelIdx++
			if currentModelIdx >= len(ModelFallbackSequence) {
				return "", fmt.Errorf("all models failed to execute: %v", apiError)
			}
			
			nextModel := ModelFallbackSequence[currentModelIdx]
			fmt.Printf("\033[33m[Redundancy] Automatically switching to %s...\033[0m\n", ModelFriendlyNames[nextModel])

			// Inject fallback system message into history
			contents = append(contents, &genai.Content{
				Role: "system",
				Parts: []*genai.Part{
					{Text: fmt.Sprintf("You are a contingency AI engine (model %s). The previous model failed. Analyze the full conversation history and continue executing the user's instruction exactly from where the previous model left off.", nextModel)},
				},
			})
			continue // Retry with the next model
		}

		// If the model requested a tool execution
		if lastToolCallPart != nil && lastToolCallPart.FunctionCall != nil {
			fnCall := lastToolCallPart.FunctionCall
			
			// Convert Args map[string]interface{}
			args := make(map[string]interface{})
			for k, v := range fnCall.Args {
				args[k] = v
			}

			PrintToolStart(fnCall.Name, args)

			// Add the assistant's tool call response to the conversation log
			contents = append(contents, &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					{
						FunctionCall:     fnCall,
						ThoughtSignature: lastToolCallPart.ThoughtSignature,
					},
				},
			})

			// Call local tool
			toolResult, err := CallLocalTool(fnCall.Name, args)
			if err != nil {
				toolResult = map[string]interface{}{"status": "error", "message": err.Error()}
			}

			// Convert toolResult to struct for Part.FunctionResponse
			// In genai-go SDK, FunctionResponse contains Name and Response map[string]any
			respMap := make(map[string]any)
			if m, ok := toolResult.(map[string]interface{}); ok {
				for k, v := range m {
					respMap[k] = v
				}
			} else {
				respMap["result"] = toolResult
			}

			fnResp := &genai.FunctionResponse{
				Name:     fnCall.Name,
				Response: respMap,
			}

			// Add tool response to contents history
			contents = append(contents, &genai.Content{
				Role: "tool",
				Parts: []*genai.Part{
					{FunctionResponse: fnResp},
				},
			})

			success, details := parseToolResult(toolResult)
			PrintToolEnd(success, details)
			continue // Run the loop again with the new context containing the tool result
		}

		finalResponse = filter.Text()
		fmt.Println() // Add final newline to terminal
		break
	}

	return finalResponse, nil
}

func float32Ptr(f float64) *float32 {
	val := float32(f)
	return &val
}
type StreamFilter struct {
	buffer        string
	inThought     bool
	thinkMode     bool
	thoughtWriter *ThoughtWriter
	thoughtBuf    strings.Builder
	textBuf       strings.Builder
	renderer      *Renderer
	prevLineCount int
}

func NewStreamFilter(thinkMode bool, tw *ThoughtWriter, r *Renderer) *StreamFilter {
	return &StreamFilter{
		thinkMode:     thinkMode,
		thoughtWriter: tw,
		renderer:      r,
	}
}

func (sf *StreamFilter) render() {
	// 1. Get terminal height
	termHeight := 24
	if sf.renderer != nil {
		sf.renderer.RefreshWidth()
		_, height, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && height > 0 {
			termHeight = height
		}
	}

	// 2. Render accumulated text using a fresh MarkdownColorizer
	var buf bytes.Buffer
	r := NewRendererWriter(&buf)
	if sf.renderer != nil {
		r.maxWidth = sf.renderer.maxWidth
		r.contentWidth = sf.renderer.contentWidth
		r.leftPad = sf.renderer.leftPad
		r.linePrefix = sf.renderer.linePrefix
		r.linePrefixWidth = sf.renderer.linePrefixWidth
	}
	r.RefreshWidth()

	c := NewMarkdownColorizer(r)
	c.Print(sf.textBuf.String())
	c.Flush()

	rendered := buf.String()

	// 3. Split rendered text into lines
	lines := strings.Split(rendered, "\n")
	numLines := len(lines)

	limit := termHeight - 1
	if limit < 1 {
		limit = 1
	}

	var linesToPrint []string
	if numLines > limit {
		linesToPrint = lines[numLines-limit:]
	} else {
		linesToPrint = lines
	}

	// 4. Move up and clear previous output
	if sf.prevLineCount > 0 {
		prevMove := sf.prevLineCount - 1
		if prevMove < 0 {
			prevMove = 0
		}
		fmt.Print("\r" + strings.Repeat("\033[F", prevMove) + "\033[J")
	}

	// 5. Print new lines
	outText := strings.Join(linesToPrint, "\n")
	fmt.Print(outText)

	// 6. Update prevLineCount
	sf.prevLineCount = len(linesToPrint)
}

func (sf *StreamFilter) Feed(chunk string) {
	sf.buffer += chunk
	hasNewText := false
	for {
		if !sf.inThought {
			// Look for start of thought tag
			idx := strings.Index(sf.buffer, "<thought>")
			if idx == -1 {
				// Check if the end of buffer has a partial prefix of "<thought>"
				// to avoid printing it prematurely
				partialPrefix := false
				tag := "<thought>"
				for i := 1; i < len(tag); i++ {
					if strings.HasSuffix(sf.buffer, tag[:i]) {
						// Print everything except the partial prefix
						toPrint := sf.buffer[:len(sf.buffer)-i]
						if len(toPrint) > 0 {
							sf.textBuf.WriteString(toPrint)
							hasNewText = true
							sf.buffer = sf.buffer[len(sf.buffer)-i:]
						}
						partialPrefix = true
						break
					}
				}
				if !partialPrefix {
					// Print all and clear buffer
					if len(sf.buffer) > 0 {
						sf.textBuf.WriteString(sf.buffer)
						hasNewText = true
					}
					sf.buffer = ""
				}
				break
			} else {
				// Print everything before "<thought>"
				before := sf.buffer[:idx]
				if len(before) > 0 {
					sf.textBuf.WriteString(before)
					hasNewText = true
				}
				sf.inThought = true
				sf.buffer = sf.buffer[idx+len("<thought>"):]
			}
		} else {
			// Look for end of thought tag
			idx := strings.Index(sf.buffer, "</thought>")
			if idx == -1 {
				// Check for partial prefix of "</thought>"
				partialPrefix := false
				tag := "</thought>"
				for i := 1; i < len(tag); i++ {
					if strings.HasSuffix(sf.buffer, tag[:i]) {
						toCollect := sf.buffer[:len(sf.buffer)-i]
						if len(toCollect) > 0 {
							sf.thoughtWriter.Write(toCollect)
							sf.thoughtBuf.WriteString(toCollect)
							sf.buffer = sf.buffer[len(sf.buffer)-i:]
						}
						partialPrefix = true
						break
					}
				}
				if !partialPrefix {
					sf.thoughtWriter.Write(sf.buffer)
					sf.thoughtBuf.WriteString(sf.buffer)
					sf.buffer = ""
				}
				break
			} else {
				// Collect everything before "</thought>"
				thoughtContent := sf.buffer[:idx]
				sf.thoughtWriter.Write(thoughtContent)
				sf.thoughtWriter.Close()
				sf.thoughtBuf.WriteString(thoughtContent)
				sf.inThought = false
				sf.buffer = sf.buffer[idx+len("</thought>"):]
			}
		}
	}
	if hasNewText {
		sf.render()
	}
}

func (sf *StreamFilter) Flush() {
	hasNewText := false
	if len(sf.buffer) > 0 {
		if !sf.inThought {
			sf.textBuf.WriteString(sf.buffer)
			hasNewText = true
		} else {
			sf.thoughtWriter.Write(sf.buffer)
			sf.thoughtWriter.Close()
			sf.thoughtBuf.WriteString(sf.buffer)
		}
		sf.buffer = ""
	}
	if hasNewText || sf.prevLineCount > 0 {
		sf.render()
	}
}

func (sf *StreamFilter) Text() string {
	return sf.textBuf.String()
}

// ThoughtWriter handles formatting and writing thoughts stream with a gray left border.
type ThoughtWriter struct {
	started   bool
	thinkMode bool
	renderer  *Renderer
}

func NewThoughtWriter(thinkMode bool, r *Renderer) *ThoughtWriter {
	return &ThoughtWriter{
		thinkMode: thinkMode,
		renderer:  r,
	}
}

func (tw *ThoughtWriter) Write(text string) {
	if !tw.thinkMode || text == "" {
		return
	}
	if !tw.started {
		tw.renderer.ForceBreak()
		tw.renderer.WriteANSI("\033[33m")
		width := tw.renderer.GetContentWidth()
		rem := width - 14
		if rem < 0 {
			rem = 0
		}
		border := "┌── Thoughts " + strings.Repeat("─", rem) + "┐"
		tw.renderer.WriteString(border)
		tw.renderer.WriteANSI("\033[0m")
		tw.renderer.ForceBreak()

		tw.renderer.SetLinePrefix("\033[90m│ \033[0m", 2)
		tw.started = true
	}

	tw.renderer.WriteString(text)
}

func (tw *ThoughtWriter) Close() {
	if tw.thinkMode && tw.started {
		tw.renderer.ForceBreak()
		tw.renderer.SetLinePrefix("", 0)

		tw.renderer.WriteANSI("\033[33m")
		width := tw.renderer.GetContentWidth()
		border := "└" + strings.Repeat("─", width-2) + "┘"
		tw.renderer.WriteString(border)
		tw.renderer.WriteANSI("\033[0m")
		tw.renderer.ForceBreak()

		tw.started = false
	}
}

var (
	currentToolName string
	currentToolArgs map[string]interface{}
)

// formatArgsForBox formats map arguments for neat box printing.
func formatArgsForBox(args map[string]interface{}) []string {
	var lines []string
	if len(args) == 0 {
		return []string{"  (No arguments)"}
	}
	
	// Extract and sort keys for deterministic order
	var keys []string
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, k := range keys {
		v := args[k]
		var valStr string
		switch val := v.(type) {
		case string:
			// Replace newlines with escaped \n
			escaped := strings.ReplaceAll(val, "\n", "\\n")
			escaped = strings.ReplaceAll(escaped, "\r", "")
			if len(escaped) > 60 {
				valStr = fmt.Sprintf("%q", escaped[:57]+"...")
			} else {
				valStr = fmt.Sprintf("%q", escaped)
			}
		default:
			valStr = fmt.Sprintf("%v", val)
		}
		lines = append(lines, fmt.Sprintf("    %s: %s", k, valStr))
	}
	return lines
}

// PrintWritingToolCallScreen displays a beautiful orange header and status box when a tool call starts.
func PrintWritingToolCallScreen() {
	width := getBoxWidth()
	borderCol := "\033[38;5;208m" // Orange
	resetCol := "\033[0m"
	textCol := "\033[1;33m" // Yellow text
	
	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("┌", "─", " WRITING TOOL CALL ", width, "┐") + resetCol)
	line := "  🤖 " + textCol + "AI is formulating and writing a tool call..." + resetCol
	fmt.Printf("%s│%s%s%s│%s\n", borderCol, resetCol, padVisual(line, width), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("└", "─", width, "┘") + resetCol)
	fmt.Println()
}

// PrintToolStart prints a minimal running indicator and saves details.
func PrintToolStart(name string, args map[string]interface{}) {
	currentToolName = name
	currentToolArgs = args

	nameCol := "\033[1;36m"  // Cyan
	resetCol := "\033[0m"
	fmt.Printf("  %s⚙ Running tool:%s %s... ", resetCol, nameCol, name)
	fmt.Printf("\033[33mRUNNING...\033[0m\r")
}

// PrintToolEnd overwrites the RUNNING indicator with the final styled visual box.
func PrintToolEnd(success bool, resultDetails string) {
	// Clear the RUNNING indicator line
	fmt.Print("\r\033[K")

	width := getBoxWidth()
	borderCol := "\033[38;5;33m" // Neon/Cool Blue border
	resetCol := "\033[0m"
	
	// Print top border
	fmt.Println(borderCol + drawBoxHeader("┌", "─", " TOOL CALL EXECUTED ", width, "┐") + resetCol)
	
	// Tool Name line
	toolLine := fmt.Sprintf("  Tool: \033[1;36m%s\033[0m", currentToolName)
	fmt.Printf("%s│%s%s%s│%s\n", borderCol, resetCol, padVisual(toolLine, width), borderCol, resetCol)
	
	// Arguments separator
	fmt.Println(borderCol + drawBoxLine("├", "─", width, "┤") + resetCol)
	
	// Arguments lines
	argLines := formatArgsForBox(currentToolArgs)
	for _, argLine := range argLines {
		fmt.Printf("%s│%s%s%s│%s\n", borderCol, resetCol, padVisual(argLine, width), borderCol, resetCol)
	}
	
	// Status separator
	fmt.Println(borderCol + drawBoxLine("├", "─", width, "┤") + resetCol)
	
	// Status line
	statusStr := "\033[1;32mSUCCESS\033[0m"
	if !success {
		statusStr = "\033[1;31mFAILED\033[0m"
	}
	if resultDetails != "" {
		// Truncate details if they are too long for one line
		cleanDetails := resultDetails
		if len(cleanDetails) > 50 {
			cleanDetails = cleanDetails[:47] + "..."
		}
		statusStr += fmt.Sprintf(" (%s)", cleanDetails)
	}
	statusLine := fmt.Sprintf("  Status: %s", statusStr)
	fmt.Printf("%s│%s%s%s│%s\n", borderCol, resetCol, padVisual(statusLine, width), borderCol, resetCol)
	
	// Bottom border
	fmt.Println(borderCol + drawBoxLine("└", "─", width, "┘") + resetCol)
	fmt.Println()
}

// parseToolResult extracts success status and details from a tool's output interface.
func parseToolResult(result interface{}) (bool, string) {
	if result == nil {
		return true, ""
	}
	m, ok := result.(map[string]interface{})
	if !ok {
		return true, fmt.Sprintf("%v", result)
	}
	
	if status, ok := m["status"].(string); ok {
		msg, _ := m["message"].(string)
		return status == "success", msg
	}
	
	if ec, ok := m["exitCode"].(int); ok {
		success := ec == 0
		details := fmt.Sprintf("exit code: %d", ec)
		if trunc, ok := m["truncated"].(bool); ok && trunc {
			details += ", output truncated"
		}
		return success, details
	}
	return true, ""
}
