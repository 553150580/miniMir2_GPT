// 本文件维护首领场内的自动战斗开关，命令只作用于当前挑战现场。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// bossAutoBattleIntent 更新挑战的开关或职业施法偏好，沿用自动战斗确认协议。
func (w *worker) bossAutoBattleIntent(battle *room, frame wire.Frame) {
	var intent pb.AutoBattleIntent // 只有当前首领场景身份有效，场内怪物数量固定。
	if proto.Unmarshal(frame.Payload, &intent) != nil || len(intent.ProtoReflect().GetUnknown()) != 0 ||
		intent.SceneEpoch != battle.Scene || intent.MonsterCount != 0 {
		w.failure(battle, frame, p.CodeInvalidArgument)
		return
	}
	if intent.AutoSkillSettings != nil {
		settings, valid := w.validateAutoSkillSettings(battle, intent.AutoSkillSettings)
		if !valid {
			w.failure(battle, frame, p.CodeInvalidArgument)
			return
		}
		battle.AutoSkills = settings
	} else {
		battle.Auto = intent.Enabled
		if !battle.Auto {
			battle.AutoTarget = actorRef{}
		}
	}
	w.hosting(battle, frame.Request)
}
