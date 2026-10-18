package interaction

import "testing"

func TestValidateAnswerRequiresEveryQuestionAndAllowedChoices(t *testing.T) {
	request := Request{ID: "question-1", Questions: []Question{
		{ID: "scope", Prompt: "Which scope?", Options: []string{"small", "large"}},
		{ID: "detail", Prompt: "Anything else?", AllowOther: true},
	}}
	if err := ValidateRequest(request); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAnswer(request, Answer{Answers: map[string]string{"scope": "small"}}); err == nil {
		t.Fatal("accepted an incomplete answer")
	}
	if err := ValidateAnswer(request, Answer{Answers: map[string]string{"scope": "medium", "detail": "none"}}); err == nil {
		t.Fatal("accepted an answer outside the offered choices")
	}
	if err := ValidateAnswer(request, Answer{Answers: map[string]string{"scope": "large", "detail": "include tests"}}); err != nil {
		t.Fatal(err)
	}
}
