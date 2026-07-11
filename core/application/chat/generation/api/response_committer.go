package api

import (
	generationcommand "myai/core/application/chat/generation/command"
	generationresult "myai/core/application/chat/generation/result"
)

type ResponseCommitter interface {
	Commit(command generationcommand.Commit) (generationresult.Commit, error)
}
