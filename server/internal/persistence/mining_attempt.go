// 本文件使用角色来源水位、永久回执和运行服围栏提交每次挖矿判定。
package persistence

import (
	"context"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const miningAttemptKind = "mining_attempt" // 矿石、锄头与托管扣时在同一来源账本提交。

// MiningAttemptRequest 是游戏进程为原矿层和工具生成的判定事实。
type MiningAttemptRequest struct {
	EventID          string    // 当前判定的服务端随机身份，用于重试与稳定抽样。
	SourceSeq        int64     // 角色预期的下一来源序号。
	ExpectedRevision int64     // 判定发起时已确认的角色版本。
	RuleVersion      uint64    // 矿层和工具配置的原规则版本。
	ConfigHash       string    // 矿层、掉矿及工具使用的原配置摘要。
	MapID            uint64    // 实际挖矿发生的矿图编号。
	ToolID           string    // 从当前背包独立选择的锄头实例。
	ToolVersion      int64     // 选择该锄头时的实例修订。
	StartedAt        time.Time // 本轮开始等待判定的服务端时刻。
	At               time.Time // 本轮执行判定的服务端时刻和矿石绑定时点。
	HostingSession   string    // 离线托管会话，在线时为空。
	HostSeconds      int64     // 只有离线执行时扣除一个完整配置间隔。
}

// MiningAttemptReceipt 固定该次判定的最终状态。Ore 为空表示判定完成但没有出矿。
type MiningAttemptReceipt struct {
	EventID              string         `bson:"event_id"`                  // 原判定身份。
	MapID                uint64         `bson:"map_id"`                    // 原矿图编号。
	RuleVersion          uint64         `bson:"rule_version"`              // 判定引用的原规则版本。
	ConfigHash           string         `bson:"config_hash"`               // 判定引用的原配置摘要。
	Layer                uint32         `bson:"layer"`                     // 原矿层，决定矿种分布。
	StartedAt            time.Time      `bson:"started_at"`                // 本轮真正开始等待的时刻。
	At                   time.Time      `bson:"event_at"`                  // 执行判定及检查出矿绑定的时刻。
	ToolID               string         `bson:"tool_id"`                   // 原锄头实例编号。
	ToolItemID           uint32         `bson:"tool_item_id"`              // 对应的锄头定义编号。
	ToolVersionBefore    int64          `bson:"tool_version_before"`       // 消耗前的锄头实例版本。
	ToolVersionAfter     int64          `bson:"tool_version_after"`        // 消耗后的版本，耗尽时保留在原回执里。
	DurabilityBeforeU    int64          `bson:"durability_before_u"`       // 消耗前的存储单位耐久。
	DurabilityAfterU     int64          `bson:"durability_after_u"`        // 消耗后的存储单位耐久。
	ToolExhausted        bool           `bson:"tool_exhausted"`            // 本轮是否耗尽并移除该锄头。
	Ore                  *domain.Item   `bson:"ore,omitempty"`             // 本轮实际产出的原矿，含回收时用于审计的原件。
	AutoRecycle          *domain.Wallet `bson:"auto_recycle,omitempty"`    // 已回收的原矿实际到账金额，空值表示没有回收。
	HostingSession       string         `bson:"hosting_session,omitempty"` // 离线托管原会话。
	HostSeconds          int64          `bson:"host_seconds"`              // 本轮实际扣除的托管秒数。
	RemainingHostSeconds int64          `bson:"remaining_host_seconds"`    // 首次提交后剩余的托管秒数。
}

// ResolveMiningAttempt 在同一来源事务扣工具、扣离线时间，并发放原矿或结算自动回收。
func (s *Store) ResolveMiningAttempt(ctx context.Context, auth Auth, q MiningAttemptRequest) (domain.Role, Operation, error) {
	q.At, q.StartedAt = bossTime(q.At), bossTime(q.StartedAt)
	if !lowerHex(q.EventID, 16) || !lowerHex(q.ToolID, 16) || !lowerHex(q.ConfigHash, 32) || q.ToolVersion <= 0 ||
		q.MapID == 0 || q.RuleVersion == 0 || q.ExpectedRevision <= 0 || q.SourceSeq <= 0 ||
		q.At.IsZero() || q.StartedAt.IsZero() || q.At.After(bossTime(time.Now())) || q.HostSeconds < 0 ||
		(q.HostingSession == "") != (q.HostSeconds == 0) || q.HostingSession != "" && !lowerHex(q.HostingSession, 16) {
		return domain.Role{}, Operation{}, Fail(p.CodeInvalidArgument, "挖矿判定的来源或时刻无效")
	}
	opID := "source_mining:" + auth.RoleID + ":" + q.EventID
	digest, err := bossDigest(q)
	if err != nil {
		return domain.Role{}, Operation{}, err
	}
	var out domain.Role   // 本次已提交或重放时所观察的角色聚合。
	var receipt Operation // 仅在原资产与审计同事务成功后返回。
	err = s.guardedTx(ctx, auth, func(tc context.Context) error {
		fence, err := s.fenceWrites(tc, auth)
		if err != nil {
			return err
		}
		role, err := s.Role(tc, auth.RoleID)
		if err != nil {
			return err
		}
		if err := s.checkRole(role, auth); err != nil {
			return err
		}
		if q.SourceSeq <= role.SourceSeq {
			// 原提交可能已完成但回包丢失。先核对原回执，再看当期配置与工具。
			var original Operation
			if err := s.DB.Collection("operations").FindOne(tc, bson.M{"_id": opID}).Decode(&original); err != nil {
				return authorityReadError(err, p.CodeOperationConflict, "挖矿原来源没有永久回执")
			}
			if original.Kind != miningAttemptKind || original.RoleID != role.ID || original.SourceSeq != q.SourceSeq ||
				original.Digest != digest || original.Mining == nil || original.Mining.EventID != q.EventID {
				return Fail(p.CodeOperationConflict, "同一挖矿来源的事实已变")
			}
			if err := s.writeFence(tc, fence); err != nil {
				return err
			}
			out, receipt = role, original
			return nil
		}
		fixture := &s.Cfg.Fixture
		if role.Revision == math.MaxInt64 || role.SourceSeq == math.MaxInt64 || role.Revision != q.ExpectedRevision ||
			q.SourceSeq != role.SourceSeq+1 || q.RuleVersion != fixture.RuleVersion || q.ConfigHash != fixture.Hash {
			return Fail(p.CodeOperationConflict, "挖矿规则、角色修订或来源水位已变化")
		}
		layer, interval, err := validateMiningAttempt(role, fixture, q)
		if err != nil {
			return err
		}
		tool, ok := MiningToolForAttempt(role, fixture, q.MapID, q.ToolID)
		if !ok || tool.Version != q.ToolVersion {
			return Fail(p.CodeVendorItemBlocked, "原锄头已移动、耗尽或修订变化")
		}
		ore, err := sampleMiningOre(role.ID, q.EventID, layer.Layer, fixture)
		if err != nil {
			return err
		}
		if ore != nil {
			until, err := s.RolePrivilegeUntilAt(tc, role, q.At)
			if err != nil {
				return err
			}
			ore.Bound = !until.After(q.At)
		}
		next, details, err := applyMiningAttempt(role, q, layer.Layer, interval, tool, fixture, ore)
		if err != nil {
			return err
		}
		proposed := Operation{ID: opID, Digest: digest, RoleID: role.ID, CommandSeq: role.EconomicSeq, SourceSeq: q.SourceSeq,
			Revision: next.Revision, Kind: miningAttemptKind, Created: bossTime(time.Now()), Mining: &details}
		audit := bson.M{"_id": opID, "kind": miningAttemptKind, "account_id": role.AccountID, "role_id": role.ID,
			"source_seq": q.SourceSeq, "event_at": q.At, "rule_version": q.RuleVersion, "config_hash": q.ConfigHash,
			"host_seconds": q.HostSeconds, "hosting_session": q.HostingSession, "mining_attempt": details, "at": proposed.Created}
		if err := s.writeSettlement(tc, fence, next, role.Revision, proposed, audit); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeOperationConflict, "挖矿原判定已占用其他来源水位")
			}
			return err
		}
		out, receipt = next, proposed
		return nil
	})
	if err != nil {
		return domain.Role{}, Operation{}, err
	}
	return out, receipt, nil
}
