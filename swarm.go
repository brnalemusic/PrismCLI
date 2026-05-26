package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SwarmMessage represents a message on the blackboard.
type SwarmMessage struct {
	ID        int       `json:"id"`
	AgentName string    `json:"agentName"`
	Role      string    `json:"role"` // "Coordinator" or "Worker"
	Content   string    `json:"content"`
	Status    string    `json:"status"` // "working", "success", "error", "idle"
	Timestamp time.Time `json:"timestamp"`
}

// Blackboard represents the shared memory for the swarm.
type Blackboard struct {
	mu            sync.Mutex
	Messages      []SwarmMessage
	AgentStatuses map[string]string // AgentName -> Status
	msgIdCounter  int
	listeners     []chan struct{}
}

// NewBlackboard creates a new blackboard instance.
func NewBlackboard() *Blackboard {
	return &Blackboard{
		AgentStatuses: make(map[string]string),
		listeners:     make([]chan struct{}, 0),
	}
}

// PostMessage adds a message to the blackboard and notifies listeners.
func (bb *Blackboard) PostMessage(agentName, role, content, status string) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	bb.msgIdCounter++
	msg := SwarmMessage{
		ID:        bb.msgIdCounter,
		AgentName: agentName,
		Role:      role,
		Content:   content,
		Status:    status,
		Timestamp: time.Now(),
	}

	bb.Messages = append(bb.Messages, msg)
	bb.AgentStatuses[agentName] = status

	// Visual output on terminal
	var prefix string
	if role == "Coordinator" {
		prefix = "⬢ \033[1;35mCoordinator\033[0m: "
	} else {
		prefix = fmt.Sprintf("⬡ \033[1;36m%s\033[0m: ", agentName)
	}

	statusStr := "\033[33mworking\033[0m"
	if status == "success" {
		statusStr = "\033[32msuccess\033[0m"
	} else if status == "error" {
		statusStr = "\033[31merror\033[0m"
	}

	lineText := fmt.Sprintf("  %s%s (%s)", prefix, content, statusStr)
	borderCol := "\033[38;5;163m" // Neon Violet
	resetCode := "\033[0m"
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCode, padVisual(lineText, 70), borderCol, resetCode)

	// Notify all waiting listeners
	for _, l := range bb.listeners {
		select {
		case l <- struct{}{}:
		default:
		}
	}
}

// ReadMessages returns a slice of messages from the blackboard.
func (bb *Blackboard) ReadMessages(limit int) []SwarmMessage {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	n := len(bb.Messages)
	if n == 0 {
		return nil
	}

	if limit > n {
		limit = n
	}

	res := make([]SwarmMessage, limit)
	copy(res, bb.Messages[n-limit:])
	return res
}

// GetStatusMap returns the current status of all agents.
func (bb *Blackboard) GetStatusMap() map[string]string {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	statusCopy := make(map[string]string)
	for k, v := range bb.AgentStatuses {
		statusCopy[k] = v
	}
	return statusCopy
}

// WaitForUpdates blocks until a new message is posted or the timeout expires.
func (bb *Blackboard) WaitForUpdates(timeout time.Duration) {
	bb.mu.Lock()
	l := make(chan struct{}, 1)
	bb.listeners = append(bb.listeners, l)
	bb.mu.Unlock()

	defer func() {
		bb.mu.Lock()
		// Remove listener
		for i, listener := range bb.listeners {
			if listener == l {
				bb.listeners = append(bb.listeners[:i], bb.listeners[i+1:]...)
				break
			}
		}
		bb.mu.Unlock()
	}()

	select {
	case <-l:
		return
	case <-time.After(timeout):
		return
	}
}

// SwarmCoordinator runs a simulated agent coordination workflow for CLI
func RunSwarmTask(ctx context.Context, goal string) {
	bb := NewBlackboard()
	borderCol := "\033[38;5;163m" // Neon Violet
	resetCol := "\033[0m"

	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("╔", "═", " SWARM COORDINATION STARTED ", 70, "╗") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(fmt.Sprintf("  Goal: %q", goal), 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)

	// 1. Coordinator plans
	bb.PostMessage("Coordinator", "Coordinator", "Analyzing goal and creating action plan...", "working")
	time.Sleep(1500 * time.Millisecond)

	bb.PostMessage("Coordinator", "Coordinator", "Plan created: Delegate research to Worker 1 (Researcher) and writing to Worker 2 (Writer).", "working")
	time.Sleep(1 * time.Second)

	// 2. Delegate to Worker 1
	bb.PostMessage("Researcher", "Worker", "Searching local and web information about the goal...", "working")
	time.Sleep(2 * time.Second)
	bb.PostMessage("Researcher", "Worker", "Information collected successfully! Summary: Local files analyzed.", "success")
	time.Sleep(1 * time.Second)

	// 3. Coordinator processes worker 1 results
	bb.PostMessage("Coordinator", "Coordinator", "Analyzing Researcher data. Delegating writing to Writer.", "working")
	time.Sleep(1 * time.Second)

	// 4. Delegate to Worker 2
	bb.PostMessage("Writer", "Worker", "Writing files and running test scripts...", "working")
	time.Sleep(2 * time.Second)
	bb.PostMessage("Writer", "Worker", "Modifications written to disk and compiled without errors.", "success")
	time.Sleep(1 * time.Second)

	// 5. Coordinator closes swarm
	bb.PostMessage("Coordinator", "Coordinator", "All workers completed their tasks successfully. Global goal achieved!", "success")
	fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
	fmt.Println()
}

// RunSubagentsSim simulates the parallel execution of multiple subagents for the CLI.
func RunSubagentsSim(quantity int, prompts []string) {
	bb := NewBlackboard()
	borderCol := "\033[38;5;163m" // Neon Violet
	resetCol := "\033[0m"

	fmt.Println()
	fmt.Println(borderCol + drawBoxHeader("╔", "═", " PARALLEL SWARM STARTED ", 70, "╗") + resetCol)
	fmt.Printf("%s║%s%s%s║%s\n", borderCol, resetCol, padVisual(fmt.Sprintf("  Quantity: %d agents", quantity), 70), borderCol, resetCol)
	fmt.Println(borderCol + drawBoxLine("╠", "═", 70, "╣") + resetCol)

	bb.PostMessage("Coordinator", "Coordinator", "Synchronizing agent swarm...", "working")
	time.Sleep(800 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < quantity; i++ {
		agentName := fmt.Sprintf("Agent #%d", i+1)
		prompt := "Generic task"
		if i < len(prompts) {
			prompt = prompts[i]
		}
		wg.Add(1)
		go func(name, p string) {
			defer wg.Done()
			bb.PostMessage(name, "Worker", fmt.Sprintf("Starting task: %q", p), "working")
			time.Sleep(1500 * time.Millisecond)
			bb.PostMessage(name, "Worker", fmt.Sprintf("Processing and executing sub-steps of: %q", p), "working")
			time.Sleep(1500 * time.Millisecond)
			bb.PostMessage(name, "Worker", fmt.Sprintf("Task completed successfully: %q", p), "success")
		}(agentName, prompt)
	}
	wg.Wait()

	bb.PostMessage("Coordinator", "Coordinator", "All subagents successfully completed their tasks. Swarm finalized!", "success")
	fmt.Println(borderCol + drawBoxLine("╚", "═", 70, "╝") + resetCol)
	fmt.Println()
}

