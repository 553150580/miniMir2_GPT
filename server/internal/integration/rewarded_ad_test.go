// 本文件在专属 MongoDB 和实际账号 RPC、游戏 WebSocket 上核对原广告到账。
package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/rewardedad"
	"minimir/internal/transport"
	"minimir/internal/wire"
)

// rewardedAdTestProvider 只编译进测试。两个随机密钥分别验证模拟渠道证明和隔离设备摘要。
type rewardedAdTestProvider struct {
	key []byte // 本次隔离用例生成的渠道认证密钥。
}

func (provider rewardedAdTestProvider) signature(raw []byte) string {
	mac := hmac.New(sha256.New, provider.key)
	mac.Write(raw)
	return "Proof " + hex.EncodeToString(mac.Sum(nil))
}

func (provider rewardedAdTestProvider) VerifyCompletion(_ context.Context, auth string, notice []byte) (rewardedad.Completion, error) {
	var completion rewardedad.Completion
	if !hmac.Equal([]byte(auth), []byte(provider.signature(notice))) || json.Unmarshal(notice, &completion) != nil {
		return rewardedad.Completion{}, rewardedad.ErrProof
	}
	return completion, nil
}

// rewardedAdTrial 共享实际 MongoDB 身份，只在临时账号端点显式注入模拟渠道。
type rewardedAdTrial struct {
	ctx      context.Context        // 隔离节点的生命周期。
	store    *persistence.Store     // 当前隔离数据库。
	account  string                 // 带可信渠道能力的临时账号节点。
	plain    string                 // 未装配渠道的真实账号节点。
	routes   [2]directoryTestEntry  // 同一游戏节点承载的两个独立运行服。
	provider rewardedAdTestProvider // 测试渠道认证实施。
	verifier *rewardedad.Verifier   // 使用正式完播校验逻辑。
}

func startRewardedAdTrial(t *testing.T) rewardedAdTrial {
	t.Helper()
	var second directoryTestEntry
	ctx, store, plain, first := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		path := os.Getenv("MINIMIR_REWARDED_AD_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var fixture devconfig.Fixture
		must(t, json.Unmarshal(raw, &fixture))
		must(t, fixture.Validate())
		if fixture.RewardedAd == nil || fixture.RewardedAd.Seconds != 3600 || fixture.RewardedAd.AccountCap != 2 ||
			fixture.RewardedAd.DeviceCap != 2 || fixture.RewardedAd.NetworkCap != 3 {
			t.Fatal("隔离规则缺少约定的广告奖励或配额")
		}
		fixture.Hash = persistence.Hash(raw)
		cfg.Fixture, cfg.FixtureFile = fixture, path
		second = makeDirectoryEntry(2, 2, *cfg, "boss-death-test", "隔离首领", "二服", 0, 1)
		cfg.OriginRoutes = append(cfg.OriginRoutes, second.origin)
		cfg.RunServers = append(cfg.RunServers, second.assignment)
		cfg.RunServerIDs = append(cfg.RunServerIDs, 2)
	})
	keys := make([]byte, 64)
	_, err := rand.Read(keys)
	must(t, err)
	provider := rewardedAdTestProvider{key: keys[:32]}
	verifier, err := rewardedad.NewVerifier(store.Cfg.Fixture.RewardedAd.Provider, keys[32:], provider)
	must(t, err)
	account := httptest.NewServer((&transport.Server{Store: store, Mode: "account", AdVerifier: verifier}).Handler())
	t.Cleanup(account.Close)
	return rewardedAdTrial{ctx, store, account.URL, plain, [2]directoryTestEntry{first, second}, provider, verifier}
}

// rewardedAdPlayer 仅保留正常注册、选服和建角得到的原授权。
type rewardedAdPlayer struct {
	login *pb.LoginResult // 正常登录得到的原账号凭据。
	role  *pb.RoleSummary // 所选永久入口与原角色。
}

func (trial rewardedAdTrial) player(t *testing.T, route int) rewardedAdPlayer {
	t.Helper()
	login := &pb.LoginResult{}
	rpc(t, trial.rpcURL(), "", p.MsgC2SLocalRegister,
		&pb.LocalLoginRequest{Account: "ad_" + domain.ID()[:10], Password: "video_test_8523"}, login, 0)
	return rewardedAdPlayer{login, createDirectoryRole(t, trial.rpcURL(), login.AccountToken, trial.routes[route].origin.OriginID, "托管广告")}
}

func (trial rewardedAdTrial) rpcURL() string { return trial.account + "/rpc" }

