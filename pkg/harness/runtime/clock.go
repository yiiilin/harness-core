package runtime

import "time"

type Clock interface {
	NowMilli() int64
}

type systemClock struct{}

func (systemClock) NowMilli() int64 {
	return time.Now().UnixMilli()
}

func (s *Service) nowMilli() int64 {
	if s == nil || s.Clock == nil {
		return time.Now().UnixMilli()
	}
	return s.Clock.NowMilli()
}
