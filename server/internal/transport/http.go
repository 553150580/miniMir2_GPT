package transport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"io"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/game"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type Server struct {
	Store   *persistence.Store
	Mode    string
	NodeKey string
	Engine  *game.Engine
	active  atomic.Int64
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]any{"status": "ok", "mode": s.Mode, "development_only": true, "runtime_ready": false, "instance": s.Store.Instance, "rule_version": s.Store.Cfg.Fixture.RuleVersion, "fixture_hash": s.Store.Cfg.Fixture.Hash})
	})
	if s.Mode == "account" {
		mux.HandleFunc("POST /rpc", s.rpc)
		mux.HandleFunc("POST /internal/register-game", s.registerGame)
		mux.HandleFunc("POST /api/register", s.jsonLogin(true))
		mux.HandleFunc("POST /api/login", s.jsonLogin(false))
		mux.HandleFunc("GET /api/roles", s.jsonRoles)
		mux.HandleFunc("POST /api/roles/create", s.jsonCreate)
		mux.HandleFunc("POST /api/ticket", s.jsonTicket)
	}
	if s.Mode == "game" {
		mux.HandleFunc("GET /ws", s.ws)
		mux.HandleFunc("GET /api/role", s.jsonRole)
	}
	if s.Mode == "gm" {
		mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Dev-Node-Key")), []byte(s.NodeKey)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
			roles, _ := s.Store.DB.Collection("roles").EstimatedDocumentCount(r.Context())
			respond(w, map[string]any{"mode": "gm", "role_count": roles, "maintenance_write_endpoints": "disabled", "production_ready": false})
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			u, e := url.Parse(origin)
			if e != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
				http.Error(w, "loopback origins only", 403)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, wire.MaxFrame)
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}
func jsonError(w http.ResponseWriter, e error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)
	respond(w, map[string]any{"error_code": persistence.Code(e), "message": e.Error()})
}
func (s *Server) auth(r *http.Request) (persistence.Auth, error) {
	return s.Store.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}
func (s *Server) rpc(w http.ResponseWriter, r *http.Request) {
	body, e := io.ReadAll(r.Body)
	if e != nil {
		http.Error(w, "frame too large", 400)
		return
	}
	f, e := wire.Decode(body)
	if e != nil {
		http.Error(w, "invalid frame", 400)
		return
	}
	var out proto.Message
	id := f.ID
	ctx := r.Context()
	decode := func(m proto.Message) error {
		if e := proto.Unmarshal(f.Payload, m); e != nil {
			return persistence.Fail(p.CodeInvalidArgument, "PB字段不合法")
		}
		return nil
	}
	switch f.ID {
	case p.MsgC2SLocalLogin, p.MsgC2SLocalRegister:
		var q pb.LocalLoginRequest
		e = decode(&q)
		if e == nil {
			out, e = s.Store.Login(ctx, q.Account, q.Password, f.ID == p.MsgC2SLocalRegister)
		}
		id = p.MsgS2CLogin
		if f.ID == p.MsgC2SLocalRegister {
			id = p.MsgS2CLocalRegister
		}
	case p.MsgC2SRoleDirectory:
		var q pb.RoleDirectoryRequest
		e = decode(&q)
		var a persistence.Auth
		if e == nil {
			a, e = s.auth(r)
		}
		if e == nil {
			out, e = s.Store.Roles(ctx, a, q.OriginServerId)
		}
		id = p.MsgS2CRoleDirectory
	case p.MsgC2SCreateRole:
		var q pb.CreateRoleRequest
		e = decode(&q)
		var a persistence.Auth
		if e == nil {
			a, e = s.auth(r)
		}
		if e == nil {
			out, e = s.Store.CreateRole(ctx, a, &q)
		}
		id = p.MsgS2CCreateRole
	case p.MsgC2SEntryTicket:
		var q pb.EntryTicketRequest
		e = decode(&q)
		var a persistence.Auth
		if e == nil {
			a, e = s.auth(r)
		}
		if e == nil {
			out, e = s.Store.IssueTicket(ctx, a, q.Role)
		}
		id = p.MsgS2CEntryTicket
	default:
		e = persistence.Fail(p.CodeInvalidArgument, "此节点不处理该消息")
	}
	result := wire.Frame{ID: id, Request: f.Request, Error: persistence.Code(e)}
	if out != nil {
		result.Payload, _ = proto.Marshal(out)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(wire.Encode(result))
}
func (s *Server) registerGame(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Dev-Node-Key")), []byte(s.NodeKey)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	var q struct {
		Node     string `json:"node_id"`
		Instance string `json:"instance_id"`
		Run      uint64 `json:"run_id"`
	}
	if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
		http.Error(w, "invalid registration", 400)
		return
	}
	run, e := s.Store.ApproveGame(r.Context(), q.Node, q.Instance, q.Run)
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, run)
}
func (s *Server) jsonLogin(register bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var q pb.LocalLoginRequest
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			jsonError(w, e)
			return
		}
		out, e := s.Store.Login(r.Context(), q.Account, q.Password, register)
		if e != nil {
			jsonError(w, e)
			return
		}
		respond(w, map[string]any{"account_id": domain.Hex(out.AccountId), "account_token": out.AccountToken, "expires_at_unix": out.ExpiresAtUnix})
	}
}
func (s *Server) jsonRoles(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	out, e := s.Store.Roles(r.Context(), a, s.Store.Cfg.OriginID)
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, out)
}
func (s *Server) jsonCreate(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	var q struct {
		Origin uint64 `json:"origin_server_id"`
		Slot   uint32 `json:"slot"`
		Job    uint32 `json:"job"`
		Name   string `json:"name"`
		OpID   string `json:"op_id"`
	}
	if e = json.NewDecoder(r.Body).Decode(&q); e != nil {
		jsonError(w, e)
		return
	}
	out, e := s.Store.CreateRole(r.Context(), a, &pb.CreateRoleRequest{OriginServerId: q.Origin, Slot: q.Slot, Job: q.Job, Name: q.Name, OpId: domain.Bytes(q.OpID)})
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, out)
}
func (s *Server) jsonTicket(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	var q struct {
		Role   string `json:"role_id"`
		Origin uint64 `json:"origin_server_id"`
	}
	if e = json.NewDecoder(r.Body).Decode(&q); e != nil {
		jsonError(w, e)
		return
	}
	out, e := s.Store.IssueTicket(r.Context(), a, &pb.RoleKey{RoleId: domain.Bytes(q.Role), OriginServerId: q.Origin})
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, out)
}
func (s *Server) jsonRole(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	role, e := s.Store.Role(r.Context(), r.URL.Query().Get("role_id"))
	if e != nil || role.AccountID != a.AccountID {
		jsonError(w, persistence.Fail(p.CodeUnauthenticated, "角色不属于该账号"))
		return
	}
	respond(w, role)
}
func Timeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Second)
}
