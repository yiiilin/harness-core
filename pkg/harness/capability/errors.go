package capability

import "errors"

var (
	ErrCapabilityNotFound        = errors.New("capability not found")
	ErrCapabilityDisabled        = errors.New("capability disabled")
	ErrCapabilityVersionNotFound = errors.New("capability version not found")
	ErrSnapshotNotFound          = errors.New("capability snapshot not found")
)
