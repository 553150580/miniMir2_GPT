// 本包从可信广告渠道核验完整播放事实，持久层只接收这里签出的归属和摘要。
package rewardedad

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"strconv"
	"time"

	"minimir/internal/devconfig"
)

var ErrProof = errors.New("激励视频渠道证明无效") // 对外统一的核验失败，不泄露渠道正文。

// Expected 由已保存的观看申请给出，包含该事件应绑定的服务器原条款。
type Expected struct {
	AttemptID string    // 游戏生成的原观看凭据。
	AccountID string    // 原账号，参与受益标记计算。
	RoleID    string    // 原受益角色。
	Origin    uint64    // 角色的永久入口服。
	Provider  string    // 原可信渠道标识。
	Placement string    // 原广告位标识。
	IssuedAt  time.Time // 播放凭据签发时刻。
	WatchEnd  time.Time // 原播放必须完成的截止时刻，不含端点。
}

// BeneficiaryReference 给渠道提供业务绑定标记，避免向其发送账号或角色原始编号。
func (e Expected) BeneficiaryReference() string {
	sum := sha256.Sum256([]byte("minimir:rewarded-ad:beneficiary:v1:" + e.AttemptID + ":" + e.AccountID + ":" + e.RoleID + ":" +
		strconv.FormatUint(e.Origin, 10)))
	return hex.EncodeToString(sum[:])
}

// Completion 是渠道适配器通过验签或主动查询得到的原播放事实。
type Completion struct {
	AttemptID   string     // 渠道保存的原观看凭据编号。
	Placement   string     // 渠道实际发生的广告位。
	Beneficiary string     // 渠道验证并返还的原角色绑定标记。
	EventID     string     // 此渠道内永久唯一的播放完成事件。
	DeviceID    string     // 渠道认证的稳定设备，不能取未签名的客户端字段。
	ClientIP    netip.Addr // 渠道认证的原客户端地址，不能取通知请求头。
	CompletedAt time.Time  // 可信完成时刻。
	Completed   bool       // 确认已播放完，提前关闭、错误或取消不发奖。
}

// Provider 由正式渠道的验签或主动查单实现。正文和认证头均视作待核验输入。
type Provider interface {
	VerifyCompletion(ctx context.Context, auth string, notice []byte) (Completion, error)
}

// Facts 只保存原事件标识摘要和额外配额维度，不持久化设备标识和 IP 原文。
type Facts struct {
	EventKey    string    `bson:"event_key"`    // 按渠道命名空间全库唯一。
	DeviceKey   string    `bson:"device_key"`   // 独立隐私密钥计算的设备摘要。
	NetworkKey  string    `bson:"network_key"`  // 统一 IPv4 映射表示之后的网络地址摘要。
	CompletedAt time.Time `bson:"completed_at"` // 结算使用的原广告实际完成时刻。
}

// Verified 的原始证明只能由 Verifier 生成，不能从客户端协议反序列化。
type Verified struct {
	expected Expected // 原服务端凭据及关联的角色身份。
	facts    Facts    // 渠道结果校验后保留的原事实。
}

// For 只有绑定到同一个已保存凭据的证明可以被结算。
func (v Verified) For(e Expected) (Facts, bool) {
	return v.facts, v.expected == e && v.facts.EventKey != ""
}

// Verifier 固定可信渠道和独立隐私密钥，适配器决定渠道的认证方式。
type Verifier struct {
	channel string   // 服务器装配的渠道身份。
	key     []byte   // 独立于回调验签的稳定密钥，仅用于摘要化设备、IP。
	adapter Provider // 可验证真实完成信息的可信适配器。
}

// NewVerifier 要求显式注入渠道能力和至少 32 字节的独立隐私密钥。
func NewVerifier(channel string, key []byte, adapter Provider) (*Verifier, error) {
	if !devconfig.ValidRewardedAdCode(channel) || len(key) < 32 || adapter == nil {
		return nil, ErrProof
	}
	return &Verifier{channel: channel, key: append([]byte(nil), key...), adapter: adapter}, nil
}

// ActiveFor 判断当前节点是否确实装配原渠道。
func (v *Verifier) ActiveFor(channel string) bool {
	return v != nil && v.adapter != nil && v.channel == channel
}

// Verify 只采信完整播放、时间有效且与原广告位和原角色均匹配的证明。
func (v *Verifier) Verify(ctx context.Context, expected Expected, auth string, notice []byte, now time.Time) (Verified, error) {
	if !v.ActiveFor(expected.Provider) || len(notice) == 0 || len(notice) > 16384 || len(auth) > 2048 ||
		len(expected.AttemptID) != 32 || expected.Origin == 0 || !expected.IssuedAt.Before(expected.WatchEnd) {
		return Verified{}, ErrProof
	}
	actual, err := v.adapter.VerifyCompletion(ctx, auth, notice)
	if ctx.Err() != nil {
		return Verified{}, ctx.Err()
	}
	completed := actual.CompletedAt.UTC().Truncate(time.Millisecond)
	ip := actual.ClientIP.Unmap()
	if err != nil || !actual.Completed || actual.AttemptID != expected.AttemptID || actual.Placement != expected.Placement ||
		actual.Beneficiary != expected.BeneficiaryReference() || !validIdentity(actual.EventID) || !validIdentity(actual.DeviceID) ||
		!ip.IsValid() || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() ||
		completed.Before(expected.IssuedAt) || !completed.Before(expected.WatchEnd) || completed.After(now) {
		return Verified{}, ErrProof
	}
	event := sha256.Sum256([]byte("minimir:rewarded-ad:event:v1:" + v.channel + ":" + actual.EventID))
	return Verified{expected: expected, facts: Facts{EventKey: hex.EncodeToString(event[:]), DeviceKey: v.subject("device", actual.DeviceID),
		NetworkKey: v.subject("ip", ip.String()), CompletedAt: completed}}, nil
}

// subject 对设备和地址使用隔离的摘要命名空间。
func (v *Verifier) subject(scope, value string) string {
	mac := hmac.New(sha256.New, v.key)
	mac.Write([]byte("minimir:rewarded-ad:scope:v1:" + scope + ":" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

// validIdentity 拒绝超长、空白和控制字符，不规整渠道的原事件号。
func validIdentity(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, c := range value {
		if c <= ' ' || c > '~' {
			return false
		}
	}
	return true
}