func (trial rewardedAdTrial) query(t *testing.T, user rewardedAdPlayer, start, id []byte, code uint32) *pb.RewardedAdState {
	t.Helper()
	state := &pb.RewardedAdState{}
	rpc(t, trial.rpcURL(), user.login.AccountToken, p.MsgC2SRewardedAd, &pb.RewardedAdRequest{
		Role: user.role.Key, RequestId: start, AttemptId: id,
	}, state, code)
	return state
}

func (trial rewardedAdTrial) attach(t *testing.T, user rewardedAdPlayer, index int) *client {
	t.Helper()
	ticket := directoryTicket(t, trial.rpcURL(), user.login.AccountToken, user.role.Key, trial.routes[index])
	client := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	t.Cleanup(client.close)
	client.auto(false, 1)
	return client
}

func (trial rewardedAdTrial) role(t *testing.T, user rewardedAdPlayer) domain.Role {
	t.Helper()
	role, err := trial.store.Role(trial.ctx, domain.Hex(user.role.Key.RoleId))
	must(t, err)
	return role
}

// adCompletion 使用观看接口返回的原条款构造测试渠道的可信完播事实。
func adCompletion(state *pb.RewardedAdState, device, ip, event string) rewardedad.Completion {
	return rewardedad.Completion{AttemptID: domain.Hex(state.AttemptId), Placement: state.Placement,
		Beneficiary: state.Beneficiary, EventID: event, DeviceID: device, ClientIP: netip.MustParseAddr(ip),
		CompletedAt: time.Now().UTC().Truncate(time.Millisecond), Completed: true}
}

// postProofOrigin 模拟渠道服务器发送回调。转发头刻意与签名事实不同。
func postProofOrigin(ctx context.Context, account, id, auth string, data []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, account+"/internal/rewarded-ad/"+id, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("X-Forwarded-For", "192.0.2.254")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, err = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, err
}

func (trial rewardedAdTrial) callback(t *testing.T, completion rewardedad.Completion, signed bool, expected int) {
	t.Helper()
	raw, err := json.Marshal(completion)
	must(t, err)
	var signature string
	if signed {
		signature = trial.provider.signature(raw)
	}
	status, err := postProofOrigin(trial.ctx, trial.account, completion.AttemptID, signature, raw)
	must(t, err)
	if status != expected {
		t.Fatalf("渠道回调返回 %d，预期 %d", status, expected)
	}
}

