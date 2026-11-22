// 本文件回归自动战斗的技能优先级、目标合法性、治疗对象和自动偏好协议。
package game

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestAutomaticCastSelectsLegalTargetAndHonorsSeparatePreferences 核验自动施法应跳过禁用雷电并选择可飞行的活怪，用诊断「自动施法偏好阻止手动合法施法」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticCastSelectsLegalTargetAndHonorsSeparatePreferences(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 1, 11)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 8, Y: 7}}
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 6}
	// visible 保存当前处理所用的可见。
	visible := *r.Monsters[0]
	visible.Handle, visible.Pos = 11, devconfig.Point{X: 12, Y: 8}
	r.Monsters, r.MonsterCount = append(r.Monsters, &visible), 2
	r.AutoSkills = &autoSkillSettings{enabled: true, disabled: []uint32{11}}
	if !w.autoSkill(r, now) || len(r.Casts) != 1 || r.Casts[0].Skill.ID != 1 || r.Casts[0].Target != visible.Handle {
		t.Fatal("自动施法没有跳过禁用雷电并选择可飞行的活怪")
	}
	clearAction(r)
	r.AutoSkills = &autoSkillSettings{}
	// mana 保存魔法值，用于当前步骤中的魔法值。
	mana := r.Player.MP
	if w.autoSkill(r, now) || r.Player.MP != mana {
		t.Fatal("关闭自动施法仍耗蓝")
	}
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 1, visible.Handle, 7, now); code != 0 || len(r.Casts) != 1 {
		t.Fatal("自动施法偏好阻止手动合法施法", code)
	}
}

// TestAutomaticHealingSkipsPendingPlayerHealingAndSelectsInjuredPet 核验玩家已有恢复队列时，伤宠应成为下一合法治疗目标。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticHealingSkipsPendingPlayerHealingAndSelectsInjuredPet(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 2, 2)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Player.HP, r.Player.HealAmount = r.Player.MaxHP-1, 20
	r.Pet.HP = r.Pet.MaxHP - 1
	if !w.autoSkill(r, now) || len(r.Casts) != 1 || r.Casts[0].Skill.ID != 2 || r.Casts[0].Target != r.Pet.Handle {
		t.Fatal("玩家已有恢复队列时，伤宠没有成为下一合法治疗目标")
	}
}

// TestBlockedPersonalGroundNavigationReleasesAutomaticCasting 核对「占格点地未以原导航身份结束，或擅自同tick忽略手动动作」与「点地失败后自动施法仍被旧目标阻塞」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestBlockedPersonalGroundNavigationReleasesAutomaticCasting(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// monster 保存从怪物集合取出的怪物。
	monster := r.Monsters[0]
	monster.NextMove, monster.NextAttack = now.Add(time.Minute), now.Add(time.Minute)
	// goal 保存位置，用于当前步骤中的目标格。
	goal := monster.Pos
	r.Auto, r.Destination = true, &goal
	r.Navigation = navigationState{InputSequence: 4, Revision: 3, Destination: &goal, Status: navigationActive}
	w.step(r, now)
	if r.Destination != nil || r.Navigation.Status != navigationBlocked || r.Navigation.Destination == nil ||
		*r.Navigation.Destination != goal || r.Navigation.InputSequence != 4 || len(r.Casts) != 0 {
		t.Fatal("占格点地未以原导航身份结束，或擅自同tick忽略手动动作")
	}
	w.step(r, now.Add(100*time.Millisecond))
	if len(r.Casts) != 1 || r.Casts[0].Skill.ID != 1 || r.Casts[0].Target != monster.Handle {
		t.Fatal("点地失败后自动施法仍被旧目标阻塞")
	}
}

// TestAutomaticThrustingAttacksSingleMonsterFromSecondCell 核对「开启刺杀后，二格单怪未直接受到合法刺杀」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticThrustingAttacksSingleMonsterFromSecondCell(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 0, 12)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// monster 保存从怪物集合取出的怪物，origin 保存位置，用于当前步骤中的大区或起点。
	monster, origin := r.Monsters[0], r.Player.Pos
	monster.Pos, monster.NextMove, monster.NextAttack = stepPoint(origin, 4, 2), now.Add(time.Minute), now.Add(time.Minute)
	r.Auto, r.Melee.Thrusting = true, true
	w.step(r, now)
	if r.Player.Pos != origin || r.Player.AttackSkillID != 12 || r.Player.AttackSequence != 1 || monster.HP >= monster.MaxHP {
		t.Fatal("开启刺杀后，二格单怪未直接受到合法刺杀")
	}
}

