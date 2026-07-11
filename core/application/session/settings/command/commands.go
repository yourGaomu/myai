package command

type SwitchModel struct {
	SessionID string
	ModelID   string
}

type SetPermissionMode struct {
	SessionID string
	Mode      string
}

type SetAgentMode struct {
	SessionID string
	Mode      string
}

type SetContextWindow struct {
	SessionID string
	WindowK   int
}