func (trial rewardedAdTrial) result(t *testing.T, player rewardedAdPlayer, id []byte, stage, code uint32) *pb.RewardedAdState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		state := trial.query(t, player, nil, id, 0)
		if state.Stage == stage && state.ResultCode == code {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("原广告阶段为 %d，状态码为 %d，预期 %d / %d", state.Stage, state.ResultCode, stage, code)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestRewardedAdMongoWS 验证在线同步、换服配额、离线到账和回执重读。
func TestRewardedAdMongoWS(t *testing.T) {
	trial := startRewardedAdTrial(t)
	first := trial.player(t, 0)
	second := rewardedAdPlayer{first.login, createDirectoryRole(t, trial.rpcURL(), first.login.AccountToken,
		trial.routes[1].origin.OriginID, "广告二服")}
	before := trial.role(t, first)
	const starting = 24*60*60 + 300 // 从超过金币购买上限的实际余额开始。
	_, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, before.ID, bson.M{"$set": bson.M{"hosting_seconds": starting}})
	must(t, err)
	online := trial.attach(t, first, 0)
	before = trial.role(t, first)
	state := &pb.RewardedAdState{}
	rpc(t, trial.plain, first.login.AccountToken, p.MsgC2SRewardedAd, &pb.RewardedAdRequest{Role: first.role.Key}, state, 0)
	if state.ChannelAvailable || state.Stage != 0 {
		t.Fatal("未装配渠道的账号节点呈现可观看广告", state)
	}
	rpc(t, trial.plain, first.login.AccountToken, p.MsgC2SRewardedAd,
		&pb.RewardedAdRequest{Role: first.role.Key, RequestId: domain.Bytes(domain.ID())}, state, p.CodeRewardedAdUnavailable)
	request := domain.Bytes(domain.ID())
	attempt := trial.query(t, first, request, nil, 0)
	if !attempt.ChannelAvailable || attempt.AccountRemaining != 2 || attempt.Stage != persistence.RewardedAdAwaiting ||
		!proto.Equal(attempt, trial.query(t, first, request, nil, 0)) ||
		!proto.Equal(attempt, trial.query(t, first, domain.Bytes(domain.ID()), nil, 0)) {
		t.Fatal("重试没有沿用原观看", attempt)
	}
	if !trial.query(t, second, nil, nil, 0).OtherRolePending {
		t.Fatal("切场到另一运行服时未提示原角色有在途广告")
	}
	trial.query(t, second, domain.Bytes(domain.ID()), nil, p.CodeRewardedAdPending)
	trial.query(t, second, nil, attempt.AttemptId, p.CodeUnauthenticated)
	other := trial.player(t, 0)
	foreign := rewardedAdPlayer{other.login, first.role}
	trial.query(t, foreign, nil, nil, p.CodeUnauthenticated)
	completion := adCompletion(attempt, "online-device", "192.0.2.1", domain.ID())
	trial.callback(t, completion, false, http.StatusUnauthorized)
	wrong := completion
	wrong.Beneficiary = domain.ID()
	trial.callback(t, wrong, true, http.StatusUnauthorized)
	wrong = completion
	wrong.Completed = false
	trial.callback(t, wrong, true, http.StatusUnauthorized)
	if state := trial.query(t, first, nil, attempt.AttemptId, 0); state.Stage != persistence.RewardedAdAwaiting || state.AccountRemaining != 2 {
		t.Fatal("未验签或未完播的通知改变了资格", state)
	}
	raw, err := json.Marshal(completion)
	must(t, err)
	callbacks := make(chan error, 2)
	for range 2 {
		go func() {
			status, err := postProofOrigin(trial.ctx, trial.account, completion.AttemptID, trial.provider.signature(raw), raw)
			if err == nil && status != http.StatusOK {
				err = persistence.Fail(uint32(status), "并发原证明未返回成功")
			}
			callbacks <- err
		}()
	}
	for range 2 {
		must(t, <-callbacks)
	}
	travelUntilWithin(online, 10*time.Second, func(frame wire.Frame) bool {
		return frame.ID == p.MsgS2CHostingState && online.hosting.RemainHostingSec == starting+3600
	})
	receipt := trial.result(t, first, attempt.AttemptId, persistence.RewardedAdGranted, 0)
	firstAfter := trial.role(t, first)
	if receipt.Receipt == nil || receipt.Receipt.GrantedSeconds != 3600 || receipt.Receipt.BalanceSeconds != starting+3600 ||
		receipt.AccountRemaining != 1 || firstAfter.HostingSeconds != starting+3600 ||
		firstAfter.Revision != before.Revision+1 || firstAfter.Wallet != before.Wallet ||
		firstAfter.EconomicSeq != before.EconomicSeq || firstAfter.SourceSeq != before.SourceSeq ||
		firstAfter.ClaimDay != before.ClaimDay || firstAfter.ClaimedSeconds != before.ClaimedSeconds {
		t.Fatal("原角色到账、修订、来源或运营日状态不符", firstAfter, receipt)
	}
	next := trial.query(t, second, domain.Bytes(domain.ID()), nil, 0)
	secondBefore := trial.role(t, second)
	online.close()
	trial.callback(t, adCompletion(next, "offline-device", "192.0.2.2", completion.EventID), true, http.StatusConflict)
	trial.callback(t, adCompletion(next, "offline-device", "192.0.2.2", domain.ID()), true, http.StatusOK)
	offline := trial.result(t, second, next.AttemptId, persistence.RewardedAdGranted, 0)
	secondAfter := trial.role(t, second)
	if secondAfter.HostingSeconds != secondBefore.HostingSeconds+3600 || secondAfter.Wallet != secondBefore.Wallet ||
		offline.AccountRemaining != 0 || secondAfter.Owner != secondBefore.Owner || secondAfter.SessionEpoch != secondBefore.SessionEpoch {
		t.Fatal("离线角色未正确到账或被隐式接管", offline, secondAfter)
	}
	trial.query(t, first, domain.Bytes(domain.ID()), nil, p.CodeRewardedAdAccountLimit)
	rejoined := trial.attach(t, first, 0)
	if rejoined.binding == online.binding || rejoined.hosting.GetRemainHostingSec() != starting+3600 {
		t.Fatal("重连原角色没有读到已持久化余额", rejoined.hosting)
	}
	if resumed := trial.attach(t, second, 1); resumed.hosting.GetRemainHostingSec() != uint32(secondAfter.HostingSeconds) {
		t.Fatal("二服的原离线角色进场余额有误", resumed.hosting)
	}
	trial.callback(t, completion, true, http.StatusOK)
	if state := trial.query(t, first, nil, attempt.AttemptId, 0); !proto.Equal(state.Receipt, receipt.Receipt) {
		t.Fatal("重复回调或重新入场改变了原回执", state)
	}
	for _, collection := range []string{"operations", "audit_outbox"} {
		count, err := trial.store.DB.Collection(collection).CountDocuments(trial.ctx, bson.M{"kind": "rewarded_ad_grant"})
		must(t, err)
		if count != 2 {
			t.Fatalf("%s 内有 %d 条广告到账记录，预期两笔", collection, count)
		}
	}
	t.Log("原账号跨两个运行服共用日限额，在线原余额超过 24 小时后仍增加 3600 秒，冷角色到账后按原身份入场，并发回调只写一笔")
}

