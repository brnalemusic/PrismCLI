package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ChatMessage represents a single message in the history.
type ChatMessage struct {
	Role      string `json:"role"`      // "user", "model", "system"
	Content   string `json:"content"`   // Text content
	Model     string `json:"model"`     // Engine used, if applicable
	Timestamp int64  `json:"timestamp"` // Unix timestamp
}

// ChatSession represents a saved conversation.
type ChatSession struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	LastModified int64         `json:"lastModified"`
	Messages     []ChatMessage `json:"messages"`
}

// GetChatsDir returns the directory where chat sessions are saved.
func GetChatsDir() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(configDir, "chats")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// GetChatPath returns the path to a specific chat session file.
func GetChatPath(sessionID string) (string, error) {
	dir, err := GetChatsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionID+".json"), nil
}

// SaveChatSession writes the chat session to disk.
func SaveChatSession(session ChatSession) error {
	path, err := GetChatPath(session.ID)
	if err != nil {
		return err
	}

	session.LastModified = time.Now().Unix()

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(session)
}

// LoadChatSession reads a chat session from disk.
func LoadChatSession(sessionID string) (ChatSession, error) {
	path, err := GetChatPath(sessionID)
	if err != nil {
		return ChatSession{}, err
	}

	file, err := os.Open(path)
	if err != nil {
		return ChatSession{}, err
	}
	defer file.Close()

	var session ChatSession
	if err := json.NewDecoder(file).Decode(&session); err != nil {
		return ChatSession{}, err
	}
	return session, nil
}

// ListChatSessions returns a list of all saved chat sessions sorted by last modified descending.
func ListChatSessions() ([]ChatSession, error) {
	dir, err := GetChatsDir()
	if err != nil {
		return nil, err
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []ChatSession
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".json") {
			id := f.Name()[:len(f.Name())-5]
			session, err := LoadChatSession(id)
			if err == nil {
				sessions = append(sessions, session)
			}
		}
	}

	// Sort by LastModified desc
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified > sessions[j].LastModified
	})

	return sessions, nil
}

// MatchResult represents a matched Q&A pair with a score.
type MatchResult struct {
	ChatTitle string
	Timestamp int64
	Question  string
	Answer    string
	Score     int
}

// SearchChatHistory scans all saved chats for keywords and returns the top 10 matched Q&A pairs.
func SearchChatHistory(query string) []MatchResult {
	sessions, err := ListChatSessions()
	if err != nil {
		return nil
	}

	// Decompose query into keywords
	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return nil
	}

	var matches []MatchResult

	for _, s := range sessions {
		// Iterate over messages. Since history stores sequentially, we look for user question
		// immediately followed by a model answer.
		for i := 0; i < len(s.Messages)-1; i++ {
			msg := s.Messages[i]
			nextMsg := s.Messages[i+1]

			if msg.Role == "user" && nextMsg.Role == "model" {
				score := 0
				contentLower := strings.ToLower(msg.Content + " " + nextMsg.Content)

				// Calculate frequency of keywords
				for _, kw := range keywords {
					score += strings.Count(contentLower, kw)
				}

				if score > 0 {
					matches = append(matches, MatchResult{
						ChatTitle: s.Title,
						Timestamp: msg.Timestamp,
						Question:  msg.Content,
						Answer:    nextMsg.Content,
						Score:     score,
					})
				}
			}
		}
	}

	// Sort matches by score desc
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Limit to top 10
	if len(matches) > 10 {
		matches = matches[:10]
	}

	return matches
}

// FormatSearchResults formats the matched Q&A pairs into a string for injection.
func FormatSearchResults(matches []MatchResult) string {
	if len(matches) == 0 {
		return "No matching previous history found."
	}

	var sb strings.Builder
	sb.WriteString("=== RELEVANT CHAT HISTORY CONTEXT ===\n\n")

	for idx, m := range matches {
		t := time.Unix(m.Timestamp, 0).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("[%d] Chat: \"%s\" on %s (Relevance: %d)\n", idx+1, m.ChatTitle, t, m.Score))
		sb.WriteString(fmt.Sprintf("User Question: %s\n", strings.TrimSpace(m.Question)))
		sb.WriteString(fmt.Sprintf("Prism Response: %s\n", strings.TrimSpace(m.Answer)))
		sb.WriteString("------------------------------------------------------------------\n")
	}

	return sb.String()
}
