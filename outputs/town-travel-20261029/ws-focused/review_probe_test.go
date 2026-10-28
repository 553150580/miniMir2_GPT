package game

import (
	"testing"
	"time"
)

func TestReviewCancelledTravelIDCanBeRecreatedAfterAnotherTerminal(t *testing.T) {
	w,r,c,now:=travelWorker(t)
	prepareTravelAt(w,r,1,travelToTown,now)
	confirmTravelAt(w,r,1,20,true,now)
	if r.Travel.Phase!=travelCancelled {t.Fatal("前态不成立")}
	prepareTravelAt(w,r,2,travelToTown,now.Add(time.Millisecond))
	confirmTravelAt(w,r,2,20,true,now.Add(time.Millisecond))
	if r.Travel.Phase!=travelCancelled {t.Fatal("第二前态不成立")}
	prepareTravelAt(w,r,1,travelToTown,now.Add(2*time.Millisecond))
	if r.Travel.ID!=travelID(1) || r.Travel.Phase!=travelPrepared {t.Fatal("旧ID未被重新建立，未复现")}
	confirmTravelAt(w,r,1,20,false,now.Add(3*time.Millisecond))
	if r.MapID!=2 || r.Scene!=21 {t.Fatal("旧确认未能再次提交，未复现")}
	t.Logf("实际复现：已取消ID1，经ID2取消后，迟到的ID1 prepare/commit仍将scene20切至Town；最终phase=%d",lastTravelState(t,c).Phase)
}

func TestReviewUnacknowledgedReturnedPersonalWorldStartsNewAttacks(t *testing.T) {
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
	if r.Player.AttackSequence==before && len(r.Attacks)+len(r.Casts)==0 {t.Fatal("未建立新AI动作，未复现")}
	t.Logf("实际复现：Ready=%t、Travel.Acked=%t 时新建攻击；PlayerAttackSequence %d→%d、attacks=%d casts=%d",r.Ready,r.Travel.Acked,before,r.Player.AttackSequence,len(r.Attacks),len(r.Casts))
}
