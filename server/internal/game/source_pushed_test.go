// 本文件用独立 C# 来源判例回归受推路径和状态处理。
package game

import (
	"compress/gzip"
	"encoding/json"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"os"
	"testing"
	"time"
)

// TestSourcePushedMatchesIndependentCSharpOracle 核对「源向量( dir distance)不符」与「源向量自身Pushed与广播路径不符」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSourcePushedMatchesIndependentCSharpOracle(t *testing.T) {
	// file 保存由打开取得的文件，err 接收打开过程中可能产生的错误，供后续失败分支处理。
	file, err := os.Open("testdata/source-pushed-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	// compressed 保存由创建“reader”取得的压缩内容。
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	// vectors 记录测试向量集合，使用临时结构列表表示。
	var vectors []struct {
		// Kind 记录类型，使用文本表示。
		Kind string `json:"kind"`
		// CanPush 标记是否处于“can”推入状态。
		CanPush bool `json:"can_push"`
		// Width 记录宽度，使用数值表示。
		Width int `json:"width"`
		// Height 记录高度，使用数值表示。
		Height int `json:"height"`
		// Blocked 记录阻挡，使用点位列表表示。
		Blocked []devconfig.Point `json:"blocked"`
		// Origin 记录大区或起点，使用点位表示。
		Origin devconfig.Point `json:"origin"`
		// Direction 记录朝向，使用数值表示。
		Direction uint32 `json:"direction"`
		// Distance 记录距离，使用数值表示。
		Distance int `json:"distance"`
		// Occupied 记录已占用，使用点位列表表示。
		Occupied []devconfig.Point `json:"occupied"`
		// Now 记录当前时间，使用数值表示。
		Now int64 `json:"now_ms"`
		// InitialAction 记录初始值动作，使用数值表示。
		InitialAction int64 `json:"initial_action_ms"`
		// InitialMove 记录初始值移动，使用数值表示。
		InitialMove int64 `json:"initial_move_ms"`
		// InitialAttack 记录初始值攻击，使用数值表示。
		InitialAttack int64 `json:"initial_attack_ms"`
		// ExpectedSteps 记录预期结果步骤集合，使用数值表示。
		ExpectedSteps int `json:"expected_steps"`
		// ExpectedPosition 记录预期结果位置，使用点位表示。
		ExpectedPosition devconfig.Point `json:"expected_position"`
		// ExpectedDirection 记录预期结果朝向，使用数值表示。
		ExpectedDirection uint32 `json:"expected_direction"`
		// ExpectedAction 记录预期结果动作，使用数值表示。
		ExpectedAction int64 `json:"expected_action_ms"`
		// ExpectedMove 记录预期结果移动，使用数值表示。
		ExpectedMove int64 `json:"project_expected_move_ms"`
		// ExpectedAttack 记录预期结果攻击，使用数值表示。
		ExpectedAttack int64 `json:"expected_attack_ms"`
		// Path 记录路径，使用点位列表表示。
		Path []devconfig.Point `json:"expected_path"`
		// OwnerPath 记录所有者路径，使用点位列表表示。
		OwnerPath []devconfig.Point `json:"expected_owner_path"`
	}
	if err = json.NewDecoder(compressed).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	// template 保存由被推动位移房间执行器取得的模板。
	template, _, _ := pushedWorker(t)
	// index 定位测试向量集合中正在处理的成员，vector 接收遍历测试向量集合时当前的元素。
	for index, vector := range vectors {
		// cfg 保存当前处理所用的服务配置。
		cfg := template.e.Store.Cfg
		cfg.Fixture.Map.Width, cfg.Fixture.Map.Height = vector.Width, vector.Height
		cfg.Fixture.Map.Blocked = append([]devconfig.Point(nil), vector.Blocked...)
		// c 保存当前处理所用的已捕获连接。
		c := &capturedConnection{}
		// r 保存当前处理所用的房间。
		r := &room{Conn: c, Ready: true, Scene: 20, Role: domain.Role{Job: 0, Level: 7}, worldState: &worldState{}}
		// definition 保存从怪物集合取出的定义。
		definition := cfg.Fixture.Monsters[0]
		definition.CanPush = vector.CanPush
		// target 保存当前处理所用的目标。
		target := &entity{Handle: 10, Kind: 3, Life: 1, Pos: vector.Origin, HP: 10, MaxHP: 10, Direction: 4, Monster: &definition, ActionReadyAt: time.UnixMilli(vector.InitialAction), NextMove: time.UnixMilli(vector.InitialMove), NextAttack: time.UnixMilli(vector.InitialAttack)}
		if vector.Kind == "human" {
			target.Handle, target.Kind = 1, 1
			r.Player = *target
			target = &r.Player
		} else {
			r.Monsters = append(r.Monsters, target)
		}
		// 仅配置oracle给出的动态占格，不能让测试辅助玩家/宠物构成额外障碍。
		// position 定位已占用中正在处理的成员，point 接收遍历已占用时当前的元素。
		for position, point := range vector.Occupied {
			r.Monsters = append(r.Monsters, &entity{Handle: uint32(100 + position), Kind: 3, HP: 10, MaxHP: 10, Pos: point, Monster: &definition})
		}
		r.MonsterCount = len(r.Monsters)
		// w 保存当前处理所用的房间执行器。
		w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{"role": r}}
		// out 保存由来源被推动位移取得的输出。
		out := w.sourcePushed(r, reference(r, target), vector.Direction, vector.Distance, time.UnixMilli(vector.Now), displacementSource{})
		if out.Steps != vector.ExpectedSteps || target.Pos != vector.ExpectedPosition || target.Direction != vector.ExpectedDirection || target.ActionReadyAt.UnixMilli() != vector.ExpectedAction || target.NextMove.UnixMilli() != vector.ExpectedMove || target.NextAttack.UnixMilli() != vector.ExpectedAttack {
			t.Fatalf("源向量%d(%s dir%d distance%d)不符：steps=%d/%d pos=%v/%v facing=%d/%d action=%d/%d move=%d/%d attack=%d/%d", index, vector.Kind, vector.Direction, vector.Distance, out.Steps, vector.ExpectedSteps, target.Pos, vector.ExpectedPosition, target.Direction, vector.ExpectedDirection, target.ActionReadyAt.UnixMilli(), vector.ExpectedAction, target.NextMove.UnixMilli(), vector.ExpectedMove, target.NextAttack.UnixMilli(), vector.ExpectedAttack)
		}
		// events 保存由位移事件集合取得的事件集合。
		events := displacementEvents(t, c)
		if len(vector.Path) == 0 {
			if len(events) != 0 || target.MovementRevision != 0 {
				t.Fatalf("源向量%d零步伪造路径或水位", index)
			}
			continue
		}
		if len(events) != 1 || len(events[0].Path) != len(vector.Path) || target.MovementRevision != 1 {
			t.Fatalf("源向量%d成功前缀丢失或拆成额外事件", index)
		}
		// position 定位路径中正在处理的成员，expected 接收遍历路径时当前的元素。
		for position, expected := range vector.Path {
			// actual 保存从路径取出的实测。
			actual := events[0].Path[position]
			if actual.X != int32(expected.X) || actual.Y != int32(expected.Y) {
				t.Fatalf("源向量%d第%d个实际邻格不符", index, position)
			}
		}
		if vector.Kind == "human" {
			if len(vector.OwnerPath) != len(vector.Path) {
				t.Fatalf("源向量%d自身Pushed与广播路径不符", index)
			}
			// position 定位所有者路径中正在处理的成员，expected 接收遍历所有者路径时当前的元素。
			for position, expected := range vector.OwnerPath {
				// actual 保存从路径取出的实测。
				actual := events[0].Path[position]
				if actual.X != int32(expected.X) || actual.Y != int32(expected.Y) {
					t.Fatalf("源向量%d自身路径不符", index)
				}
			}
		}
	}
	if len(vectors) != 3892 {
		t.Fatalf("独立来源向量数量变化：%d", len(vectors))
	}
}
