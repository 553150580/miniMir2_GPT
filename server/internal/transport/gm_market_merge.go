// 本文件挂载 GM 市场清退阶段命令及只读路由放行证明，真实合服路由不在此处发布。
package transport

import (
	"context"
	"encoding/hex"
	"math"
	"net/http"

	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

type gmMarketPrepareRequest struct {
	PlanID      string `json:"plan_id"`       // 调用方固定的清退计划身份。
	SourceRunID uint64 `json:"source_run_id"` // 保存历史交易归属的源运行服。
	TargetRunID uint64 `json:"target_run_id"` // 预计承接合服路由的目标运行服。
}

type gmMarketVersionRequest struct {
	PlanID          string `json:"plan_id"`          // 原清退计划身份。
	ExpectedVersion int64  `json:"expected_version"` // 操作员已核对的阶段版本。
}

type gmMarketStepRequest struct {
	gmMarketVersionRequest
	RequestID string `json:"request_id"` // 单步命令身份，网络重试应沿用原值。
}

// mountGMMarketMerge 将可写阶段限定在 GM，并保留单独的只读状态和证明查询。
func (s *Server) mountGMMarketMerge(mux *http.ServeMux) {
	mux.HandleFunc("POST "+gmMarketPath+"/prepare", s.gmMarketPrepare)
	mux.HandleFunc("POST "+gmMarketPath+"/freeze", gmMarketPhase(s.Store.FreezeMarketMerge))
	mux.HandleFunc("POST "+gmMarketPath+"/step", s.gmMarketStep)
	mux.HandleFunc("POST "+gmMarketPath+"/seal", gmMarketPhase(s.Store.SealMarketMerge))
	mux.HandleFunc("POST "+gmMarketPath+"/restore", gmMarketPhase(s.Store.RestoreMarketMerge))
	mux.HandleFunc("GET "+gmMarketPath+"/status", s.gmMarketStatus)
	mux.HandleFunc("GET "+gmMarketPath+"/route-release-proof", s.gmMarketRouteReleaseProof)
}

// gmMarketID 确保同一个 128 位标识只有一种小写十六进制编码。
func gmMarketID(value string) bool {
	if len(value) != 32 {
		return false
	}
	raw, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(raw) == value
}

// gmMarketPrepare 固定两服和当时的原路由控制快照。
func (s *Server) gmMarketPrepare(w http.ResponseWriter, r *http.Request) {
	request, ok := gmMarketBody[gmMarketPrepareRequest](w, r, "plan_id", "source_run_id", "target_run_id")
	if !ok {
		return
	}
	if !gmMarketID(request.PlanID) || request.SourceRunID == 0 || request.TargetRunID == 0 ||
		request.SourceRunID == request.TargetRunID || request.SourceRunID > math.MaxInt64 || request.TargetRunID > math.MaxInt64 {
		writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "清退计划或运行服编号不合法")
		return
	}
	plan, err := s.Store.PrepareMarketMerge(r.Context(), request.PlanID, request.SourceRunID, request.TargetRunID)
	gmMarketRespond(w, plan, err)
}

// gmMarketPhase 使用统一的版本身份请求冻结、封账或按原路由恢复服务。
func gmMarketPhase(apply func(context.Context, string, int64) (persistence.MarketMergePlan, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, ok := gmMarketBody[gmMarketVersionRequest](w, r, "plan_id", "expected_version")
		if !ok {
			return
		}
		if !request.valid() {
			writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "清退计划或预期版本不合法")
			return
		}
		plan, err := apply(r.Context(), request.PlanID, request.ExpectedVersion)
		gmMarketRespond(w, plan, err)
	}
}

// valid 核对所有写操作共用的计划和版本。
func (r gmMarketVersionRequest) valid() bool {
	return gmMarketID(r.PlanID) && r.ExpectedVersion > 0
}

// gmMarketStep 一次只请求持久层确认一个清退步骤。
func (s *Server) gmMarketStep(w http.ResponseWriter, r *http.Request) {
	request, ok := gmMarketBody[gmMarketStepRequest](w, r, "plan_id", "expected_version", "request_id")
	if !ok {
		return
	}
	if !request.valid() || !gmMarketID(request.RequestID) {
		writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "清退计划、预期版本或单步请求编号不合法")
		return
	}
	receipt, err := s.Store.StepMarketMerge(r.Context(), request.PlanID, request.ExpectedVersion, request.RequestID)
	gmMarketRespond(w, receipt, err)
}

// gmMarketStatus 只读取原计划及最近的可见清退结果。
func (s *Server) gmMarketStatus(w http.ResponseWriter, r *http.Request) {
	query, ok := gmMarketQuery(r, "plan_id")
	if !ok || !gmMarketID(query.Get("plan_id")) {
		writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "清退计划查询参数不合法")
		return
	}
	plan, err := s.Store.MarketMergeStatus(r.Context(), query.Get("plan_id"))
	gmMarketRespond(w, plan, err)
}

// gmMarketRouteReleaseProof 校验封账版本和原路由仍满足要求，并返回同一封账证明。
func (s *Server) gmMarketRouteReleaseProof(w http.ResponseWriter, r *http.Request) {
	query, ok := gmMarketQuery(r, "plan_id", "expected_version")
	version, valid := gmMarketVersion(query.Get("expected_version"))
	if !ok || !valid || !gmMarketID(query.Get("plan_id")) {
		writeGMMarketError(w, http.StatusBadRequest, p.CodeInvalidArgument, "路由放行证明查询参数不合法")
		return
	}
	plan, err := s.Store.VerifyMarketMergeRouteRelease(r.Context(), query.Get("plan_id"), version)
	gmMarketRespond(w, plan, err)
}

// gmMarketRespond 映射确认结果，保持超时可用原身份重查，不透出可能包含地址或内部控制信息的报错。
func gmMarketRespond(w http.ResponseWriter, receipt any, err error) {
	if err == nil {
		respond(w, receipt)
		return
	}
	code := persistence.Code(err)
	switch code {
	case p.CodeInvalidArgument:
		writeGMMarketError(w, http.StatusBadRequest, code, "清退命令参数不合法")
	case p.CodeUnauthenticated:
		writeGMMarketError(w, http.StatusUnauthorized, code, "GM 节点身份已失效")
	case p.CodeLineForbidden:
		writeGMMarketError(w, http.StatusForbidden, code, "当前节点无清退权限")
	case p.CodeOperationConflict, p.CodeRouteChanged, p.CodeLineRecovering:
		writeGMMarketError(w, http.StatusConflict, code, "请核对清退阶段、预期版本和游戏节点租约")
	default:
		writeGMMarketError(w, http.StatusServiceUnavailable, code, "清退结果暂不可确认，请按原请求身份查询或重试")
	}
}
