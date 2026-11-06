package repository

import "errors"

var ErrNotFound = errors.New("repository record not found")
var ErrConflict = errors.New("repository record changed")
