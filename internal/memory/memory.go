// Package memory manages AGENTS.md memory files for microclaw.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager handles reading and writing AGENTS.md memory files.
type Manager struct {
	dataDir string
}

// New creates a new memory Manager rooted at dataDir.
func New(dataDir string) *Manager {
	return &Manager{dataDir: dataDir}
}

// globalMemoryPath returns the path to the global AGENTS.md.
func (m *Manager) globalMemoryPath() string {
	return filepath.Join(m.dataDir, "runtime", "groups", "AGENTS.md")
}

// chatMemoryPath returns the path to the per-chat AGENTS.md.
func (m *Manager) chatMemoryPath(chatID int64) string {
	return filepath.Join(m.dataDir, "runtime", "groups", fmt.Sprintf("%d", chatID), "AGENTS.md")
}

// ReadMemory reads and concatenates global + per-chat AGENTS.md content.
func (m *Manager) ReadMemory(chatID int64) string {
	var parts []string

	global, err := os.ReadFile(m.globalMemoryPath())
	if err == nil && len(global) > 0 {
		parts = append(parts, "## Global Memory\n"+string(global))
	}

	if chatID != 0 {
		chat, err := os.ReadFile(m.chatMemoryPath(chatID))
		if err == nil && len(chat) > 0 {
			parts = append(parts, fmt.Sprintf("## Chat Memory (chat %d)\n%s", chatID, string(chat)))
		}
	}

	return strings.Join(parts, "\n\n")
}

// WriteMemory writes content to either the global or per-chat AGENTS.md.
func (m *Manager) WriteMemory(chatID int64, content string, global bool) error {
	var path string
	if global {
		path = m.globalMemoryPath()
	} else {
		path = m.chatMemoryPath(chatID)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating memory directory: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// AppendMemory appends content to the memory file.
func (m *Manager) AppendMemory(chatID int64, content string, global bool) error {
	existing := ""
	var path string
	if global {
		path = m.globalMemoryPath()
	} else {
		path = m.chatMemoryPath(chatID)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		existing = string(data)
	}
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return m.WriteMemory(chatID, existing+content, global)
}