// TestRewardedAdJointQuotaAndOperatingDayMongo 验证不同账号共享可信设备或 IP 以及跨零点的原归属。
func TestRewardedAdJointQuotaAndOperatingDayMongo(t *testing.T) {
	trial := startRewardedAdTrial(t)
	sharedIP := "198.51.100.10"
	grant := func(device, ip string, code uint32) rewardedAdPlayer {
		t.Helper()
		player := trial.player(t, 0)
		attempt := trial.query(t, player, domain.Bytes(domain.ID()), nil, 0)
		proof := adCompletion(attempt, device, ip, domain.ID())
		trial.callback(t, proof, true, http.StatusOK)
		if code == 0 {
			trial.result(t, player, attempt.AttemptId, persistence.RewardedAdGranted, 0)
		} else {
			state := trial.result(t, player, attempt.AttemptId, persistence.RewardedAdDenied, code)
			trial.callback(t, proof, true, http.StatusOK)
			if state.AccountRemaining != 2 || state.Receipt != nil || trial.role(t, player).HostingSeconds != 0 {
				t.Fatal("超额回调消耗未触及的账号额度或发了托管秒数", state)
			}
		}
		return player
	}
	grant("shared-device", sharedIP, 0)
	grant("shared-device", "::ffff:"+sharedIP, 0)
	grant("shared-device", "192.0.2.50", p.CodeRewardedAdDeviceLimit)
	grant("third-device", sharedIP, 0)
	unblocked := grant("new-device-after-deny", sharedIP, p.CodeRewardedAdNetworkLimit)
	retry := trial.query(t, unblocked, domain.Bytes(domain.ID()), nil, 0)
	trial.callback(t, adCompletion(retry, "new-device-after-deny", "192.0.2.50", domain.ID()), true, http.StatusOK)
	trial.result(t, unblocked, retry.AttemptId, persistence.RewardedAdGranted, 0)

	// 在同一个真实数据库中移动测试时钟，先播放完，再跨上海零点提交可信结果。
	player := trial.player(t, 1)
	auth, err := trial.store.Authenticate(trial.ctx, player.login.AccountToken)
	must(t, err)
	zone := time.FixedZone("上海测试时区", 8*60*60)
	local := time.Now().In(zone)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	ticket, err := trial.store.StartRewardedAd(trial.ctx, auth, player.role.Key, domain.Bytes(domain.ID()), midnight.Add(-20*time.Second))
	must(t, err)
	proof := rewardedad.Completion{AttemptID: ticket.ID, Placement: ticket.Rules.Placement,
		Beneficiary: ticket.Expected().BeneficiaryReference(), EventID: domain.ID(), DeviceID: "midnight-device",
		ClientIP: netip.MustParseAddr("192.0.2.62"), CompletedAt: midnight.Add(-time.Millisecond), Completed: true}
	raw, err := json.Marshal(proof)
	must(t, err)
	afterMidnight := midnight.Add(10 * time.Second)
	verified, err := trial.verifier.Verify(trial.ctx, ticket.Expected(), trial.provider.signature(raw), raw, afterMidnight)
	must(t, err)
	expired, err := trial.store.RecordRewardedAd(trial.ctx, ticket.ID, verified, ticket.CallbackEnd)
	if persistence.Code(err) != p.CodeRewardedAdExpired || expired.ID != "" {
		t.Fatal("确认窗口结束后接受了首次回调", err)
	}
	record, err := trial.store.RecordRewardedAd(trial.ctx, ticket.ID, verified, afterMidnight)
	must(t, err)
	state := trial.result(t, player, domain.Bytes(ticket.ID), persistence.RewardedAdGranted, 0)
	previousDay := midnight.AddDate(0, 0, -1).Format(time.DateOnly)
	if record.OperatingDay != previousDay || state.Receipt.GetOperatingDay() != previousDay ||
		state.OperatingDay != midnight.Format(time.DateOnly) || state.AccountRemaining != 2 || state.Receipt.GetGrantedSeconds() != 3600 {
		t.Fatal("隔夜回调未按原完播日核算", record, state)
	}
	if replay, err := trial.store.RecordRewardedAd(trial.ctx, ticket.ID, verified, ticket.CallbackEnd.Add(time.Second)); err != nil ||
		replay.Stage != persistence.RewardedAdGranted || replay.Receipt == nil {
		t.Fatal("超过确认窗口后未保留可重查的原凭据", replay, err)
	}
	t.Log("账号未因共享 IP 受到额外封禁，IPv4 映射合计三笔成功，设备限额拒绝第三笔，跨上海零点回调沿用原完播日")
}
