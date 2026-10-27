package domain

import (
	"crypto/rand"
	"encoding/hex"
	pb "minimir/api/generated/minimirv1"
	"time"
)

func ID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func Bytes(id string) []byte { b, _ := hex.DecodeString(id); return b }
func Hex(b []byte) string    { return hex.EncodeToString(b) }

type Account struct {
	ID           string `bson:"_id" json:"account_id"`
	Name         string `bson:"name" json:"account"`
	Salt         []byte `bson:"salt" json:"-"`
	PasswordHash []byte `bson:"password_hash" json:"-"`
	Active       bool   `bson:"active" json:"-"`
	AuthVersion  int64  `bson:"auth_version" json:"-"`
	AuthGuard    int64  `bson:"auth_guard_seq" json:"-"`
}
type Wallet struct {
	Gold         int64 `bson:"gold" json:"gold"`
	BoundGold    int64 `bson:"bound_gold" json:"bound_gold"`
	Yuanbao      int64 `bson:"yuanbao_units" json:"yuanbao_units"`
	BoundYuanbao int64 `bson:"bound_yuanbao_units" json:"bound_yuanbao_units"`
}

func (w Wallet) Proto(rev int64) *pb.WalletState {
	return &pb.WalletState{Gold: w.Gold, BoundGold: w.BoundGold, YuanbaoUnits: w.Yuanbao, BoundYuanbaoUnits: w.BoundYuanbao, Revision: uint64(rev)}
}

type Item struct {
	ID       string `bson:"instance_id" json:"instance_id"`
	ItemID   uint32 `bson:"item_id" json:"item_id"`
	Quantity int64  `bson:"quantity" json:"quantity"`
	Bound    bool   `bson:"bound" json:"bound"`
	Version  int64  `bson:"version" json:"version"`
}

func (i Item) Proto() *pb.ItemView {
	return &pb.ItemView{InstanceId: Bytes(i.ID), ItemId: i.ItemID, Quantity: i.Quantity, Bound: i.Bound, Version: uint64(i.Version)}
}

type Equipped struct {
	Slot uint32 `bson:"slot" json:"slot"`
	Item Item   `bson:"item" json:"item"`
}
type Report struct {
	SessionID   string `bson:"session_id" json:"hosting_session_id"`
	UsedSeconds int64  `bson:"used_seconds" json:"used_seconds"`
	Kills       int64  `bson:"kills" json:"kills"`
	Experience  int64  `bson:"experience" json:"experience"`
	Earned      Wallet `bson:"earned" json:"earned"`
	Drops       []Item `bson:"drops" json:"drops"`
	StopReason  uint32 `bson:"stop_reason" json:"stop_reason"`
	Sequence    int64  `bson:"sequence" json:"settlement_sequence"`
}
type Role struct {
	ID               string         `bson:"_id" json:"role_id"`
	AccountID        string         `bson:"account_id" json:"account_id"`
	Origin           uint64         `bson:"origin_server_id" json:"origin_server_id"`
	Slot             uint32         `bson:"slot" json:"slot"`
	Job              uint32         `bson:"job" json:"job"`
	Gender           uint32         `bson:"gender" json:"gender"`
	Name             string         `bson:"name" json:"name"`
	NameKey          string         `bson:"name_key" json:"-"`
	Level            uint32         `bson:"level" json:"level"`
	Experience       int64          `bson:"experience" json:"experience"`
	Revision         int64          `bson:"revision" json:"revision"`
	EconomicSeq      int64          `bson:"economic_command_seq" json:"economic_command_seq"`
	SourceSeq        int64          `bson:"source_seq" json:"source_seq"`
	Owner            string         `bson:"owner_instance" json:"-"`
	RunEpoch         int64          `bson:"run_epoch" json:"-"`
	SessionEpoch     int64          `bson:"session_epoch" json:"binding_epoch"`
	Wallet           Wallet         `bson:"wallet" json:"wallet"`
	Items            []Item         `bson:"items" json:"items"`
	Equipped         []Equipped     `bson:"equipped" json:"equipped"`
	Capacity         int            `bson:"capacity" json:"capacity"`
	PickupStopped    bool           `bson:"pickup_stopped" json:"pickup_stopped"`
	LastDiscardItem  uint32         `bson:"last_discard_item_id" json:"last_discard_item_id"`
	LastDiscardBound bool           `bson:"last_discard_bound" json:"last_discard_bound"`
	HostingSeconds   int64          `bson:"hosting_seconds" json:"hosting_seconds"`
	ClaimDay         string         `bson:"claim_day" json:"-"`
	ClaimedSeconds   int64          `bson:"claimed_seconds" json:"-"`
	NoEquipmentKills int            `bson:"no_equipment_kills" json:"-"`
	PrivilegeUntil   time.Time      `bson:"privilege_until" json:"-"`
	HostingCardUntil time.Time      `bson:"hosting_card_until" json:"-"`
	Report           Report         `bson:"hosting_report" json:"hosting_report"`
	Skills           []LearnedSkill `bson:"learned_skills" json:"learned_skills"`
}
type LearnedSkill struct {
	SkillID uint32 `bson:"skill_id" json:"skill_id"`
	Level   uint32 `bson:"level" json:"level"`
}

func (r Role) Skill(id uint32) (uint32, bool) {
	for _, s := range r.Skills {
		if s.SkillID == id {
			return s.Level, true
		}
	}
	return 0, false
}

func (r Role) Summary() *pb.RoleSummary {
	return &pb.RoleSummary{Key: &pb.RoleKey{RoleId: Bytes(r.ID), OriginServerId: r.Origin}, Name: r.Name, Job: r.Job, Gender: r.Gender, Level: r.Level, Slot: r.Slot, AppearanceVersion: 1}
}
func (r Role) Inventory() *pb.InventoryResult {
	p := &pb.InventoryResult{Capacity: uint32(r.Capacity), Revision: uint64(r.Revision), Level: r.Level, Experience: r.Experience}
	for _, i := range r.Items {
		p.Items = append(p.Items, i.Proto())
	}
	for _, i := range r.Equipped {
		p.Equipped = append(p.Equipped, &pb.EquippedItem{Slot: i.Slot, Item: i.Item.Proto()})
	}
	for _, s := range r.Skills {
		p.LearnedSkills = append(p.LearnedSkills, &pb.LearnedSkill{SkillId: s.SkillID, Level: s.Level})
	}
	return p
}
func (r Role) HostingReport() *pb.HostingReport {
	p := &pb.HostingReport{HostingSessionId: Bytes(r.Report.SessionID), UsedSeconds: uint64(r.Report.UsedSeconds), Kills: uint64(r.Report.Kills), Experience: r.Report.Experience, Earned: r.Report.Earned.Proto(r.Report.Sequence), StopReason: r.Report.StopReason, SettlementSequence: uint64(r.Report.Sequence)}
	for _, i := range r.Report.Drops {
		p.Drops = append(p.Drops, i.Proto())
	}
	return p
}
