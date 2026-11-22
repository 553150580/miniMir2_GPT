package main

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"minimir/internal/persistence"
)

type soakHostingRole struct {
	ID        string `bson:"_id"`
	Remaining int64  `bson:"hosting_seconds"`
	Report    struct {
		Kills      int64  `bson:"kills"`
		StopReason uint32 `bson:"stop_reason"`
	} `bson:"hosting_report"`
}

type soakHostingSample struct {
	ExpectedRoles     int   `json:"expected_roles"`
	ObservedRoles     int   `json:"observed_roles"`
	DebitedRoles      int   `json:"debited_roles"`
	DebitedThisWindow int   `json:"debited_roles_this_window"`
	DebitedSeconds    int64 `json:"debited_seconds"`
	SecondsThisWindow int64 `json:"debited_seconds_this_window"`
	ShownOfflineKills int   `json:"roles_with_observed_offline_kills"`
	StoppedByDeath    int   `json:"roles_with_death_stop"`
	AvailableBalance  int64 `json:"total_remaining_seconds"`
}

type soakHostingLedger struct {
	ids      []string
	previous map[string]int64
	fought   map[string]bool
	latest   soakHostingSample
}

func newSoakHostingLedger(roles []loadIdentity) *soakHostingLedger {
	ids := make([]string, len(roles))
	previous := make(map[string]int64, len(roles))
	for i, role := range roles {
		ids[i] = role.ID
		previous[role.ID] = soakHostingBalance
	}
	return &soakHostingLedger{ids: ids, previous: previous, fought: make(map[string]bool)}
}

func (l *soakHostingLedger) sample(ctx context.Context, store *persistence.Store) (soakHostingSample, error) {
	ctx, stop := context.WithTimeout(ctx, 8*time.Second)
	defer stop()
	cursor, err := store.DB.Collection("roles").Find(ctx, bson.M{"_id": bson.M{"$in": l.ids}},
		options.Find().SetProjection(bson.M{"hosting_seconds": 1, "hosting_report.kills": 1, "hosting_report.stop_reason": 1}))
	if err != nil {
		return soakHostingSample{}, err
	}
	var rows []soakHostingRole
	if err = cursor.All(ctx, &rows); err != nil {
		return soakHostingSample{}, err
	}
	if len(rows) != len(l.ids) {
		return soakHostingSample{}, fmt.Errorf("托管持久化抽查预期 %d 个角色，实际 %d", len(l.ids), len(rows))
	}
	report := soakHostingSample{ExpectedRoles: len(l.ids), ObservedRoles: len(rows)}
	for _, role := range rows {
		last := l.previous[role.ID]
		if role.Remaining < 0 || role.Remaining > last {
			return soakHostingSample{}, fmt.Errorf("托管时长余额越界或逆向增加")
		}
		report.AvailableBalance += role.Remaining
		report.DebitedSeconds += soakHostingBalance - role.Remaining
		if role.Remaining < soakHostingBalance {
			report.DebitedRoles++
		}
		if role.Remaining < last {
			report.DebitedThisWindow++
			report.SecondsThisWindow += last - role.Remaining
		}
		if role.Report.Kills > 0 {
			l.fought[role.ID] = true
		}
		if role.Report.StopReason == 2 {
			report.StoppedByDeath++
		}
		l.previous[role.ID] = role.Remaining
	}
	report.ShownOfflineKills = len(l.fought)
	l.latest = report
	return report, nil
}
