// 本文件挂载各节点的回环 HTTP 路由，并把账号二进制 RPC、开发 JSON 和内部注册请求转给持久层。

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

// Server 路由当前节点允许的请求，只有游戏节点挂载内存运行时。
type Server struct {
	Store   *persistence.Store // 节点使用的专用库连接及身份。
	Mode    string             // 选择账号、游戏或 GM 路由集合。
	NodeKey string             // 内部登记和 GM 只读接口的共享密钥。
	Engine  *game.Engine       // 游戏节点将已校验的连接交给此运行时。
	active  atomic.Int64       // 游戏连接数，用于升级前的接入配额。
}

// Handler 只挂载当前节点的路由，并统一约束请求来源和体积。
// s 是调用该方法的服务端实例。
func (s *Server) Handler() http.Handler {
	// mux 保存由创建“serve”“mux”取得的“mux”。
	mux := http.NewServeMux()
	// w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
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
		// w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
		mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Dev-Node-Key")), []byte(s.NodeKey)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
			// roles 保存由“estimated”文档数量取得的角色集合。
			roles, _ := s.Store.DB.Collection("roles").EstimatedDocumentCount(r.Context())
			respond(w, map[string]any{"mode": "gm", "role_count": roles, "maintenance_write_endpoints": "disabled", "production_ready": false})
		})
	}
	// w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 开发路由仅服务本机来源。预检提前结束，正式请求再经过帧大小限制。
		// origin 保存由“get”取得的大区或起点。
		origin := r.Header.Get("Origin")
		if origin != "" {
			// u 保存“parse”返回的结果，e 接收“parse”供后续步骤使用。
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

// respond 将返回对象序列化为 JSON 并写入响应。
// w 写入当前 HTTP 响应头和正文，value 是要序列化的响应对象。
func respond(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

// jsonError 将持久层错误转换为带业务错误码的 HTTP 400 JSON 响应。
// w 写入当前 HTTP 响应头和正文，e 记录错误。
func jsonError(w http.ResponseWriter, e error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)
	respond(w, map[string]any{"error_code": persistence.Code(e), "message": e.Error()})
}

// auth 检查 HTTP 请求的 Bearer 凭据，返回通过验证的账号授权。
// s 是调用该方法的服务端实例，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) auth(r *http.Request) (persistence.Auth, error) {
	return s.Store.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// rpc 按共享消息号解码账号请求，保留原请求序号并统一编码业务错误。
// s 是调用该方法的服务端实例，w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) rpc(w http.ResponseWriter, r *http.Request) {
	// body 保存受帧大小限制的完整账号请求体，e 保存读取请求体时出现的错误。
	body, e := io.ReadAll(r.Body)
	if e != nil {
		http.Error(w, "frame too large", 400)
		return
	}
	// f 保存已解析的序号、消息号及账号 Protobuf 请求。
	f, e := wire.Decode(body)
	if e != nil {
		http.Error(w, "invalid frame", 400)
		return
	}
	// out 暂存账号业务返回的 Protobuf 对象。
	var out proto.Message
	// id 选择与请求类型对应的服务端回包消息号。
	id := f.ID
	// ctx 传递 HTTP 请求的截止时间和取消信号。
	ctx := r.Context()
	// decode 封装解码对应的后续处理，m 记录消息。
	decode := func(m proto.Message) error {
		// e 保存 Protobuf 解码错误，供调用端转换为协议错误。
		if e := proto.Unmarshal(f.Payload, m); e != nil {
			return persistence.Fail(p.CodeInvalidArgument, "PB字段不合法")
		}
		return nil
	}
	// 注册与登录直接校验账号输入，其余账号业务都先检查请求携带的凭据。
	switch f.ID {
	case p.MsgC2SLocalLogin, p.MsgC2SLocalRegister:
		// q 记录本地登录请求。
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
		// q 承载角色目录请求中的入口服编号。
		var q pb.RoleDirectoryRequest
		e = decode(&q)
		// a 保存通过账号令牌校验后取得的授权。
		var a persistence.Auth
		if e == nil {
			a, e = s.auth(r)
		}
		if e == nil {
			out, e = s.Store.Roles(ctx, a, q.OriginServerId)
		}
		id = p.MsgS2CRoleDirectory
	case p.MsgC2SCreateRole:
		// q 承载建角的职业、槽位、角色名和原操作标识。
		var q pb.CreateRoleRequest
		e = decode(&q)
		// a 保存通过账号令牌校验后取得的授权。
		var a persistence.Auth
		if e == nil {
			a, e = s.auth(r)
		}
		if e == nil {
			out, e = s.Store.CreateRole(ctx, a, &q)
		}
		id = p.MsgS2CCreateRole
	case p.MsgC2SEntryTicket:
		// q 承载本次取票所选角的身份。
		var q pb.EntryTicketRequest
		e = decode(&q)
		// a 保存通过账号令牌校验后取得的授权。
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
	// result 保存账号返回帧，关联原请求并携带业务错误码。
	result := wire.Frame{ID: id, Request: f.Request, Error: persistence.Code(e)}
	if out != nil {
		result.Payload, _ = proto.Marshal(out)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(wire.Encode(result))
}

// registerGame 核对同机节点密钥，通过账号节点的持久事务批准游戏运行所有权。
// s 是调用该方法的服务端实例，w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) registerGame(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Dev-Node-Key")), []byte(s.NodeKey)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	// q 记录临时结构。
	var q struct {
		// Node 记录节点，使用文本表示。
		Node string `json:"node_id"`
		// Instance 记录进程或道具实例，使用文本表示。
		Instance string `json:"instance_id"`
		// Run 记录运行批次，使用数值表示。
		Run uint64 `json:"run_id"`
	}
	// e 接收解码供后续步骤使用。
	if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
		http.Error(w, "invalid registration", 400)
		return
	}
	// run 保存由批准游戏取得的运行批次，e 接收批准游戏供后续步骤使用。
	run, e := s.Store.ApproveGame(r.Context(), q.Node, q.Instance, q.Run)
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, run)
}

