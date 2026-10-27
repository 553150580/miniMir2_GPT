package persistence

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

func learnFixture(t *testing.T) (domain.Role, AssetRequest, devconfig.Fixture) {
	t.Helper()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := cfg.Fixture.Skill(1)
	id := domain.ID()
	return domain.Role{Job: s.JobID, Level: s.LearnLevel, Items: []domain.Item{{ID: id, ItemID: s.BookItemID, Quantity: 2, Bound: true, Version: 1}}}, AssetRequest{ItemID: id, SkillID: s.ID}, cfg.Fixture
}
func TestLearningConsumesOneBookWithZeroLevelWithoutAliasing(t *testing.T) {
	r, q, f := learnFixture(t)
	before := append([]domain.Item(nil), r.Items...)
	out, code := learnSkill(r, q, f)
	level, known := out.Skill(q.SkillID)
	if code != 0 || !known || level != 0 || out.Items[0].Quantity != 1 || out.Items[0].Version != 2 || !out.Items[0].Bound || !reflect.DeepEqual(r.Items, before) || len(r.Skills) != 0 {
		t.Fatal("学习未原子消费一本、零级未区分已学，或污染输入切片")
	}
	inventory := out.Inventory()
	if len(inventory.LearnedSkills) != 1 || inventory.LearnedSkills[0].SkillId != q.SkillID || inventory.LearnedSkills[0].Level != 0 {
		t.Fatal("PB库存未保留零级已学技能")
	}
	_, code = learnSkill(out, q, f)
	if code != p.CodeOperationConflict || out.Items[0].Quantity != 1 {
		t.Fatal("重复学习被当作升级消费")
	}
}
func TestLearningRejectsWrongIdentityJobLevelAndBookWithoutMutation(t *testing.T) {
	for _, change := range []func(*domain.Role, *AssetRequest){
		func(r *domain.Role, _ *AssetRequest) { r.Job = 0 },
		func(r *domain.Role, _ *AssetRequest) { r.Level = 1 },
		func(_ *domain.Role, q *AssetRequest) { q.ItemID = domain.ID() },
		func(r *domain.Role, _ *AssetRequest) { r.Items[0].ItemID = 21 },
		func(r *domain.Role, _ *AssetRequest) { r.Items[0].Quantity = 0 },
	} {
		r, q, f := learnFixture(t)
		change(&r, &q)
		before := append([]domain.Item(nil), r.Items...)
		out, code := learnSkill(r, q, f)
		if code != p.CodeInvalidArgument || !reflect.DeepEqual(before, r.Items) || !reflect.DeepEqual(before, out.Items) || len(out.Skills) != 0 {
			t.Fatal("失败学习改变了库存或技能")
		}
	}
}
