package helpers

import (
	"bytes"
	"io"
	"os"
)

// MockIO provides mock IO functionality for testing
type MockIO struct {
	Stdout *bytes.Buffer
	Stderr *bytes.Buffer
	Files  map[string][]byte
}

// NewMockIO creates a new MockIO instance
func NewMockIO() *MockIO {
	return &MockIO{
		Stdout: new(bytes.Buffer),
		Stderr: new(bytes.Buffer),
		Files:  make(map[string][]byte),
	}
}

// CaptureOutput captures stdout and stderr output
func (m *MockIO) CaptureOutput() func() {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	
	r, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w

	go func() {
		io.Copy(m.Stdout, r)
	}()

	return func() {
		w.Close()
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}
}

// WriteFile mocks file writing
func (m *MockIO) WriteFile(filename string, data []byte, perm os.FileMode) error {
	m.Files[filename] = data
	return nil
}

// ReadFile mocks file reading
func (m *MockIO) ReadFile(filename string) ([]byte, error) {
	if data, ok := m.Files[filename]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
} 