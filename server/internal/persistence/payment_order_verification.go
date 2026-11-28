// 本文件绑定渠道原单、保存主动查单证明并记录对账分歧。实际钱包与卡期由独立履约事务处理。
package persistence

import (
	"context"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/payment"
	p "minimir/internal/protocol"
)

// PaymentOrderVerification 保存第一次主动核验的实付证明和冻结条款摘要。
type PaymentOrderVerification struct {
	TransactionID string    `bson:"transaction_id"` // 渠道与商户作用域内唯一的实付交易号。
	OrderDigest   string    `bson:"order_digest"`   // 已核验原单及其不可变履约量的摘要。
	PaidAt        time.Time `bson:"paid_at"`        // 渠道确认的支付时刻。
	VerifiedAt    time.Time `bson:"verified_at"`    // 本地首次提交核验的时刻。
}

// PaymentReconciliationStatus 表示一次经认证的原单查询产生的确定观察。
type PaymentReconciliationStatus string

const (
	PaymentReconciliationPaid     PaymentReconciliationStatus = "paid"     // 渠道确认原金额、商品及受益人均匹配。
	PaymentReconciliationPending  PaymentReconciliationStatus = "pending"  // 渠道当前未确认成功支付。
	PaymentReconciliationClosed   PaymentReconciliationStatus = "closed"   // 渠道已关闭原单。
	PaymentReconciliationRefunded PaymentReconciliationStatus = "refunded" // 渠道确认撤销或退款。
	PaymentReconciliationMismatch PaymentReconciliationStatus = "mismatch" // 经认证的查单结果与绑定或原证明冲突。
)

// PaymentOrderReconciliation 保存最后一次实际改变对账观察的时刻和分类，不保存渠道响应体或凭据。
type PaymentOrderReconciliation struct {
	Status    PaymentReconciliationStatus `bson:"status"`     // 已知的渠道状态或差异类型。
	CheckedAt time.Time                   `bson:"checked_at"` // 持久记录该观察时的服务端时刻。
}

// BindPaymentOrder 接受可信渠道受理后获得的原单号，商户作用域和已绑定值均不可被改派。
// 此内部入口只绑定订单身份，后续必须经过主动查单才能确认付款。
func (s *Store) BindPaymentOrder(ctx context.Context, orderID, providerID, merchantID, externalOrderID string) (PaymentOrder, error) {
	if !lowerHex(orderID, 16) || !validPaymentChannelID(providerID, 32) || !validPaymentChannelID(merchantID, 128) ||
		!validPaymentChannelID(externalOrderID, 128) {
		return PaymentOrder{}, Fail(p.CodeInvalidArgument, "渠道订单绑定参数无效")
	}
	var result PaymentOrder // 本次提交完成的绑定或其原幂等结果。
	err := s.tx(ctx, func(tc context.Context) error {
		order, err := s.readPaymentOrder(tc, orderID)
		if err != nil {
			return err
		}
		if order.Quote.ProviderID != providerID || order.Quote.MerchantID != merchantID {
			return Fail(p.CodeOperationConflict, "渠道或商户与原订单不符")
		}
		if order.ExternalOrderID == externalOrderID && order.State != PaymentOrderCreated {
			result = order
			return nil
		}
		if order.State != PaymentOrderCreated || order.ExternalOrderID != "" {
			return Fail(p.CodeOperationConflict, "支付原单已绑定其他渠道订单")
		}
		at := bossTime(time.Now())
		change := bson.M{"state": PaymentOrderBound, "external_order_id": externalOrderID}
		if err = s.updatePaymentOrder(tc, order, change, at); err != nil {
			return err
		}
		order.State, order.ExternalOrderID = PaymentOrderBound, externalOrderID
		order.Revision, order.UpdatedAt = order.Revision+1, at
		result = order
		return nil
	})
	if mongo.IsDuplicateKeyError(err) {
		err = Fail(p.CodeOperationConflict, "该渠道订单已绑定其他本地原单")
	}
	if err != nil {
		return PaymentOrder{}, err
	}
	return result, nil
}

// RecordVerifiedPayment 持久化可信查单器产出的支付证明。原渠道交易的重试返回同一状态。
// 仅按订单写入第一份支付证明。存在分歧时，使用 ReconcilePaymentOrder 重新验证并记录结果。
func (s *Store) RecordVerifiedPayment(ctx context.Context, proof payment.VerifiedPayment) (PaymentOrder, error) {
	return s.recordVerifiedPayment(ctx, proof, 0)
}

