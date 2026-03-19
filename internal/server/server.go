package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/yiiilin/harness-core/internal/auth"
	"github.com/yiiilin/harness-core/internal/config"
	"github.com/yiiilin/harness-core/internal/protocol"
	"github.com/yiiilin/harness-core/internal/runtime"
	"github.com/yiiilin/harness-core/internal/tool"
)

type Server struct {
	cfg      config.Config
	store    *runtime.Store
	registry *tool.Registry
	upgrader websocket.Upgrader
}

func New(cfg config.Config, store *runtime.Store, registry *tool.Registry) *Server {
	return &Server{
		cfg:      cfg,
		store:    store,
		registry: registry,
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/ws", s.ws)
	log.Printf("harness-core listening on %s", s.cfg.Addr)
	return http.ListenAndServe(s.cfg.Addr, mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	authed := false
	if auth.ValidToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), s.cfg.SharedToken) {
		authed = true
	}
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			_ = conn.WriteJSON(protocol.Response{Type: "response", OK: false, Error: &protocol.ErrorBody{Code: "BAD_JSON", Message: err.Error()}})
			continue
		}
		if !authed {
			if env.Type == "auth" && auth.ValidToken(env.Token, s.cfg.SharedToken) {
				authed = true
				_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: true, Result: map[string]any{"authenticated": true}})
				continue
			}
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: false, Error: &protocol.ErrorBody{Code: "UNAUTHENTICATED", Message: "authenticate first"}})
			continue
		}
		s.handle(conn, env)
	}
}

func (s *Server) handle(conn *websocket.Conn, env protocol.Envelope) {
	switch env.Action {
	case "session.ping":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: true, Result: map[string]any{"pong": true}})
	case "session.create":
		var payload protocol.SessionCreatePayload
		_ = json.Unmarshal(env.Payload, &payload)
		sess := s.store.Create(payload.Title, payload.Goal)
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: true, Result: sess})
	case "session.get":
		var payload struct{ ID string `json:"id"` }
		_ = json.Unmarshal(env.Payload, &payload)
		sess, err := s.store.Get(payload.ID)
		if err != nil {
			_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: false, Error: &protocol.ErrorBody{Code: "NOT_FOUND", Message: err.Error()}})
			return
		}
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: true, Result: sess})
	case "tool.list":
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: true, Result: s.registry.List()})
	default:
		_ = conn.WriteJSON(protocol.Response{ID: env.ID, Type: "response", OK: false, Error: &protocol.ErrorBody{Code: "UNKNOWN_ACTION", Message: "unknown action"}})
	}
}
