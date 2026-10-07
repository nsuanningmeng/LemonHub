package service

import (
	"errors"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
)

func CheckSensitiveMessages(messages []dto.Message) ([]string, error) {
	if len(messages) == 0 {
		return nil, nil
	}

	for _, message := range messages {
		arrayContent := message.ParseContent()
		for _, m := range arrayContent {
			if m.Type == "image_url" {
				// TODO: check image url
				continue
			}
			// 检查 text 是否为空
			if m.Text == "" {
				continue
			}
			if ok, words := SensitiveWordContains(m.Text); ok {
				return words, errors.New("sensitive words detected")
			}
		}
	}
	return nil, nil
}

func CheckSensitiveText(text string) (bool, []string) {
	return SensitiveWordContains(text)
}

// SensitiveWordContains 是否包含敏感词，返回是否包含敏感词和敏感词列表
func SensitiveWordContains(text string) (bool, []string) {
	if len(setting.SensitiveWords) == 0 {
		return false, nil
	}
	if len(text) == 0 {
		return false, nil
	}
	checkText := strings.ToLower(text)
	if !setting.SensitiveWordsWholeWordEnabled {
		return AcSearch(checkText, setting.SensitiveWords, true)
	}
	m := getOrBuildAC(setting.SensitiveWords)
	if m == nil {
		return false, nil
	}
	// Classify original dictionary entries before lowercasing: Unicode letters
	// such as Kelvin sign can normalize to ASCII without becoming ASCII entries.
	wholeWords := make(map[string]bool, len(setting.SensitiveWords))
	for _, entry := range setting.SensitiveWords {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		asciiLetters := true
		for _, r := range entry {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				asciiLetters = false
				break
			}
		}
		normalized := strings.ToLower(entry)
		previous, exists := wholeWords[normalized]
		wholeWords[normalized] = asciiLetters && (!exists || previous)
	}
	textRunes := []rune(checkText)
	// Rejected substring hits must not hide a later valid whole-word match.
	for _, hit := range m.MultiPatternSearch(textRunes, false) {
		word := string(hit.Word)
		if wholeWords[word] {
			start, end := hit.Pos, hit.Pos+len(hit.Word)
			if start > 0 {
				before := textRunes[start-1]
				if unicode.IsLetter(before) || unicode.IsDigit(before) || before == '_' {
					continue
				}
			}
			if end < len(textRunes) {
				after := textRunes[end]
				if unicode.IsLetter(after) || unicode.IsDigit(after) || after == '_' {
					continue
				}
			}
		}
		return true, []string{word}
	}
	return false, nil
}

// SensitiveWordReplace 敏感词替换，返回是否包含敏感词和替换后的文本
func SensitiveWordReplace(text string, returnImmediately bool) (bool, []string, string) {
	if len(setting.SensitiveWords) == 0 {
		return false, nil, text
	}
	checkText := strings.ToLower(text)
	m := getOrBuildAC(setting.SensitiveWords)
	hits := m.MultiPatternSearch([]rune(checkText), returnImmediately)
	if len(hits) > 0 {
		words := make([]string, 0, len(hits))
		var builder strings.Builder
		builder.Grow(len(text))
		lastPos := 0

		for _, hit := range hits {
			pos := hit.Pos
			word := string(hit.Word)
			builder.WriteString(text[lastPos:pos])
			builder.WriteString("**###**")
			lastPos = pos + len(word)
			words = append(words, word)
		}
		builder.WriteString(text[lastPos:])
		return true, words, builder.String()
	}
	return false, nil, text
}