// recordVerifiedPayment 让主动对账的查询结果与开始时的原单修订保持一致。
func (s *Store) recordVerifiedPayment(ctx context.Context, proof payment.VerifiedPayment, observedRevision int64) (PaymentOrder, error) {
	if !proof.Valid() {
		return PaymentOrder{}, Fail(p.CodeInvalidArgument, "缺少有效渠道支付证明")
	}
	expected := proof.Order()
	var result PaymentOrder // 本次核验成功或已存在的原订单状态。
	var disagreement bool   // 是否已记录渠道返回了不同的原交易。
	err := s.tx(ctx, func(tc context.Context) error {
		disagreement = false
		order, err := s.readPaymentOrder(tc, expected.ID)
		if err != nil {
			return err
		}
		if order.Expected() != expected || !order.Quote.valid() ||
			(order.State != PaymentOrderBound && order.State != PaymentOrderVerified && order.State != PaymentOrderReview && order.State != PaymentOrderFulfilled) {
			return Fail(p.CodeOperationConflict, "支付证明与持久原订单不符")
		}
		if observedRevision != 0 && order.Revision != observedRevision {
			return Fail(p.CodeOperationConflict, "原订单在主动查询期间已变化")
		}
		if order.Quote.Kind == payment.KindBattlePass && order.State != PaymentOrderFulfilled &&
			(!order.BattlePassReserved || !order.acceptsBattlePassPaidAt(proof.PaidAt())) {
			result, err = s.writePaymentObservation(tc, order, PaymentReconciliationMismatch)
			disagreement = err == nil
			return err
		}
		digest, err := order.verificationDigest()
		if err != nil {
			return err
		}
		if order.Verification != nil {
			original := order.Verification
			if original.OrderDigest != digest || original.TransactionID != proof.TransactionID() || !original.PaidAt.Equal(proof.PaidAt()) {
				if observedRevision == 0 {
					return Fail(p.CodeOperationConflict, "渠道实付证明与已确认的原交易不一致")
				}
				result, err = s.writePaymentObservation(tc, order, PaymentReconciliationMismatch)
				disagreement = err == nil
				return err
			}
			if observedRevision == 0 || order.Reconciliation != nil && order.Reconciliation.Status == PaymentReconciliationPaid {
				result = order
				return nil
			}
		} else if order.State != PaymentOrderBound || observedRevision == 0 && requiresPaymentRecheck(order.Reconciliation) {
			return Fail(p.CodeOperationConflict, "原订单需要按当前状态重新主动对账")
		}
		at := bossTime(time.Now())
		change := bson.M{}
		if order.Verification == nil {
			order.Verification = &PaymentOrderVerification{TransactionID: proof.TransactionID(), OrderDigest: digest, PaidAt: proof.PaidAt(), VerifiedAt: at}
			order.VerifiedAt = at
			change["verification"], change["verified_at"] = order.Verification, at
		}
		order.Reconciliation = &PaymentOrderReconciliation{Status: PaymentReconciliationPaid, CheckedAt: at}
		change["reconciliation"] = order.Reconciliation
		if order.State != PaymentOrderFulfilled {
			change["state"] = PaymentOrderVerified
		}
		if err = s.updatePaymentOrder(tc, order, change, at); err != nil {
			return err
		}
		if order.State != PaymentOrderFulfilled {
			order.State = PaymentOrderVerified
		}
		order.Revision, order.UpdatedAt = order.Revision+1, at
		result = order
		return nil
	})
	if mongo.IsDuplicateKeyError(err) {
		err = Fail(p.CodeOperationConflict, "该渠道实付交易已核验至其他原单")
	}
	if err != nil {
		return PaymentOrder{}, err
	}
	if disagreement {
		return result, payment.ErrOrderMismatch
	}
	return result, nil
}

// ReconcilePaymentOrder 以持久原单主动查证渠道事实，受理结果、实付及履约各自保留。
// 已确定但未支付的结果按原修订记账，并把原对账原因返回给内部调用方。
func (s *Store) ReconcilePaymentOrder(ctx context.Context, verifier *payment.Verifier, orderID string) (PaymentOrder, error) {
	order, err := s.GetPaymentOrder(ctx, orderID)
	if err != nil {
		return PaymentOrder{}, err
	}
	proof, queryErr := verifier.Verify(ctx, order.Expected())
	if queryErr == nil {
		return s.recordVerifiedPayment(ctx, proof, order.Revision)
	}
	status := reconciliationStatus(queryErr)
	if status == "" {
		return order, queryErr
	}
	updated, err := s.recordPaymentDisagreement(ctx, order, status)
	if err != nil {
		return PaymentOrder{}, err
	}
	return updated, queryErr
}

// reconciliationStatus 只保存来源已认证、状态明确的查询分歧。超时及查询失败沿原单重试。
func reconciliationStatus(err error) PaymentReconciliationStatus {
	switch {
	case errors.Is(err, payment.ErrNotPaid):
		return PaymentReconciliationPending
	case errors.Is(err, payment.ErrClosed):
		return PaymentReconciliationClosed
	case errors.Is(err, payment.ErrRefunded):
		return PaymentReconciliationRefunded
	case errors.Is(err, payment.ErrOrderMismatch):
		return PaymentReconciliationMismatch
	default:
		return ""
	}
}

// requiresPaymentRecheck 指示原单曾产生需主动刷新才能采信的渠道分歧。
func requiresPaymentRecheck(observation *PaymentOrderReconciliation) bool {
	return observation != nil && observation.Status != PaymentReconciliationPaid && observation.Status != PaymentReconciliationPending
}

