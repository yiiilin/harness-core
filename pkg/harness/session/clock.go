package session

import "time"

type Clock interface {
	NowMilli() int64
}

type systemClock struct{}

func (systemClock) NowMilli() int64 {
	return time.Now().UnixMilli()
}
