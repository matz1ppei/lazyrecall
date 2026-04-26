package ai

import "strings"

func resolveSupportLanguage(studyLang string) string {
	switch strings.ToLower(strings.TrimSpace(studyLang)) {
	case "english":
		return "Japanese"
	default:
		return "English"
	}
}
