// 本文件回归自动用蓝药的机会判定，以及提交结果未知时沿原操作恢复且只生效一次。
package game

import (
	"strconv"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestAutomaticManaPotionRequiresCastOpportunity 核对「缺少法术机会或药品资格时发起了消费」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticManaPotionRequiresCastOpportunity(t *testing.T) {
	// checks 建立带初始值的临时结构列表，供本段继续处理。
	checks := []struct {
		// name 记录名称，使用文本表示。
		name string
		// set 记录设置，使用供当前流程调用的函数表示。
		set func(*worker, *room, time.Time)
	}{
		// r 指向当前执行器拥有并更新的目标房间。
		{"关闭自动施法", func(_ *worker, r *room, _ time.Time) { r.AutoSkills = &autoSkillSettings{} }},
		// r 指向当前执行器拥有并更新的目标房间。
		{"禁用单个技能", func(_ *worker, r *room, _ time.Time) {
			r.AutoSkills = &autoSkillSettings{enabled: true, disabled: []uint32{1}}
		}},
		// r 指向当前执行器拥有并更新的目标房间。
		{"技能未学", func(_ *worker, r *room, _ time.Time) { r.Role.Skills = nil }},
		// r 指向当前执行器拥有并更新的目标房间。
		{"目标超出射程", func(_ *worker, r *room, _ time.Time) {
			r.Player.Pos, r.Monsters[0].Pos = devconfig.Point{X: 2, Y: 8}, devconfig.Point{X: 16, Y: 8}
		}},
		// w 记录房间执行器。
		{"目标被阻挡", func(w *worker, _ *room, _ time.Time) {
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 8, Y: 7}}
		}},
		// r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
		{"技能冷却", func(_ *worker, r *room, now time.Time) { r.NextSpell = now.Add(time.Second) }},
		// r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
		{"共享用药冷却", func(_ *worker, r *room, now time.Time) { r.Potions.NextUse = now.Add(time.Second) }},
		// r 指向当前执行器拥有并更新的目标房间。
		{"前瓶仍在恢复", func(_ *worker, r *room, _ time.Time) { r.Potions.MP = 1 }},
		// r 指向当前执行器拥有并更新的目标房间。
		{"最大魔法不足", func(_ *worker, r *room, _ time.Time) { r.Player.MaxMP = 1 }},
		// r 指向当前执行器拥有并更新的目标房间。
		{"只有红药与锁定蓝药", func(_ *worker, r *room, _ time.Time) {
			r.Role.Items[0].Locked = true
			r.Role.Items = append(r.Role.Items, domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 1, Version: 1})
		}},
	}
	// check 接收遍历“checks”时当前的元素。
	for _, check := range checks {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(check.name, func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
			w, r, _ := stageBWorker(t, 1, 1)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			prepareAutoTargetTest(w, r, now, devconfig.Point{X: 8, Y: 6})
			r.Player.MP = 0
			r.Role.Items = []domain.Item{{ID: domain.ID(), ItemID: 36, Quantity: 1, Version: 1}}
			check.set(w, r, now)
			if w.autoSkill(r, now) || r.Pending || r.Player.CastSequence != 0 {
				t.Fatal("缺少法术机会或药品资格时发起了消费")
			}
		})
	}
}

