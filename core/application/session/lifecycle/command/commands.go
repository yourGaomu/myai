package command

type CreateSession struct {
	Title string
}

type LoadSession struct {
	SessionID string
}

type DeleteSession struct {
	SessionID string
}

type RestoreSession struct {
	SessionID string
}

type ClearSession struct {
	Title string
}
