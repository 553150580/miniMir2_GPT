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

func TestSourcePushedMatchesIndependentCSharpOracle(t *testing.T) {
	file, err := os.Open("testdata/source-pushed-vectors.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	var vectors []struct {
		Kind              string            `json:"kind"`
		CanPush           bool              `json:"can_push"`
		Width             int               `json:"width"`
		Height            int               `json:"height"`
		Blocked           []devconfig.Point `json:"blocked"`
		Origin            devconfig.Point   `json:"origin"`
		Direction         uint32            `json:"direction"`
		Distance          int               `json:"distance"`
		Occupied          []devconfig.Point `json:"occupied"`
		Now               int64             `json:"now_ms"`
		InitialAction     int64             `json:"initial_action_ms"`
		InitialMove       int64             `json:"initial_move_ms"`
		InitialAttack     int64             `json:"initial_attack_ms"`
		ExpectedSteps     int               `json:"expected_steps"`
		ExpectedPosition  devconfig.Point   `json:"expected_position"`
		ExpectedDirection uint32            `json:"expected_direction"`
		ExpectedAction    int64             `json:"expected_action_ms"`
		ExpectedMove      int64             `json:"project_expected_move_ms"`
		ExpectedAttack    int64             `json:"expected_attack_ms"`
		Path              []devconfig.Point `json:"expected_path"`
		OwnerPath         []devconfig.Point `json:"expected_owner_path"`
	}
	if err = json.NewDecoder(compressed).Decode(&vectors); err != nil {
		t.Fatal(err)
	}
	template, _, _ := pushedWorker(t)
	for index, vector := range vectors {
		cfg := template.e.Store.Cfg
		cfg.Fixture.Map.Width, cfg.Fixture.Map.Height = vector.Width, vector.Height
		cfg.Fixture.Map.Blocked = append([]devconfig.Point(nil), vector.Blocked...)
		c := &capturedConnection{}
		r := &room{Conn: c, Ready: true, Scene: 20, Role: domain.Role{Job: 0, Level: 7}}
		definition := cfg.Fixture.Monsters[0]
		definition.CanPush = vector.CanPush
		target := &entity{Handle: 10, Kind: 3, Life: 1, Pos: vector.Origin, HP: 10, MaxHP: 10, Direction: 4, Monster: &definition, ActionReadyAt: time.UnixMilli(vector.InitialAction), NextMove: time.UnixMilli(vector.InitialMove), NextAttack: time.UnixMilli(vector.InitialAttack)}
		if vector.Kind == "human" {
			target.Handle, target.Kind = 1, 1
			r.Player = *target
			target = &r.Player
		} else {
			r.Monsters = append(r.Monsters, target)
		}
		// 仅配置oracle给出的动态占格，不能让测试辅助玩家/宠物构成额外障碍。
		for position, point := range vector.Occupied {
			r.Monsters = append(r.Monsters, &entity{Handle: uint32(100 + position), Kind: 3, HP: 10, MaxHP: 10, Pos: point, Monster: &definition})
		}
		r.MonsterCount = len(r.Monsters)
		w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}, rooms: map[string]*room{"role": r}}
		out := w.sourcePushed(r, reference(r, target), vector.Direction, vector.Distance, time.UnixMilli(vector.Now), displacementSource{})
		if out.Steps != vector.ExpectedSteps || target.Pos != vector.ExpectedPosition || target.Direction != vector.ExpectedDirection || target.ActionReadyAt.UnixMilli() != vector.ExpectedAction || target.NextMove.UnixMilli() != vector.ExpectedMove || target.NextAttack.UnixMilli() != vector.ExpectedAttack {
			t.Fatalf("源向量%d(%s dir%d distance%d)不符：steps=%d/%d pos=%v/%v facing=%d/%d action=%d/%d move=%d/%d attack=%d/%d", index, vector.Kind, vector.Direction, vector.Distance, out.Steps, vector.ExpectedSteps, target.Pos, vector.ExpectedPosition, target.Direction, vector.ExpectedDirection, target.ActionReadyAt.UnixMilli(), vector.ExpectedAction, target.NextMove.UnixMilli(), vector.ExpectedMove, target.NextAttack.UnixMilli(), vector.ExpectedAttack)
		}
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
		for position, expected := range vector.Path {
			actual := events[0].Path[position]
			if actual.X != int32(expected.X) || actual.Y != int32(expected.Y) {
				t.Fatalf("源向量%d第%d个实际邻格不符", index, position)
			}
		}
		if vector.Kind == "human" {
			if len(vector.OwnerPath) != len(vector.Path) {
				t.Fatalf("源向量%d自身Pushed与广播路径不符", index)
			}
			for position, expected := range vector.OwnerPath {
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