// recordPaymentDisagreement 按查单前原修订记录分歧，已履约的资产回执继续保留。
func (s *Store) recordPaymentDisagreement(ctx context.Context, order PaymentOrder, status PaymentReconciliationStatus) (PaymentOrder, error) {
	var result PaymentOrder // 对账时原单修订对应的最新确定观察。
	err := s.tx(ctx, func(tc context.Context) error {
		updated, err := s.writePaymentObservation(tc, order, status)
		result = updated
		return err
	})
	if err != nil {
		return PaymentOrder{}, err
	}
	return result, nil
}

// writePaymentObservation 与调用者的核验事务共享快照，保留所有已提交的原支付与资产证据。
func (s *Store) writePaymentObservation(ctx context.Context, order PaymentOrder, status PaymentReconciliationStatus) (PaymentOrder, error) {
	if order.Reconciliation != nil && order.Reconciliation.Status == status {
		return order, nil
	}
	at := bossTime(time.Now())
	observation := &PaymentOrderReconciliation{Status: status, CheckedAt: at}
	change := bson.M{"reconciliation": observation}
	if order.State == PaymentOrderVerified {
		change["state"] = PaymentOrderReview
	}
	// 仅渠道确认关闭且不存在实付证明时释放原角色的升档步骤。
	release := order.Quote.Kind == payment.KindBattlePass && order.BattlePassReserved && order.Verification == nil && status == PaymentReconciliationClosed
	if release {
		change["battle_pass_reserved"] = false
	}
	if err := s.updatePaymentOrder(ctx, order, change, at); err != nil {
		return PaymentOrder{}, err
	}
	if order.State == PaymentOrderVerified {
		order.State = PaymentOrderReview
	}
	if release {
		order.BattlePassReserved = false
	}
	order.Reconciliation = observation
	order.Revision, order.UpdatedAt = order.Revision+1, at
	return order, nil
}

// verificationDigest 固定核验时的受益人、渠道绑定和报价授予量。新账单格式使用独立版本。
func (order PaymentOrder) verificationDigest() (string, error) {
	return bossDigest(struct {
		Version  uint32                // 核验摘要结构版本。
		Expected payment.ExpectedOrder // 完整已绑定原单的核验身份。
		Quote    PaymentOrderQuote     // 同次核验使用的原服务端报价。
	}{1, order.Expected(), order.Quote})
}

// hasVerifiedPayment 检查订单可提交履约及其持久核验与完整原条款相符。
func (order PaymentOrder) hasVerifiedPayment() bool {
	proof := order.Verification
	if (order.State != PaymentOrderVerified && order.State != PaymentOrderFulfilled) || proof == nil ||
		!validPaymentChannelID(order.ExternalOrderID, 128) || !validPaymentChannelID(proof.TransactionID, 128) ||
		proof.PaidAt.IsZero() || proof.VerifiedAt.IsZero() || proof.PaidAt.After(proof.VerifiedAt) ||
		!order.VerifiedAt.Equal(proof.VerifiedAt) || order.Expected().BeneficiaryReference() == "" || !order.Quote.valid() {
		return false
	}
	if order.State == PaymentOrderVerified && (order.Reconciliation == nil || order.Reconciliation.Status != PaymentReconciliationPaid) {
		return false
	}
	digest, err := order.verificationDigest()
	return err == nil && digest == proof.OrderDigest
}

// markPaymentOrderFulfilled 只更新原单状态与资产回执指针，须在创建该回执的资产事务中调用。
func (s *Store) markPaymentOrderFulfilled(ctx context.Context, order PaymentOrder, assetOperationRef string, at time.Time) error {
	at = bossTime(at)
	if mongo.SessionFromContext(ctx) == nil || order.State != PaymentOrderVerified || !order.hasVerifiedPayment() ||
		assetOperationRef == "" || len(assetOperationRef) > 256 || at.IsZero() || at.Before(order.VerifiedAt) {
		return Fail(p.CodeOperationConflict, "原支付订单或资产回执证据无效")
	}
	fields := bson.M{"state": PaymentOrderFulfilled, "asset_operation_ref": assetOperationRef, "fulfilled_at": at}
	if order.Quote.Kind == payment.KindBattlePass {
		fields["battle_pass_reserved"] = false
	}
	return s.updatePaymentOrder(ctx, order, fields, at)
}

// updatePaymentOrder 在原阶段和修订仍匹配时写入新状态，避免覆盖并发的查单或履约。
func (s *Store) updatePaymentOrder(ctx context.Context, order PaymentOrder, fields bson.M, at time.Time) error {
	if order.Revision <= 0 || order.Revision == math.MaxInt64 {
		return Fail(p.CodeOperationConflict, "支付原单修订无效")
	}
	fields["updated_at"] = at
	filter := bson.M{"_id": order.ID, "revision": order.Revision, "state": order.State}
	result, err := s.DB.Collection(paymentOrderCollection).UpdateOne(ctx, filter, bson.M{"$set": fields, "$inc": bson.M{"revision": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "支付原单已由并发请求更新")
	}
	return nil
}
