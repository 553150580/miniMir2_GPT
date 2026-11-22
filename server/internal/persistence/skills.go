// 角色学习技能时的持久变更。校验职业、等级及技能书实例，扣除原书并记录配置给定的初始技能等级。

package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// learnSkill 核验技能定义、职业等级和原技能书后消耗一本书，新增初始已学技能。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func learnSkill(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, uint32) {
	// s 接收技能供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	s, ok := f.Skill(q.SkillID)
	if !ok || s.JobID != r.Job || r.Level < s.LearnLevel {
		return r, p.CodeInvalidArgument
	}
	// learned 保存由技能取得的已学习。
	if _, learned := r.Skill(q.SkillID); learned {
		return r, p.CodeOperationConflict
	}
	// index 保存当前处理所用的下标。
	index := -1
	// i 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			index = i
			break
		}
	}
	if index < 0 {
		return r, p.CodeInvalidArgument
	}
	// item 保存从道具集合取出的道具。
	item := r.Items[index]
	// book 保存由道具取得的技能书，exists 保存由道具取得的存在。
	book, exists := f.Item(item.ItemID)
	if !exists || book.BookSkillID != s.ID || item.ItemID != s.BookItemID || item.Quantity <= 0 || item.Version < 1 || item.Version == math.MaxInt64 {
		return r, p.CodeInvalidArgument
	}
	r.Items = append([]domain.Item(nil), r.Items...)
	if item.Quantity == 1 {
		r.Items = append(r.Items[:index], r.Items[index+1:]...)
	} else {
		r.Items[index].Quantity--
		r.Items[index].Version++
	}
	r.Skills = append(append([]domain.LearnedSkill(nil), r.Skills...), domain.LearnedSkill{SkillID: s.ID, Level: s.LearnedLevel})
	return r, 0
}
