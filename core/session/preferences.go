package session

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const MaxStyleInstructionRunes = 2000

func NormalizeStyleInstruction(instruction string) (string, error) {
	normalized := strings.TrimSpace(instruction)
	if utf8.RuneCountInString(normalized) > MaxStyleInstructionRunes {
		return "", fmt.Errorf("style instruction must not exceed %d characters", MaxStyleInstructionRunes)
	}
	return normalized, nil
}
