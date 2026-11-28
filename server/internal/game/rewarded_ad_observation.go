// 本文件将已提交的广告到账结果应用到原附着，保留运行中的战斗与托管状态。
package game

import (
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// applyRewardedAdGrant 检查原资产快照后，只更新托管秒数和角色修订。
func (w *worker) applyRewardedAdGrant(r *room, store *persistence.Store, attemptID string, next domain.Role, receipt persistence.Operation) {
	grant := receipt.RewardedAd
	if grant == nil || grant.GrantedAt.IsZero() || receipt.Code != 0 || receipt.RoleID != r.Role.ID ||
		grant.Attempt != attemptID || grant.Account != r.Auth.AccountID || grant.Origin != r.Role.Origin ||
		next.ID != r.Role.ID || next.AccountID != r.Auth.AccountID || next.Origin != r.Role.Origin ||
		next.Owner != store.Instance || next.RunServerID != store.Cfg.RunID || next.RunEpoch != store.Epoch ||
		next.SessionEpoch != r.Auth.SessionEpoch || next.Revision != receipt.Revision ||
		next.Revision <= r.Role.Revision || next.Revision-r.Role.Revision != 1 ||
		next.EconomicSeq != r.Role.EconomicSeq || next.SourceSeq != r.Role.SourceSeq || next.Wallet != r.Role.Wallet ||
		grant.Seconds == 0 || grant.Before != r.Role.HostingSeconds || grant.Balance != next.HostingSeconds ||
		grant.Balance-grant.Before != int64(grant.Seconds) {
		return
	}
	r.Role.HostingSeconds, r.Role.Revision = next.HostingSeconds, next.Revision
	w.assets(r)
}
