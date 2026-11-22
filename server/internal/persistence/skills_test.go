// 验证技能学习与设置的职业、等级、配置边界，以及技能书消耗和已学状态更新。

package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// learnFixture 处理学习开发测试配置。
// t 提供断言失败报告、子用例和清理注册。
func learnFixture(t *testing.T) (domain.Role, AssetRequest, devconfig.Fixture) {
	t.Helper()
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// s 接收技能供后续步骤使用。
	s, _ := cfg.Fixture.Skill(1)
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	return domain.Role{Job: s.JobID, Level: s.LearnLevel, Items: []domain.Item{{ID: id, ItemID: s.BookItemID, Quantity: 2, Bound: true, Version: 1}}}, AssetRequest{ItemID: id, SkillID: s.ID}, cfg.Fixture
}

// TestLearningConsumesOneBookWithZeroLevelWithoutAliasing 核对「学习未原子消费一本、零级未区分已学，或污染输入切片」与「PB库存未保留零级已学技能」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLearningConsumesOneBookWithZeroLevelWithoutAliasing(t *testing.T) {
	// r 接收学习开发测试配置供后续步骤使用，q 接收学习开发测试配置供后续步骤使用，f 接收学习开发测试配置供后续步骤使用。
	r, q, f := learnFixture(t)
	// before 保留加入新条目后的操作前的状态。
	before := append([]domain.Item(nil), r.Items...)
	// out 保存由学习技能取得的输出，code 保存由学习技能取得的结果码。
	out, code := learnSkill(r, q, f)
	// level 保存由技能取得的等级，known 保存由技能取得的已知状态。
	level, known := out.Skill(q.SkillID)
	if code != 0 || !known || level != 0 || out.Items[0].Quantity != 1 || out.Items[0].Version != 2 || !out.Items[0].Bound || !reflect.DeepEqual(r.Items, before) || len(r.Skills) != 0 {
		t.Fatal("学习未原子消费一本、零级未区分已学，或污染输入切片")
	}
	// inventory 接收背包资产供后续步骤使用。
	inventory := out.Inventory()
	if len(inventory.LearnedSkills) != 1 || inventory.LearnedSkills[0].SkillId != q.SkillID || inventory.LearnedSkills[0].Level != 0 {
		t.Fatal("PB库存未保留零级已学技能")
	}
	_, code = learnSkill(out, q, f)
	if code != p.CodeOperationConflict || out.Items[0].Quantity != 1 {
		t.Fatal("重复学习被当作升级消费")
	}
}

// TestLearningRejectsWrongIdentityJobLevelAndBookWithoutMutation 核对「失败学习改变了库存或技能」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLearningRejectsWrongIdentityJobLevelAndBookWithoutMutation(t *testing.T) {
	// change 接收遍历供当前流程调用的函数列表时当前的元素。
	for _, change := range []func(*domain.Role, *AssetRequest){
		// r 记录角色。
		func(r *domain.Role, _ *AssetRequest) { r.Job = 0 },
		// r 记录角色。
		func(r *domain.Role, _ *AssetRequest) { r.Level = 1 },
		// q 记录资产请求。
		func(_ *domain.Role, q *AssetRequest) { q.ItemID = domain.ID() },
		// r 记录角色。
		func(r *domain.Role, _ *AssetRequest) { r.Items[0].ItemID = 21 },
		// r 记录角色。
		func(r *domain.Role, _ *AssetRequest) { r.Items[0].Quantity = 0 },
	} {
		// r 接收学习开发测试配置供后续步骤使用，q 接收学习开发测试配置供后续步骤使用，f 接收学习开发测试配置供后续步骤使用。
		r, q, f := learnFixture(t)
		change(&r, &q)
		// before 保留加入新条目后的操作前的状态。
		before := append([]domain.Item(nil), r.Items...)
		// out 保存由学习技能取得的输出，code 保存由学习技能取得的结果码。
		out, code := learnSkill(r, q, f)
		if code != p.CodeInvalidArgument || !reflect.DeepEqual(before, r.Items) || !reflect.DeepEqual(before, out.Items) || len(out.Skills) != 0 {
			t.Fatal("失败学习改变了库存或技能")
		}
	}
}
