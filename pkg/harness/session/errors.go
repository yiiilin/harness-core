package session

import "errors"

var ErrSessionNotFound = errors.New("session not found")
var ErrSessionVersionConflict = errors.New("session version conflict")
var ErrSessionLeaseNotHeld = errors.New("session lease not held")
