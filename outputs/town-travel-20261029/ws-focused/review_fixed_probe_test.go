package game

import (
	p "minimir/internal/protocol"
	"testing"
	"time"
)

func TestReviewFixedCancelledIDCannotBeRecreatedAfterAnotherTerminal(t *testing.T) {
	w,r,c,now:=travelWorker(t)
	prepareTravelAt(w,r,1,travelToTown,now)
	confirmTravelAt(w,r,1,20,true,now)
	prepareTravelAt(w,r,2,travelToTown,now.Add(time.Millisecond))
	confirmTravelAt(w,r,2,20,true,now.Add(time.Millisecond))
	last:=r.Travel
	prepareTravelAt(w,r,1,travelToTown,now.Add(2*time.Millisecond))
	if r.Travel!=last || r.Travel.Phase!=travelCancelled || c.frames[len(c.frames)-1].Error!=p.CodeOperationConflict {t.Fatal("迟到prepare仍重建已取消ID1")}
	confirmTravelAt(w,r,1,20,false,now.Add(3*time.Millisecond))
	if r.MapID!=1 || r.Scene!=20 || r.Travel!=last || c.frames[len(c.frames)-1].Error!=p.CodeOperationConflict {t.Fatal("迟到commit仍产生场景移交")}
}

func TestReviewFixedUnacknowledgedReturnDoesNotStartNewAI(t *testing.T) {
	w,r,_,now:=travelWorker(t)
	r.Monsters[0].HP,r.Monsters[0].MaxHP=1000,1000
	prepareTravelAt(w,r,1,travelToTown,now)
	confirmTravelAt(w,r,1,20,false,now)
	if !w.finishMapTravelAck(r,now.Add(time.Millisecond)) {t.Fatal("Town ACK前态失败")}
	r.Ready=true
	prepareTravelAt(w,r,2,travelToPersonal,now.Add(time.Second))
	confirmTravelAt(w,r,2,21,false,now.Add(time.Second))
	if r.Ready || len(r.Attacks)+len(r.Casts)!=0 {t.Fatal("返图未ACK且无旧队列前态失败")}
	before:=r.Player.AttackSequence
	w.step(r,now.Add(time.Second+time.Millisecond))
	if r.Player.AttackSequence!=before || len(r.Attacks)+len(r.Casts)!=0 {t.Fatal("资源/基线未ACK仍开始新攻击")}
}
