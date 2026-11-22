package game

import (
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

var benchmarkBaselineFrame []byte

func BenchmarkPersonalBaseline(b *testing.B) {
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		b.Fatal(err)
	}
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}}
	r := &room{}
	w.initialize(r, domain.Role{Job: 0, Level: 45})
	r.MonsterCount = len(r.Monsters)
	b.ReportAllocs()
	for b.Loop() {
		chunks, err := w.buildBaseline(r, 0)
		if err != nil || len(chunks) != 1 {
			b.Fatalf("普通挂机场景没有生成单帧基线：%v", err)
		}
		benchmarkBaselineFrame = wire.Encode(wire.Message(p.MsgS2CSceneBaseline, 0, 1, chunks[0]))
	}
}
