// 本文件将 Go 伤害计算同独立 C# 来源对照向量逐案比对。

package devconfig

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

// 期望值来自tools/combat独立编译Crystal原函数的.NET oracle；只作测试证据，不参与配置装载。
// t 提供断言失败报告、子用例和清理注册。
func TestSourceDamageMatchesIndependentCSharpOracle(t *testing.T) {
	// file 保存由打开取得的文件，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	file, err := os.Open("testdata/source-damage-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// reader 保存由创建“reader”取得的“reader”。
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// vectors 记录测试向量集合，使用临时结构列表表示。
	var vectors []struct {
		// SkillID 标识当前业务使用的技能。
		SkillID uint32 `json:"skill_id"`
		// Rank 记录排序位次，使用数值表示。
		Rank uint32 `json:"rank"`
		// Base 记录基础值，使用数值表示。
		Base int64 `json:"base"`
		// MPowerRoll 记录m能力值随机值，使用数值表示。
		MPowerRoll int64 `json:"mpower_roll"`
		// PowerRoll 记录能力值随机值，使用数值表示。
		PowerRoll int64 `json:"power_roll"`
		// MPowerBase 记录m能力值基础值，使用数值表示。
		MPowerBase int64 `json:"mpower_base"`
		// PowerBase 记录能力值基础值，使用数值表示。
		PowerBase int64 `json:"power_base"`
		// MultiplierBasePPM 记录倍率基础值“ppm”，使用数值表示。
		MultiplierBasePPM int64 `json:"multiplier_base_ppm"`
		// MultiplierBonusPPM 记录倍率加成“ppm”，使用数值表示。
		MultiplierBonusPPM int64 `json:"multiplier_bonus_ppm"`
		// ExpectedPower 记录预期结果能力值，使用数值表示。
		ExpectedPower int64 `json:"expected_power"`
		// ExpectedDamage 记录预期结果伤害，使用数值表示。
		ExpectedDamage int64 `json:"expected_damage"`
	}
	// err 接收解码过程中可能产生的错误，供后续失败分支处理。
	if err := json.NewDecoder(reader).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 29512 {
		t.Fatal("独立源向量数量不完整")
	}
	// i 定位测试向量集合中正在处理的成员，v 接收遍历测试向量集合时当前的元素。
	for i, v := range vectors {
		// s 组装技能所需的数据。
		s := Skill{MultiplierBasePPM: v.MultiplierBasePPM, MultiplierBonusPPM: v.MultiplierBonusPPM}
		// power 保存由来源能力值取得的能力值。
		power := s.SourcePower(v.Rank, v.MPowerBase+v.MPowerRoll, v.PowerBase+v.PowerRoll)
		// damage 保存由来源伤害取得的伤害。
		damage := s.SourceDamage(v.Rank, v.Base, power)
		if power != v.ExpectedPower || damage != v.ExpectedDamage {
			t.Fatalf("C#向量%d skill%d rank%d base%d：power%d/%d damage%d/%d", i, v.SkillID, v.Rank, v.Base, power, v.ExpectedPower, damage, v.ExpectedDamage)
		}
	}
}