// TestManaPotionUnknownKeepsOperationAndAppliesOnlyMPOnce 核对「手动缺蓝未启动原流程、假发药效或提前扣库存」与「自动用药与手动未共享冷却，或重复回执重复入队」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestManaPotionUnknownKeepsOperationAndAppliesOnlyMPOnce(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 1, 11)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	prepareAutoTargetTest(w, r, now, devconfig.Point{X: 8, Y: 6})
	r.Auth.RoleID, r.Role.ID = "role", "role"
	r.Player.Life, r.Role.Origin = 4, 1
	r.Role.SourceSeq, r.Role.Revision, r.Role.EconomicSeq = 17, 22, 9
	r.Potions.LastSourceSeq, r.Potions.LastAppliedSeq = 17, 9
	r.Potions.HP = 7
	// blue 建立带初始值的道具，供本段继续处理。
	blue := domain.Item{ID: domain.ID(), ItemID: 36, Quantity: 2, Version: 5}
	r.Role.Items = []domain.Item{blue}
	// spell 保存由技能取得的法术。
	spell, _ := w.e.Store.Cfg.Fixture.Skill(1)
	r.Player.MP = spell.MPBaseCost
	if !w.autoSkill(r, now) || r.Pending || len(r.Casts) != 1 || r.Casts[0].Skill.ID != spell.ID {
		t.Fatal("有蓝量足够的低阶技能时本可施法却先喝药")
	}
	clearAction(r)
	r.Player.MP = 0
	r.Auto = false
	// manual 保存当前处理所用的手动。
	manual := &pb.CastIntent{SceneEpoch: r.Scene, InputSequence: 8, SkillId: spell.ID, TargetHandle: r.Monsters[0].Handle}
	if w.castWithManaRecovery(r, manual, now) != p.CodeSkillResource || !r.Pending || r.Potions.MP != 0 || r.Role.Items[0] != blue {
		t.Fatal("手动缺蓝未启动原流程、假发药效或提前扣库存")
	}
	// binding 初始化用作会话绑定的资产会话绑定。
	binding := assetBinding{room: r, auth: r.Auth, generation: r.AssetGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	// q 组装资产请求所需的数据。
	q := persistence.AssetRequest{Kind: persistence.AssetAutoUsePotion, ItemID: blue.ID, ItemDefinitionID: blue.ItemID, SceneEpoch: r.Scene, CasterLife: r.Player.Life, SourceSeq: 18}
	// retry 保存待按原身份重试的操作，用于当前步骤中的重试，frames 记录协议帧集合中的条目数。
	retry, frames := r.PendingRetry, len(c.frames)
	w.finishAsset(binding, q, wire.Frame{}, "", domain.Role{}, &pb.OperationResult{}, persistence.Operation{}, persistence.Fail(p.CodeOperationPending, "未知提交"), retry)
	if !r.Pending || r.PendingRetry != retry || r.PendingOperationID != binding.id || len(c.frames) != frames || r.Potions.MP != 0 {
		t.Fatal("未知提交期间更换了原操作或发出假药效")
	}
	// role 保存当前处理所用的角色。
	role := r.Role
	role.Revision, role.SourceSeq = 23, 18
	role.Items = []domain.Item{{ID: blue.ID, ItemID: blue.ItemID, Quantity: 1, Version: 6}}
	// receipt 初始化用作回执的操作。
	receipt := persistence.Operation{Kind: q.Kind, RoleID: role.ID, Digest: binding.fingerprint, SourceSeq: q.SourceSeq, CommandSeq: role.EconomicSeq, Potion: &persistence.PotionReceipt{Scene: strconv.FormatUint(r.Scene, 10), ItemID: blue.ItemID, MP: 30}}
	w.finishAsset(binding, q, wire.Frame{}, "", role, &pb.OperationResult{}, receipt, nil, retry)
	if r.Pending || r.Role.Items[0].Quantity != 1 || r.Potions.MP != 30 || r.Potions.HP != 7 || r.Potions.LastAppliedSeq != 9 || r.Potions.LastSourceSeq != 18 || r.Player.CastSequence != 1 {
		t.Fatal("确认后未按独立来源序号入队，影响了红药、手动经济水位或重放了手动施法")
	}
	if potionGate(r, r.Scene, r.Potions.NextUse.Add(-time.Nanosecond)) != p.CodeItemUseCooldown || w.applyPotionReceipt(r, receipt, now) || r.Potions.MP != 30 {
		t.Fatal("自动用药与手动未共享冷却，或重复回执重复入队")
	}
}
