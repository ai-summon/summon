package platform

import "fmt"

// MockCmdRunner records CLI commands for test assertions.
type MockCmdRunner struct {
	Calls  []MockCall
	// Err, if non-nil, is returned for every call.
	Err    error
	// CallErrors maps "name arg1 arg2" → error for selective failures.
	CallErrors map[string]error
}

// MockCall records a single CLI invocation.
type MockCall struct {
	Name string
	Args []string
}

func (m *MockCmdRunner) Run(name string, args ...string) (string, string, error) {
	m.Calls = append(m.Calls, MockCall{Name: name, Args: args})

	key := name + " " + joinArgs(args)
	if m.CallErrors != nil {
		if err, ok := m.CallErrors[key]; ok {
			return "", "", err
		}
	}
	if m.Err != nil {
		return "", "", fmt.Errorf("mock error: %w", m.Err)
	}
	return "", "", nil
}

// FindCall returns the first call matching the given command name prefix.
func (m *MockCmdRunner) FindCall(name string, argPrefix ...string) *MockCall {
	for i := range m.Calls {
		if m.Calls[i].Name != name {
			continue
		}
		if len(argPrefix) == 0 {
			return &m.Calls[i]
		}
		match := true
		for j, prefix := range argPrefix {
			if j >= len(m.Calls[i].Args) || m.Calls[i].Args[j] != prefix {
				match = false
				break
			}
		}
		if match {
			return &m.Calls[i]
		}
	}
	return nil
}

// CallCount returns the number of calls matching the given command name.
func (m *MockCmdRunner) CallCount(name string) int {
	count := 0
	for _, c := range m.Calls {
		if c.Name == name {
			count++
		}
	}
	return count
}

// Reset clears all recorded calls.
func (m *MockCmdRunner) Reset() {
	m.Calls = nil
}
