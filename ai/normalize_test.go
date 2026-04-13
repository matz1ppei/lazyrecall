package ai

import "testing"

func TestMatchAnswer(t *testing.T) {
	tests := []struct {
		input, correct string
		want           bool
	}{
		{"cancion", "canción", true},
		{"HOLA", "hola", true},
		{"uber", "über", true},
		{"hello", "world", false},
		{"  hola  ", "hola", true},
		{"", "", true},
	}
	for _, tc := range tests {
		got := MatchAnswer(tc.input, tc.correct)
		if got != tc.want {
			t.Errorf("MatchAnswer(%q, %q) = %v, want %v", tc.input, tc.correct, got, tc.want)
		}
	}
}
