package runtime

import (
	"context"
	"errors"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type Info struct {
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	Transport     string `json:"transport"`
	AuthMode      string `json:"auth_mode"`
	StorageMode   string `json:"storage_mode"`
	ToolCount     int    `json:"tool_count"`
	VerifierCount int    `json:"verifier_count"`
}

type Service struct {
	Sessions  session.Store
	Tools     *tool.Registry
	Verifiers *verify.Registry
}

func New(sessions session.Store, tools *tool.Registry, verifiers *verify.Registry) *Service {
	return &Service{Sessions: sessions, Tools: tools, Verifiers: verifiers}
}

func (s *Service) Ping() map[string]any {
	return map[string]any{"pong": true}
}

func (s *Service) RuntimeInfo() Info {
	return Info{
		Name:          "harness-core",
		Mode:          "kernel-first",
		Transport:     "adapter-defined",
		AuthMode:      "shared-token-v1",
		StorageMode:   "in-memory-dev",
		ToolCount:     len(s.Tools.List()),
		VerifierCount: len(s.Verifiers.List()),
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

func (s *Service) ListVerifiers() []verify.Definition {
	return s.Verifiers.List()
}

func (s *Service) EnsureTool(name string) error {
	_, ok := s.Tools.Get(name)
	if !ok {
		return errors.New("tool not registered")
	}
	return nil
}

func (s *Service) InvokeAction(ctx context.Context, spec action.Spec) (action.Result, error) {
	return s.Tools.Invoke(ctx, spec)
}
