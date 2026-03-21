package capability

import "errors"

var (
	ErrCapabilityNotFound        = errors.New("capability not found")
	ErrCapabilityDisabled        = errors.New("capability disabled")
	ErrCapabilityVersionNotFound = errors.New("capability version not found")
	ErrCapabilityViewNotFound    = errors.New("capability view not found")
	ErrCapabilityViewDrift       = errors.New("capability view drift detected")
	ErrSnapshotNotFound          = errors.New("capability snapshot not found")
)
