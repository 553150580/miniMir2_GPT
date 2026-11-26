// 本文件把已提交的支付资产叠加到正确的角色附着，沿用现有钱包与权益通知。
package game

import (
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// applyPaymentGrant 只接受紧接本角色当前快照的财务写入，避免挤掉未应用的其它永久结果。
func (w *worker) applyPaymentGrant(r *room, store *persistence.Store, orderID string, next domain.Role, receipt persistence.Operation, now time.Time) {
	grant := receipt.PaymentGrant
	if grant == nil || grant.GrantedAt.IsZero() || receipt.Code != 0 || receipt.RoleID != r.Role.ID ||
		grant.OrderID != orderID || grant.AccountID != r.Auth.AccountID || grant.OriginServerID != r.Role.Origin ||
		next.ID != r.Role.ID || next.AccountID != r.Auth.AccountID || next.Origin != r.Role.Origin ||
		next.Owner != store.Instance || next.RunServerID != store.Cfg.RunID || next.RunEpoch != store.Epoch ||
		next.SessionEpoch != r.Auth.SessionEpoch || next.Revision != receipt.Revision ||
		next.Revision <= r.Role.Revision || next.Revision-r.Role.Revision != 1 ||
		next.EconomicSeq != r.Role.EconomicSeq || next.SourceSeq != r.Role.SourceSeq ||
		grant.WalletBefore != r.Role.Wallet || grant.WalletAfter != next.Wallet {
		return
	}
	card := grant.Card
	if card != nil && card.Kind != devconfig.RoleCardKindPrivilege && card.Kind != devconfig.RoleCardKindHosting {
		return
	}
	previousPrivilege := r.Role.PrivilegeUntil
	r.Role.Wallet.Yuanbao = next.Wallet.Yuanbao
	if card != nil {
		at := r.observeRoleCardChange(card.Kind, card.PreviousSince, card.PreviousUntil, grant.GrantedAt, now)
		switch card.Kind {
		case devconfig.RoleCardKindPrivilege:
			r.Role.PrivilegeSince, r.Role.PrivilegeUntil = card.ValidSince, card.ValidUntil
		case devconfig.RoleCardKindHosting:
			r.Role.HostingCardSince, r.Role.HostingCardUntil = card.ValidSince, card.ValidUntil
		}
		w.acceptRolePrivilege(r, at, previousPrivilege)
	}
	r.Role.Revision = next.Revision
	w.assets(r)
}
