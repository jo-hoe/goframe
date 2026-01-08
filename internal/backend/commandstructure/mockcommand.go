package commandstructure

// mockCommand is a simple mock implementation of the Command interface for testing
type mockCommand struct {
	name        string
	executeFunc func([]byte) ([]byte, error)
}

func (m *mockCommand) Name() string {
	return m.name
}

func (m *mockCommand) Execute(imageData []byte) ([]byte, error) {
	if m.executeFunc != nil {
		return m.executeFunc(imageData)
	}
	return imageData, nil
}

// newMockCommand creates a mock command with default behavior (pass-through)
func newMockCommand(name string) *mockCommand {
	return &mockCommand{
		name: name,
		executeFunc: func(data []byte) ([]byte, error) {
			return data, nil
		},
	}
}

// newMockCommandWithError creates a mock command that returns an error
func newMockCommandWithError(name string, err error) *mockCommand {
	return &mockCommand{
		name: name,
		executeFunc: func(data []byte) ([]byte, error) {
			return nil, err
		},
	}
}
