package helpers

import (
	"os"
	"syscall"
)

// MockProcess represents a mock process for testing
type MockProcess struct {
	Pid    int
	Signal os.Signal
}

// MockProcessManager manages mock processes
type MockProcessManager struct {
	Processes map[int]*MockProcess
}

// NewMockProcessManager creates a new mock process manager
func NewMockProcessManager() *MockProcessManager {
	return &MockProcessManager{
		Processes: make(map[int]*MockProcess),
	}
}

// CreateProcess creates a mock process
func (m *MockProcessManager) CreateProcess(pid int) {
	m.Processes[pid] = &MockProcess{
		Pid: pid,
	}
}

// FindProcess finds a mock process
func (m *MockProcessManager) FindProcess(pid int) (*MockProcess, error) {
	if proc, exists := m.Processes[pid]; exists {
		return proc, nil
	}
	return nil, os.ErrNotExist
}

// Signal sends a signal to the mock process
func (p *MockProcess) Signal(sig os.Signal) error {
	p.Signal = sig
	return nil
} 