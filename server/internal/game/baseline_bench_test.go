// 本文件测量普通个人挂机场景构造和编码完整基线的时间与内存分配。
package game

import (
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// benchmarkBaselineFrame 记录“benchmark”场景基线协议帧，使用字节序列表示。
var benchmarkBaselineFrame []byte

// BenchmarkPersonalBaseline 核验普通挂机场景应生成单帧基线。
// b 控制基准循环并收集性能指标。
func BenchmarkPersonalBaseline(b *testing.B) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		b.Fatal(err)
	}
	// w 保存当前处理所用的房间执行器。
	w := &worker{e: &Engine{Store: &persistence.Store{Cfg: cfg}}}
	// r 保存当前处理所用的房间。
	r := &room{}
	w.initialize(r, domain.Role{Job: 0, Level: 45})
	r.MonsterCount = len(r.Monsters)
	b.ReportAllocs()
	for b.Loop() {
		// chunks 保存由构建场景基线取得的“chunks”，err 接收构建场景基线过程中可能产生的错误，供后续失败分支处理。
		chunks, err := w.buildBaseline(r, 0)
		if err != nil || len(chunks) != 1 {
			b.Fatalf("普通挂机场景没有生成单帧基线：%v", err)
		}
		benchmarkBaselineFrame = wire.Encode(wire.Message(p.MsgS2CSceneBaseline, 0, 1, chunks[0]))
	}
}
