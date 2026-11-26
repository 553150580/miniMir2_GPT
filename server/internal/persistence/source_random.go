// 本文件将不可变服务端来源转换为可重放的等概率抽样。
package persistence

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strconv"
)

// sourceDraw 在正整数上限内使用拒绝抽样，调用方通过种子区分不同业务及抽样步骤。
func sourceDraw(seed string, bound uint32) uint32 {
	span := uint64(bound)
	limit := uint64(math.MaxUint64) - uint64(math.MaxUint64)%span
	for round := uint64(0); ; round++ {
		digest := sha256.Sum256([]byte(seed + ":" + strconv.FormatUint(round, 10)))
		n := binary.BigEndian.Uint64(digest[:8])
		if n < limit {
			return uint32(n % span)
		}
	}
}
