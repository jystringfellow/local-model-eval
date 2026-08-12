package association

import (
	"regexp"
	"strings"
)

// wholePhrase reports whether phrase occurs case-insensitively without being
// embedded inside a larger alphanumeric token.
func wholePhrase(text, phrase string) bool {
	phrase = strings.TrimSpace(phrase)
	if phrase == "" {
		return false
	}
	re := regexp.MustCompile(`(?i)(^|[^[:alnum:]])` + regexp.QuoteMeta(phrase) + `([^[:alnum:]]|$)`)
	return re.FindStringIndex(text) != nil
}

func ticket(text, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	re := regexp.MustCompile(`(?i)(^|[^[:alnum:]])` + regexp.QuoteMeta(code) + `-[[:alnum:]]+`)
	return re.FindStringIndex(text) != nil
}
