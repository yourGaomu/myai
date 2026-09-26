package command

type TurnHookKind string

const (
	TurnHookSessionStart     TurnHookKind = "session_start"
	TurnHookUserPromptSubmit TurnHookKind = "user_prompt_submit"
	TurnHookStop             TurnHookKind = "stop"
)

type TurnHook struct {
	Kind          TurnHookKind
	SessionID     string
	Prompt        string
	LastAssistant string
}

type TurnHookOutcome struct {
	Denied       bool
	Continuation string
}
