// 本文件让原账号查看观看状态，并独立接收可信渠道的完整播放证明。
package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/rewardedad"
)

// rewardedAdRPC 允许原玩家查看回执或发起观看。到账由可信回调和所属运行服完成。
func (s *Server) rewardedAdRPC(r *http.Request, request *pb.RewardedAdRequest) (*pb.RewardedAdState, error) {
	auth, err := s.auth(r)
	if err != nil {
		return nil, err
	}
	if len(request.RequestId) != 0 && len(request.AttemptId) != 0 ||
		(len(request.AttemptId) != 0 && len(request.AttemptId) != 16) {
		return nil, persistence.Fail(p.CodeInvalidArgument, "请选择一次观看或查看原凭据")
	}
	rules := s.Store.Cfg.Fixture.RewardedAd
	playable := rules != nil && s.AdVerifier.ActiveFor(rules.Provider)
	at, id := time.Now(), domain.Hex(request.AttemptId)
	if len(request.RequestId) != 0 {
		if !playable {
			return nil, persistence.Fail(p.CodeRewardedAdUnavailable, "可信广告渠道尚未开放")
		}
		attempt, err := s.Store.StartRewardedAd(r.Context(), auth, request.Role, request.RequestId, at)
		if err != nil {
			return nil, err
		}
		id = attempt.ID
	}
	return s.Store.ReadRewardedAd(r.Context(), auth, request.Role, id, playable, at)
}

// rewardedAdCallback 仅接收渠道证明，回调发送者的网络地址和原客户端字段不参与结算。
func (s *Server) rewardedAdCallback(w http.ResponseWriter, r *http.Request) {
	const maxNoticeBytes = 16 * 1024 // 渠道原证明的请求体上限。
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	auth := r.Header.Get("Authorization")
	if len(auth) > 2048 {
		http.Error(w, "invalid completion proof", http.StatusUnauthorized)
		return
	}
	notice, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxNoticeBytes))
	if err != nil {
		http.Error(w, "invalid completion body", http.StatusBadRequest)
		return
	}
	attempt, err := s.Store.RewardedAdAttempt(ctx, r.PathValue("attempt"))
	if errors.Is(err, mongo.ErrNoDocuments) || persistence.Code(err) == p.CodeInvalidArgument {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "completion verification unavailable", http.StatusServiceUnavailable)
		return
	}
	at := time.Now()
	proof, err := s.AdVerifier.Verify(ctx, attempt.Expected(), auth, notice, at)
	if errors.Is(err, rewardedad.ErrProof) {
		http.Error(w, "invalid completion proof", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "completion verification unavailable", http.StatusServiceUnavailable)
		return
	}
	result, err := s.Store.RecordRewardedAd(ctx, attempt.ID, proof, at)
	switch persistence.Code(err) {
	case 0:
		respond(w, map[string]uint32{"stage": result.Stage, "result_code": result.ResultCode})
	case p.CodeRewardedAdExpired, p.CodeOperationConflict:
		http.Error(w, "completion conflicts with original attempt", http.StatusConflict)
	default:
		http.Error(w, "completion verification pending", http.StatusServiceUnavailable)
	}
}
