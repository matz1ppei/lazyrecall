package ai

import "context"

// MockClient is a test double for the Client interface.
type MockClient struct {
	HintResult string
	HintErr    error
	CardFront  string
	CardBack   string
	CardHint   string
	CardErr    error
}

func (m *MockClient) GenerateHint(_ context.Context, _, _ string) (string, error) {
	return m.HintResult, m.HintErr
}

func (m *MockClient) GenerateCard(_ context.Context, _ string) (string, string, string, error) {
	return m.CardFront, m.CardBack, m.CardHint, m.CardErr
}
