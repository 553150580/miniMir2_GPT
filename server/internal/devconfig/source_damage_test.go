package devconfig

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"testing"
)

// 期望值来自tools/combat独立编译Crystal原函数的.NET oracle；只作测试证据，不参与配置装载。
func TestSourceDamageMatchesIndependentCSharpOracle(t *testing.T) {
	file, err := os.Open("testdata/source-damage-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var vectors []struct {
		SkillID            uint32 `json:"skill_id"`
		Rank               uint32 `json:"rank"`
		Base               int64  `json:"base"`
		MPowerRoll         int64  `json:"mpower_roll"`
		PowerRoll          int64  `json:"power_roll"`
		MPowerBase         int64  `json:"mpower_base"`
		PowerBase          int64  `json:"power_base"`
		MultiplierBasePPM  int64  `json:"multiplier_base_ppm"`
		MultiplierBonusPPM int64  `json:"multiplier_bonus_ppm"`
		ExpectedPower      int64  `json:"expected_power"`
		ExpectedDamage     int64  `json:"expected_damage"`
	}
	if err := json.NewDecoder(reader).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 29512 {
		t.Fatal("独立源向量数量不完整")
	}
	for i, v := range vectors {
		s := Skill{MultiplierBasePPM: v.MultiplierBasePPM, MultiplierBonusPPM: v.MultiplierBonusPPM}
		power := s.SourcePower(v.Rank, v.MPowerBase+v.MPowerRoll, v.PowerBase+v.PowerRoll)
		damage := s.SourceDamage(v.Rank, v.Base, power)
		if power != v.ExpectedPower || damage != v.ExpectedDamage {
			t.Fatalf("C#向量%d skill%d rank%d base%d：power%d/%d damage%d/%d", i, v.SkillID, v.Rank, v.Base, power, v.ExpectedPower, damage, v.ExpectedDamage)
		}
	}
}
