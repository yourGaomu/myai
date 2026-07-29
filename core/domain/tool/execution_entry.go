package tool

import "time"

type ResultStatus string

const (
	ResultStatusSuccess  ResultStatus = "success"
	ResultStatusFailed   ResultStatus = "failed"
	ResultStatusDenied   ResultStatus = "denied"
	ResultStatusTimeout  ResultStatus = "timeout"
	ResultStatusCanceled ResultStatus = "canceled"
)

type ToolOutput struct {
	Content      string
	Status       ResultStatus
	ErrorCode    string
	ErrorMessage string
	Truncated    bool
}

func SuccessOutput(content string) ToolOutput {
	return ToolOutput{Content: content, Status: ResultStatusSuccess}
}

func FailedOutput(status ResultStatus, errorCode string, message string) ToolOutput {
	return ToolOutput{
		Content:      message,
		Status:       status,
		ErrorCode:    errorCode,
		ErrorMessage: message,
	}
}

func (o ToolOutput) Normalized() ToolOutput {
	if o.Status == "" {
		o.Status = ResultStatusSuccess
	}
	if o.Status != ResultStatusSuccess {
		if o.ErrorMessage == "" {
			o.ErrorMessage = o.Content
		}
		if o.Content == "" {
			o.Content = o.ErrorMessage
		}
	}
	return o
}

func (o ToolOutput) Failed() bool {
	return o.Normalized().Status != ResultStatusSuccess
}

type ExecutionEntryKind string

const (
	ExecutionEntryToolCall   ExecutionEntryKind = "tool_call"
	ExecutionEntryToolResult ExecutionEntryKind = "tool_result"
)

type ExecutionEntry struct {
	Kind            ExecutionEntryKind
	SessionID       string
	ToolCallID      string
	ToolName        string
	Arguments       string
	Content         string
	Error           string
	Status          ResultStatus
	ErrorCode       string
	Truncated       bool
	PromptContent   string
	PromptError     string
	PromptTruncated bool
	CreatedAt       time.Time
}