// TestAutoSkillSettingsProtocolPreservesGlobalAutoAndValidatesRole 核对「非法重复、错职业、仅手动或未知技能改变了偏好」与「只更新自动施法偏好时改变了全局自动或怪数」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoSkillSettingsProtocolPreservesGlobalAutoAndValidatesRole(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 1)
	// intent 封装意图对应的后续处理，request 关联客户端请求与服务端的对应响应，settings 记录“settings”，使用自动技能“settings”表示。
	intent := func(request uint32, settings *pb.AutoSkillSettings) {
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SAutoBattle, request, 3, &pb.AutoBattleIntent{
			SceneEpoch: r.Scene, AutoSkillSettings: settings,
		})})
	}
	// hosting 封装离线托管对应的后续处理，request 关联客户端请求与服务端的对应响应，enabled 标记是否处于已开启状态。
	// disabled 记录“disabled”，使用可变数量的数值表示。
	hosting := func(request uint32, enabled bool, disabled ...uint32) {
		t.Helper()
		// frame 接收遍历协议帧集合时当前的元素。
		for _, frame := range c.frames {
			if frame.ID == p.MsgS2CHostingState && frame.Request == request {
				// state 记录状态，使用离线托管状态表示。
				var state pb.HostingState
				if proto.Unmarshal(frame.Payload, &state) != nil || state.AutoBattleEnabled != r.Auto || state.AutoSkillSettings == nil ||
					!proto.Equal(state.AutoSkillSettings, &pb.AutoSkillSettings{Enabled: enabled, DisabledSkillIds: disabled}) {
					t.Fatalf("设置回显缺省或未从权威状态规范化：%v", &state)
				}
				return
			}
		}
		t.Fatalf("缺原请求%d的回显", request)
	}
	w.hosting(r, 1)
	hosting(1, true)
	r.Auto = true
	intent(2, &pb.AutoSkillSettings{DisabledSkillIds: []uint32{11, 1}})
	if !r.Auto || r.MonsterCount != 1 {
		t.Fatal("只更新自动施法偏好时改变了全局自动或怪数")
	}
	hosting(2, false, 1, 11)

	// prefs 保存自动技能集合，用于当前步骤中的“prefs”。
	prefs := r.AutoSkills
	// i 定位数值列表列表中正在处理的成员，ids 接收遍历数值列表列表时当前的元素。
	for i, ids := range [][]uint32{{1, 1}, {2}, {21}, {0}} {
		// request 保存由无符号整数“32”取得的请求。
		request := uint32(10 + i)
		c.frames = nil
		intent(request, &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: ids})
		if r.AutoSkills != prefs || len(c.frames) < 2 || c.frames[0].Request != request || c.frames[0].Error != p.CodeInvalidArgument {
			t.Fatal("非法重复、错职业、仅手动或未知技能改变了偏好")
		}
		hosting(request, false, 1, 11)
	}
	r.Ready = false
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 18, 3, &pb.EnterWorldAck{
		SceneEpoch: r.Scene, BaselineId: r.Baseline + 1, AutoSkillSettings: &pb.AutoSkillSettings{Enabled: true},
	})})
	if r.Ready || r.AutoSkills != prefs {
		t.Fatal("不匹配的首屏ACK改变了就绪或现有偏好")
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 19, 3, &pb.EnterWorldAck{
		SceneEpoch: r.Scene, BaselineId: r.Baseline, AutoSkillSettings: &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: []uint32{2}},
	})})
	if !r.Ready || r.AutoSkills != prefs {
		t.Fatal("已失效偏好阻止首屏就绪或覆盖原值")
	}
	r.Ready = false
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SEnterWorldAck, 20, 3, &pb.EnterWorldAck{
		SceneEpoch: r.Scene, BaselineId: r.Baseline, AutoSkillSettings: &pb.AutoSkillSettings{Enabled: true, DisabledSkillIds: []uint32{11}},
	})})
	if !r.Ready || !r.AutoSkills.permits(1) || r.AutoSkills.permits(11) {
		t.Fatal("匹配入场ACK未在Ready前同步偏好")
	}
	r.MapID, r.Auto, r.MonsterCount = 3, false, 0
	intent(21, &pb.AutoSkillSettings{})
	hosting(21, false)
	if r.Auto || r.MonsterCount != 0 {
		t.Fatal("城镇设置偏好改变了当前自动战斗")
	}
}

// TestSkillModeCorrelatesRequestAndKeepsPendingChargeGuard 核对「只改临时开关的技能被永久结算冻结，或重复启用改变修订」与「烈火在永久结算中越过原资源门禁」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSkillModeCorrelatesRequestAndKeepsPendingChargeGuard(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 0, 12, 25, 26)
	r.Pending = true
	// send 封装发送对应的后续处理，request 关联客户端请求与服务端的对应响应，skillID 标识当前业务使用的技能。
	send := func(request, skillID uint32) {
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SSkillModeIntent, request, 3, &pb.SkillModeIntent{
			SceneEpoch: r.Scene, InputSequence: request, SkillId: skillID, Enabled: true,
		})})
	}
	send(1, 12)
	send(2, 25)
	send(3, 12)
	if !r.Melee.Thrusting || !r.Melee.HalfMoon || r.Melee.Revision != 2 || !r.Pending {
		t.Fatal("只改临时开关的技能被永久结算冻结，或重复启用改变修订")
	}
	// acks 创建用于维护确认集合的容器或通道。
	acks := make(map[uint32]bool)
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		acks[frame.Request] = frame.ID == p.MsgS2CSkillModeState && frame.Error == 0
	}
	if !acks[1] || !acks[2] || !acks[3] {
		t.Fatal("技能模式更新未关联对应请求")
	}
	// mana 保存魔法值，用于当前步骤中的魔法值。
	mana := r.Player.MP
	send(4, 26)
	if r.Player.MP != mana || r.Melee.Flaming || c.frames[len(c.frames)-2].Error != p.CodeSkillStateBlocked {
		t.Fatal("烈火在永久结算中越过原资源门禁")
	}
}
