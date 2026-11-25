package interaction

import "testing"

func TestValidateStructuredAnswers(t *testing.T) {
	request := Request{ID: "question-1", Questions: []Question{
		{ID: "scope", Prompt: "Which scope?", Choices: []Option{{Label: "small"}, {Label: "large"}}},
		{ID: "detail", Prompt: "Anything else?", AllowOther: true},
	}}
	if err := ValidateRequest(request); err != nil {
		t.Fatal(err)
	}
	for name, answer := range map[string]Answer{
		"missing question": {Structured: map[string]QuestionAnswer{"scope": {Selected: []string{"small"}}}},
		"unknown option":   {Structured: map[string]QuestionAnswer{"scope": {Selected: []string{"medium"}}, "detail": {Other: "none"}}},
		"unknown question": {Structured: map[string]QuestionAnswer{"scope": {Selected: []string{"small"}}, "detail": {Other: "ok"}, "unknown": {Other: "extra"}}},
		"empty":            {Structured: map[string]QuestionAnswer{"scope": {Selected: []string{"small"}}, "detail": {Other: "  "}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAnswer(request, answer); err == nil {
				t.Fatal("invalid structured answer was accepted")
			}
		})
	}
	answer := Answer{Structured: map[string]QuestionAnswer{"scope": {Selected: []string{"large"}}, "detail": {Other: "include tests"}}}
	if err := ValidateAnswer(request, answer); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMultiSelectAllowsOtherAndRejectsSingleSelectCombination(t *testing.T) {
	multi := Request{ID: "q", Questions: []Question{{ID: "features", Prompt: "Features?", MultiSelect: true, AllowOther: true, Choices: []Option{{Label: "export"}, {Label: "search"}}}}}
	if err := ValidateAnswer(multi, Answer{Structured: map[string]QuestionAnswer{"features": {Selected: []string{"export", "search"}, Other: "dark mode"}}}); err != nil {
		t.Fatal(err)
	}
	single := Request{ID: "q", Questions: []Question{{ID: "style", Prompt: "Style?", AllowOther: true, Choices: []Option{{Label: "dark"}}}}}
	if err := ValidateAnswer(single, Answer{Structured: map[string]QuestionAnswer{"style": {Selected: []string{"dark"}, Other: "custom"}}}); err == nil {
		t.Fatal("single choice accepted a combined custom answer")
	}
	if err := ValidateAnswer(single, Answer{Structured: map[string]QuestionAnswer{"style": {Selected: []string{"dark", "dark"}}}}); err != nil {
		t.Fatalf("duplicate selection should normalize away: %v", err)
	}
}

func TestValidateRequestAndAnswerRejectInvalidChoices(t *testing.T) {
	for _, question := range []Question{
		{ID: "q", Prompt: "Pick", Choices: []Option{{Label: ""}}},
		{ID: "q", Prompt: "Pick", Choices: []Option{{Label: "A"}, {Label: " A "}}},
		{ID: "q", Prompt: "Pick", Choices: []Option{{Label: "A", Recommended: true}, {Label: "B", Recommended: true}}},
	} {
		if err := ValidateRequest(Request{ID: "r", Questions: []Question{question}}); err == nil {
			t.Fatalf("invalid request accepted: %#v", question)
		}
	}
	request := Request{ID: "r", Questions: []Question{{ID: "q", Prompt: "Open", AllowOther: false, Choices: []Option{{Label: "A"}}}}}
	if err := ValidateAnswer(request, Answer{Structured: map[string]QuestionAnswer{"q": {Other: "custom"}}}); err == nil {
		t.Fatal("custom text was accepted while disabled")
	}
	if err := ValidateAnswer(request, Answer{Cancelled: true}); err != nil {
		t.Fatal(err)
	}
}