// jsonLogin 返回JSON登录供调用方使用。
// s 是调用该方法的服务端实例，register 指明此处理函数执行注册还是验证存量账号。
func (s *Server) jsonLogin(register bool) http.HandlerFunc {
	// w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
	return func(w http.ResponseWriter, r *http.Request) {
		// q 记录本地登录请求。
		var q pb.LocalLoginRequest
		// e 接收解码供后续步骤使用。
		if e := json.NewDecoder(r.Body).Decode(&q); e != nil {
			jsonError(w, e)
			return
		}
		// out 保存由登录取得的输出，e 接收登录供后续步骤使用。
		out, e := s.Store.Login(r.Context(), q.Account, q.Password, register)
		if e != nil {
			jsonError(w, e)
			return
		}
		respond(w, map[string]any{"account_id": domain.Hex(out.AccountId), "account_token": out.AccountToken, "expires_at_unix": out.ExpiresAtUnix})
	}
}

// jsonRoles 处理JSON角色集合。
// s 是调用该方法的服务端实例，w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) jsonRoles(w http.ResponseWriter, r *http.Request) {
	// a 保存由授权取得的a，e 接收授权供后续步骤使用。
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	// out 保存由角色集合取得的输出。
	out, e := s.Store.Roles(r.Context(), a, s.Store.Cfg.OriginID)
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, out)
}

// jsonCreate 处理开发调试用的 JSON 建角入口，复用账号鉴权和持久建角流程。
// s 是调用该方法的服务端实例，w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) jsonCreate(w http.ResponseWriter, r *http.Request) {
	// a 保存由授权取得的a，e 接收授权供后续步骤使用。
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	// q 记录临时结构。
	var q struct {
		// Origin 记录大区或起点，使用数值表示。
		Origin uint64 `json:"origin_server_id"`
		// Slot 记录槽位，使用数值表示。
		Slot uint32 `json:"slot"`
		// Job 记录任务或职业编号，使用数值表示。
		Job uint32 `json:"job"`
		// Name 记录名称，使用文本表示。
		Name string `json:"name"`
		// OpID 标识当前业务使用的操作。
		OpID string `json:"op_id"`
	}
	if e = json.NewDecoder(r.Body).Decode(&q); e != nil {
		jsonError(w, e)
		return
	}
	// out 保存由创建角色取得的输出。
	out, e := s.Store.CreateRole(r.Context(), a, &pb.CreateRoleRequest{OriginServerId: q.Origin, Slot: q.Slot, Job: q.Job, Name: q.Name, OpId: domain.Bytes(q.OpID)})
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, out)
}

// jsonTicket 处理JSON入场票据。
// s 是调用该方法的服务端实例，w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) jsonTicket(w http.ResponseWriter, r *http.Request) {
	// a 保存由授权取得的a，e 接收授权供后续步骤使用。
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	// q 记录临时结构。
	var q struct {
		// Role 记录角色，使用文本表示。
		Role string `json:"role_id"`
		// Origin 记录大区或起点，使用数值表示。
		Origin uint64 `json:"origin_server_id"`
	}
	if e = json.NewDecoder(r.Body).Decode(&q); e != nil {
		jsonError(w, e)
		return
	}
	// out 保存由签发入场票据取得的输出。
	out, e := s.Store.IssueTicket(r.Context(), a, &pb.RoleKey{RoleId: domain.Bytes(q.Role), OriginServerId: q.Origin})
	if e != nil {
		jsonError(w, e)
		return
	}
	respond(w, out)
}

// jsonRole 处理JSON角色。
// s 是调用该方法的服务端实例，w 写入当前 HTTP 响应头和正文，r 提供当前 HTTP 请求、请求头和取消信号。
func (s *Server) jsonRole(w http.ResponseWriter, r *http.Request) {
	// a 保存由授权取得的a，e 接收授权供后续步骤使用。
	a, e := s.auth(r)
	if e != nil {
		jsonError(w, e)
		return
	}
	// role 接收角色供后续步骤使用。
	role, e := s.Store.Role(r.Context(), r.URL.Query().Get("role_id"))
	if e != nil || role.AccountID != a.AccountID {
		jsonError(w, persistence.Fail(p.CodeUnauthenticated, "角色不属于该账号"))
		return
	}
	respond(w, role)
}

// Timeout 处理超时。
// parent 为超时控制继承上层取消信号。
func Timeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Second)
}
