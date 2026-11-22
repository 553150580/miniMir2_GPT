// 本文件调度独立入场和业务数据库队列，将执行结果投递回原房间分片。
package game

import "context"

const (
	// databaseBusinessWorkers 上限是处理普通资产操作与收益结算的工作协程数。
	databaseBusinessWorkers = 48
	// databaseAdmissionWorkers 上限是专供票据校验与入场事务使用的工作协程数。
	databaseAdmissionWorkers = 6
)

// startDatabaseWorkers 为入场、普通业务与收益结算建立受控的数据库执行协程。
// e 是调用该方法的游戏引擎实例。
func (e *Engine) startDatabaseWorkers() {
	// 入场有独立的执行协程和槽位；票据查询与入场事务分别排队，避免前一阶段占满后一阶段。
	for range databaseAdmissionWorkers {
		e.wg.Go(func() { e.runDatabaseJobs(e.admissionEntries, e.admissionTickets) })
	}
	for range databaseBusinessWorkers {
		e.wg.Go(func() { e.runDatabaseJobs(e.jobs, e.settlementJobs) })
	}
}

// runDatabaseJobs 异步完成数据库操作并将回调移交房间原分片。
// e 是调用该方法的游戏引擎实例，primary 是当前轮询中先尝试读取的数据库队列，secondary 是当前轮询中的备用队列，下一轮与优先队列互换。
func (e *Engine) runDatabaseJobs(primary, secondary <-chan job) {
	// 两条队列同时繁忙时交替服务；一条暂时空闲时另一条可借用全部执行能力。
	for e.ctx.Err() == nil {
		// j 是取出的数据库任务，携带执行闭包和回调所属的房间分片，ok 表明队列返回了一项工作，引擎停止时为假。
		j, ok := takeDatabaseJob(e.ctx, primary, secondary)
		if !ok {
			return
		}
		primary, secondary = secondary, primary
		// callback 把本次数据库确认结果带回原房间执行器，写操作协程无需修改房间。
		callback := j.run()
		if callback == nil {
			continue
		}
		select {
		// 数据库协程交回结果，只有房间分片能用该回调改写内存状态。
		case j.target.done <- callback:
		case <-e.ctx.Done():
			return
		}
	}
}

// takeDatabaseJob 优先尝试指定队列，两个队列为空时等待任务或取消。
// ctx 在游戏引擎停止之后中断队列等待，primary 是本轮优先读取的数据库任务队列，secondary 在优先队列暂时空闲时提供任务。
func takeDatabaseJob(ctx context.Context, primary, secondary <-chan job) (job, bool) {
	select {
	// j 接收优先通道送来的当前位置的下标。
	case j := <-primary:
		return j, true
	default:
	}
	select {
	// j 接收优先通道送来的当前位置的下标。
	case j := <-primary:
		return j, true
	// j 接收次优先通道送来的当前位置的下标。
	case j := <-secondary:
		return j, true
	case <-ctx.Done():
		return job{}, false
	}
}

// offerDatabaseJob 尝试无阻塞入队，满时返回失败供业务选择拒绝或重试。
// queue 是调用者选择的目标队列，满载时立即拒绝投递，j 记录当前位置的下标，使用任务或职业编号表示。
func offerDatabaseJob(queue chan<- job, j job) bool {
	select {
	case queue <- j:
		return true
	default:
		return false
	}
}

// submitEntry 使用独立入场事务队列，手工测试装配可借用已有业务队列。
// e 是调用该方法的游戏引擎实例，j 记录当前位置的下标，使用任务或职业编号表示。
func (e *Engine) submitEntry(j job) bool {
	// queue 通常指向独立入场队列，仅手工装配的测试引擎使用普通业务队列。
	queue := e.admissionEntries
	if queue == nil {
		// 手工装配的房间测试沿用单一任务队列。
		queue = e.jobs
	}
	return offerDatabaseJob(queue, j)
}

// submit 选择独立结算队列或普通业务队列，保留已有任务顺序。
// w 是调用该方法的房间执行器实例，j 记录当前位置的下标，使用任务或职业编号表示。
func (w *worker) submit(j job) bool {
	if j.admission {
		return w.e.submitEntry(j)
	}
	// queue 初始指向普通业务队列，收益结算任务可改走单独的结算队列。
	queue := w.e.jobs
	if j.settlement && w.e.settlementJobs != nil {
		// 每房间至多有一笔待确认的结算。有限 FIFO 容纳已产生的收益，保留首次提交的顺序。
		queue = w.e.settlementJobs
	}
	return offerDatabaseJob(queue, j)
}
