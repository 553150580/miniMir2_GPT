package game

import "context"

const (
	databaseBusinessWorkers  = 48
	databaseAdmissionWorkers = 6
)

func (e *Engine) startDatabaseWorkers() {
	// 入场有独立的执行协程和槽位；票据查询与入场事务分别排队，避免前一阶段占满后一阶段。
	for range databaseAdmissionWorkers {
		e.wg.Go(func() { e.runDatabaseJobs(e.admissionEntries, e.admissionTickets) })
	}
	for range databaseBusinessWorkers {
		e.wg.Go(func() { e.runDatabaseJobs(e.jobs, e.settlementJobs) })
	}
}

func (e *Engine) runDatabaseJobs(primary, secondary <-chan job) {
	// 两条队列同时繁忙时交替服务；一条暂时空闲时另一条可借用全部执行能力。
	for e.ctx.Err() == nil {
		j, ok := takeDatabaseJob(e.ctx, primary, secondary)
		if !ok {
			return
		}
		primary, secondary = secondary, primary
		callback := j.run()
		if callback == nil {
			continue
		}
		select {
		case j.target.done <- callback:
		case <-e.ctx.Done():
			return
		}
	}
}

func takeDatabaseJob(ctx context.Context, primary, secondary <-chan job) (job, bool) {
	select {
	case j := <-primary:
		return j, true
	default:
	}
	select {
	case j := <-primary:
		return j, true
	case j := <-secondary:
		return j, true
	case <-ctx.Done():
		return job{}, false
	}
}

func offerDatabaseJob(queue chan<- job, j job) bool {
	select {
	case queue <- j:
		return true
	default:
		return false
	}
}

func (e *Engine) submitEntry(j job) bool {
	queue := e.admissionEntries
	if queue == nil {
		// 手工装配的房间测试沿用单一任务队列。
		queue = e.jobs
	}
	return offerDatabaseJob(queue, j)
}

func (w *worker) submit(j job) bool {
	queue := w.e.jobs
	if j.settlement && w.e.settlementJobs != nil {
		// 每房间至多有一笔待确认的结算。有限 FIFO 容纳已产生的收益，保留首次提交的顺序。
		queue = w.e.settlementJobs
	}
	return offerDatabaseJob(queue, j)
}
