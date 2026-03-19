package runtime

import (
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
)

type Info struct {
	Name        string `json:"name"`
	Mode        string `json:"mode"`
	Transport   string `json:"transport"`
	AuthMode    string `json:"auth_mode"`
	StorageMode string `json:"storage_mode"`
}

type Service struct {
	Sessions session.Store
	Tools    *tool.Registry
}

func New(sessions session.Store, tools *tool.Registry) *Service {
	return &Service{Sessions: sessions, Tools: tools}
}

func (s *Service) Ping() map[string]any {
	return map[string]any{"pong": true}
}

func (s *Service) RuntimeInfo() Info {
	return Info{
		Name:        "harness-core",
		Mode:        "kernel-first",
		Transport:   "adapter-defined",
		AuthMode:    "shared-token-v1",
		StorageMode: "in-memory-dev",
	}
}

func (s *Service) CreateSession(title, goal string) session.State {
	return s.Sessions.Create(title, goal)
}

func (s *Service) GetSession(id string) (session.State, error) {
	return s.Sessions.Get(id)
}

func (s *Service) ListSessions() []session.State {
	return s.Sessions.List()
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
