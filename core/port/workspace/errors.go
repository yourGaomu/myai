package workspace

import "errors"

var ErrConflict = errors.New("workspace changes conflict with the current source workspace")
