package ai

import "context"

// MockClient is a test double for the Client interface.
type MockClient struct {
	HintResult               string
	HintErr                  error
	ExampleResult            string
	ExampleTranslationResult string
	ExampleErr               error
	TranslationResult        string
	TranslationErr           error
	CardFront                string
	CardBack                 string
	CardHint                 string
	CardErr                  error
}

func (m *MockClient) GenerateHint(_ context.Context, _, _ string) (string, error) {
	return m.HintResult, m.HintErr
}

func (m *MockClient) GenerateExample(_ context.Context, _, _ string) (string, string, error) {
	return m.ExampleResult, m.ExampleTranslationResult, m.ExampleErr
}

func (m *MockClient) GenerateExampleTranslation(_ context.Context, _, _, _ string) (string, error) {
	return m.TranslationResult, m.TranslationErr
}

func (m *MockClient) GenerateCard(_ context.Context, _ string) (string, string, string, error) {
	return m.CardFront, m.CardBack, m.CardHint, m.CardErr
}

func (m *MockClient) GenerateCards(_ context.Context, _ string, rankStart, rankEnd int) ([]GeneratedCard, error) {
	if m.CardErr != nil {
		return nil, m.CardErr
	}
	count := rankEnd - rankStart + 1
	cards := make([]GeneratedCard, count)
	for i := range cards {
		cards[i] = GeneratedCard{Front: m.CardFront, Back: m.CardBack, Hint: m.CardHint}
	}
	return cards, nil
}

func (m *MockClient) GenerateWordList(_ context.Context, _ string, rankStart, rankEnd int, _ []string) ([]WordPair, error) {
	if m.CardErr != nil {
		return nil, m.CardErr
	}
	count := rankEnd - rankStart + 1
	pairs := make([]WordPair, count)
	for i := range pairs {
		pairs[i] = WordPair{Front: m.CardFront, Back: m.CardBack}
	}
	return pairs, nil
}

func (m *MockClient) GenerateCardsForWords(_ context.Context, _ string, words []string) ([]GeneratedCard, error) {
	if m.CardErr != nil {
		return nil, m.CardErr
	}
	cards := make([]GeneratedCard, len(words))
	for i, w := range words {
		cards[i] = GeneratedCard{Front: w, Back: m.CardBack, Hint: m.CardHint}
	}
	return cards, nil
}

func (m *MockClient) GenerateCardsFromWords(_ context.Context, words []WordPair) ([]GeneratedCard, error) {
	if m.CardErr != nil {
		return nil, m.CardErr
	}
	cards := make([]GeneratedCard, len(words))
	for i, w := range words {
		cards[i] = GeneratedCard{Front: w.Front, Back: w.Back, Hint: m.CardHint}
	}
	return cards, nil
}
