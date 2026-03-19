package runtime

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type Service struct {
	Sessions *session.MemoryStore
	Tools    *tool.Registry
}

func New(sessions *session.MemoryStore, tools *tool.Registry) *Service {
	return &Service{Sessions: sessions, Tools: tools}
}

func (s *Service) Ping() map[string]any {
	return map[string]any{"pong": true}
}

func (s *Service) CreateSession(title, goal string) session.State {
	return s.Sessions.Create(title, goal)
}

func (s *Service) GetSession(id string) (session.State, error) {
	return s.Sessions.Get(id)
}

func (s *Service) ListTools() []tool.Definition {
	return s.Tools.List()
}

func (s *Service) EnsureTool(name string) error {
	_, ok := s.Tools.Get(name)
	if !ok {
		return errors.New("tool not registered")
	}
	return nil
}
