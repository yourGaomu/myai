package interaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Question struct {
	ID         string
	Prompt     string
	Options    []string
	AllowOther bool
}

type Request struct {
	ID              string
	SessionID       string
	SourceSessionID string
	SourceTaskID    string
	RequestID       string
	RunID           string
	ToolCallID      string
	Title           string
	Questions       []Question
}

type Answer struct {
	Answers   map[string]string
	Cancelled bool
}

type UserQuestionHandler func(ctx context.Context, request Request) (Answer, error)

// 1. 在领域边界校验提问内容，避免不完整的问题进入交互流程。
func ValidateRequest(request Request) error {
	if strings.TrimSpace(request.ID) == "" {
		return errors.New("question id is required")
	}
	if len(request.Questions) == 0 || len(request.Questions) > 5 {
		return errors.New("between one and five questions are required")
	}
	seen := make(map[string]struct{}, len(request.Questions))
	for _, question := range request.Questions {
		id := strings.TrimSpace(question.ID)
		if id == "" || strings.TrimSpace(question.Prompt) == "" {
			return errors.New("question id and prompt are required")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate question id %q", id)
		}
		seen[id] = struct{}{}
		if len(question.Options) > 8 {
			return fmt.Errorf("question %q has more than eight options", id)
		}
		optionSet := make(map[string]struct{}, len(question.Options))
		for _, option := range question.Options {
			option = strings.TrimSpace(option)
			if option == "" {
				return fmt.Errorf("question %q has an empty option", id)
			}
			if _, exists := optionSet[option]; exists {
				return fmt.Errorf("question %q has duplicate option %q", id, option)
			}
			optionSet[option] = struct{}{}
		}
	}
	return nil
}

func ValidateAnswer(request Request, answer Answer) error {
	if answer.Cancelled {
		return nil
	}
	if len(answer.Answers) != len(request.Questions) {
		return errors.New("an answer is required for every question")
	}
	for _, question := range request.Questions {
		value := strings.TrimSpace(answer.Answers[question.ID])
		if value == "" {
			return fmt.Errorf("answer for question %q is empty", question.ID)
		}
		if len(question.Options) == 0 || question.AllowOther {
			continue
		}
		matched := false
		for _, option := range question.Options {
			if value == option {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("answer for question %q is not one of the offered options", question.ID)
		}
	}
	for id := range answer.Answers {
		found := false
		for _, question := range request.Questions {
			if question.ID == id {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("answer contains unknown question id %q", id)
		}
	}
	return nil
}
