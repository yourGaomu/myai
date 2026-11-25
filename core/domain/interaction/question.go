package interaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Option struct {
	Label       string
	Description string
	Recommended bool
}

type Question struct {
	ID          string
	Prompt      string
	Choices     []Option
	MultiSelect bool
	AllowOther  bool
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

type QuestionAnswer struct {
	Selected []string
	Other    string
}

type Answer struct {
	Structured      map[string]QuestionAnswer
	AdoptedDefaults bool
	Cancelled       bool
}

type UserQuestionHandler func(ctx context.Context, request Request) (Answer, error)

func ValidateRequest(request Request) error {
	if strings.TrimSpace(request.ID) == "" {
		return errors.New("question id is required")
	}
	if len(request.Questions) == 0 || len(request.Questions) > 5 {
		return errors.New("between one and five questions are required")
	}
	seenQuestions := make(map[string]struct{}, len(request.Questions))
	for _, question := range request.Questions {
		id := strings.TrimSpace(question.ID)
		if id == "" || strings.TrimSpace(question.Prompt) == "" {
			return errors.New("question id and prompt are required")
		}
		if _, exists := seenQuestions[id]; exists {
			return fmt.Errorf("duplicate question id %q", id)
		}
		seenQuestions[id] = struct{}{}
		if len(question.Choices) == 0 && !question.AllowOther {
			return fmt.Errorf("question %q requires a choice or a custom answer", id)
		}
		if len(question.Choices) > 8 {
			return fmt.Errorf("question %q has more than eight choices", id)
		}
		seenChoices := make(map[string]struct{}, len(question.Choices))
		recommended := 0
		for _, choice := range question.Choices {
			label := strings.TrimSpace(choice.Label)
			if label == "" {
				return fmt.Errorf("question %q has an empty choice label", id)
			}
			if _, exists := seenChoices[label]; exists {
				return fmt.Errorf("question %q has duplicate choice %q", id, label)
			}
			seenChoices[label] = struct{}{}
			if choice.Recommended {
				recommended++
			}
		}
		if !question.MultiSelect && recommended > 1 {
			return fmt.Errorf("single-select question %q has more than one recommended choice", id)
		}
	}
	return nil
}

func ValidateAnswer(request Request, answer Answer) error {
	if answer.Cancelled {
		return nil
	}
	if len(answer.Structured) != len(request.Questions) {
		return errors.New("a structured answer is required for every question")
	}
	known := make(map[string]Question, len(request.Questions))
	for _, question := range request.Questions {
		known[question.ID] = question
	}
	for id, entry := range answer.Structured {
		question, exists := known[id]
		if !exists {
			return fmt.Errorf("answer contains unknown question id %q", id)
		}
		selected := cleanSelected(entry.Selected)
		other := strings.TrimSpace(entry.Other)
		if len(selected) == 0 && other == "" {
			return fmt.Errorf("answer for question %q is empty", id)
		}
		if !question.MultiSelect && len(selected) > 1 {
			return fmt.Errorf("single-select question %q cannot select multiple choices", id)
		}
		if !question.MultiSelect && len(selected) > 0 && other != "" {
			return fmt.Errorf("single-select question %q cannot combine a choice and custom text", id)
		}
		allowed := make(map[string]struct{}, len(question.Choices))
		for _, choice := range question.Choices {
			allowed[strings.TrimSpace(choice.Label)] = struct{}{}
		}
		for _, label := range selected {
			if _, ok := allowed[label]; !ok {
				return fmt.Errorf("answer %q for question %q is not an offered choice", label, id)
			}
		}
		if other != "" && !question.AllowOther {
			return fmt.Errorf("custom answer is not allowed for question %q", id)
		}
	}
	return nil
}

func NormalizeAnswer(request Request, answer Answer) Answer {
	if answer.Cancelled {
		return Answer{Cancelled: true}
	}
	normalized := Answer{
		Structured:      make(map[string]QuestionAnswer, len(request.Questions)),
		AdoptedDefaults: answer.AdoptedDefaults,
	}
	for _, question := range request.Questions {
		value := answer.Structured[question.ID]
		normalized.Structured[question.ID] = QuestionAnswer{
			Selected: cleanSelected(value.Selected),
			Other:    strings.TrimSpace(value.Other),
		}
	}
	return normalized
}

func cleanSelected(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}
