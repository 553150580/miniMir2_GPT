package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func learnSkill(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, uint32) {
	s, ok := f.Skill(q.SkillID)
	if !ok || s.JobID != r.Job || r.Level < s.LearnLevel {
		return r, p.CodeInvalidArgument
	}
	if _, learned := r.Skill(q.SkillID); learned {
		return r, p.CodeOperationConflict
	}
	index := -1
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			index = i
			break
		}
	}
	if index < 0 {
		return r, p.CodeInvalidArgument
	}
	item := r.Items[index]
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
