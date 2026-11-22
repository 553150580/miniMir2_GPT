package main

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	loadMonsterCount               = 8
	loadAutoActivateRequest uint32 = 50001
)

type loadBot struct {
	socket                *websocket.Conn
	ctx                   context.Context
	cancel                context.CancelFunc
	binding, scene, mapID uint64
	handle                uint32
	x, y                  int32

	intentional, auto, dead                              atomic.Bool
	attack, cast, settlement, death, errors, heartbeatOK atomic.Uint64
	baselines, positionSamples, changedPosition          atomic.Uint64
	heartbeatRequest                                     atomic.Uint32
	autoActivation                                       chan error

	statusMu    sync.Mutex
	closeReason string
	errorCodes  map[uint32]uint64
}

type loadBotSnapshot struct {
	Online, Ready, Auto, Dead                                        bool
	Attack, Cast, Settlement, Death, Errors, CloseCount, HeartbeatOK uint64
	Baselines, PositionSamples, ChangedPosition                      uint64
	ZoneWidth, ZoneHeight                                            int32
	Scene, MapID                                                     uint64
	CloseReason                                                      string
	ErrorCodes                                                       map[uint32]uint64
}

// ctx覆盖整个压测生命周期，入场另设与客户端一致的十秒期限。
func joinLoadBot(ctx context.Context, address, ticket string, activate bool) (*loadBot, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, fmt.Errorf("压测客户端仅连接隔离的数字回环地址：%q", address)
	}
	admission, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	socket, response, err := websocket.Dial(admission, "ws://"+address+"/ws", nil)
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("WebSocket入场HTTP %d：%w", response.StatusCode, err)
		}
		return nil, fmt.Errorf("建立WebSocket：%w", err)
	}
	socket.SetReadLimit(wire.MaxFrame)
	run, cancel := context.WithCancel(ctx)
	b := &loadBot{socket: socket, ctx: run, cancel: cancel, autoActivation: make(chan error, 1)}
	if err := b.enter(admission, ticket, activate); err != nil {
		b.close()
		return nil, err
	}
	go b.drain()
	go b.heartbeats()
	return b, nil
}

func (b *loadBot) enter(ctx context.Context, ticket string, activate bool) error {
	if err := b.send(ctx, p.MsgC2SEnterLine, 1, &pb.EnterLineRequest{AccountTicket: ticket}); err != nil {
		return err
	}
	var entry pb.EnterLineResult
	f, err := b.await(ctx, p.MsgS2CEnterLine, 1, &entry)
	if err != nil {
		return err
	}
	if f.Binding == 0 || entry.SceneKind != devconfig.SceneKindPersonal || entry.RoomId != 1 ||
		entry.ResourceVersion != 26 || entry.SceneEpoch == 0 || entry.BaselineId == 0 || entry.PlayerHandle == 0 || entry.Position == nil {
		return fmt.Errorf("入场未返回规则26的个人地图和角色")
	}
	b.binding, b.scene, b.mapID, b.handle = f.Binding, entry.SceneEpoch, entry.RoomId, entry.PlayerHandle
	b.x, b.y = entry.Position.X, entry.Position.Y
	var chunks uint32
	var seen uint64
	player := false
	for matched := uint32(0); chunks == 0 || matched < chunks; matched++ {
		var baseline pb.SceneBaselineChunk
		if _, err := b.await(ctx, p.MsgS2CSceneBaseline, 0, &baseline); err != nil {
			return err
		}
		if baseline.SceneEpoch != b.scene || baseline.BaselineId != entry.BaselineId ||
			baseline.ChunkCount == 0 || baseline.ChunkCount > 64 || baseline.ChunkIndex >= baseline.ChunkCount ||
			(chunks != 0 && baseline.ChunkCount != chunks) || seen&(uint64(1)<<baseline.ChunkIndex) != 0 {
			return fmt.Errorf("个人地图首基线分块与入场回执不匹配")
		}
		seen |= uint64(1) << baseline.ChunkIndex
		chunks = baseline.ChunkCount
		for _, entity := range baseline.Entities {
			if entity.EntityHandle == b.handle && entity.EntityKind == 1 && entity.Hp > 0 && entity.Position != nil {
				player = entity.Position.X == b.x && entity.Position.Y == b.y
			}
		}
	}
	if !player || b.x < 0 || b.x >= 13 || b.y < 0 || b.y >= 13 {
		return fmt.Errorf("个人地图首基线没有匹配的在场角色")
	}
	if err := b.send(ctx, p.MsgC2SEnterWorldAck, 2, &pb.EnterWorldAck{SceneEpoch: b.scene, BaselineId: entry.BaselineId}); err != nil {
		return err
	}
	var ready pb.SyncReady
	if _, err := b.await(ctx, p.MsgS2CSyncReady, 2, &ready); err != nil {
		return err
	}
	if ready.SceneEpoch != b.scene || ready.BaselineId != entry.BaselineId {
		return fmt.Errorf("场景就绪回执与入场不匹配")
	}
	monsterCount := uint32(1)
	if activate {
		monsterCount = loadMonsterCount
	}
	if err := b.send(ctx, p.MsgC2SAutoBattle, 3, &pb.AutoBattleIntent{SceneEpoch: b.scene, Enabled: activate, MonsterCount: monsterCount}); err != nil {
		return err
	}
	var hosting pb.HostingState
	if _, err := b.await(ctx, p.MsgS2CHostingState, 3, &hosting); err != nil {
		return err
	}
	if hosting.AutoBattleEnabled != activate {
		return fmt.Errorf("服务端未确认预期的入场自动战斗状态")
	}
	b.auto.Store(hosting.AutoBattleEnabled)
	return nil
}

// 已连接的角色使用正常在线协议切换，仍由唯一的读取协程处理回执和心跳。
func (b *loadBot) enableAuto(ctx context.Context) error {
	if b.auto.Load() {
		return nil
	}
	ctx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if err := b.send(ctx, p.MsgC2SAutoBattle, loadAutoActivateRequest, &pb.AutoBattleIntent{
		SceneEpoch: b.scene, Enabled: true, MonsterCount: loadMonsterCount,
	}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("等待自动战斗回执：%w", ctx.Err())
	case <-b.ctx.Done():
		return fmt.Errorf("自动战斗确认前连接断开")
	case err := <-b.autoActivation:
		return err
	}
}

func (b *loadBot) receiveAutoActivation(err error) {
	select {
	case b.autoActivation <- err:
	default:
	}
}

func (b *loadBot) send(ctx context.Context, id uint16, request uint32, q proto.Message) error {
	msg := wire.Encode(wire.Message(id, request, b.binding, q))
	if err := b.socket.Write(ctx, websocket.MessageBinary, msg); err != nil {
		return fmt.Errorf("发送游戏协议 %d：%w", id, err)
	}
	return nil
}

func (b *loadBot) await(ctx context.Context, id uint16, request uint32, out proto.Message) (wire.Frame, error) {
	for {
		f, err := b.read(ctx)
		if err != nil {
			return f, fmt.Errorf("等待游戏协议 %d：%w", id, err)
		}
		if f.Error != 0 {
			return f, fmt.Errorf("入场期间协议 %d 请求 %d 返回错误码 %d", f.ID, f.Request, f.Error)
		}
		if f.ID == id && f.Request == request {
			if err := proto.Unmarshal(f.Payload, out); err != nil {
				return f, fmt.Errorf("解码游戏协议 %d：%w", id, err)
			}
			return f, nil
		}
	}
}

func (b *loadBot) read(ctx context.Context) (wire.Frame, error) {
	kind, raw, err := b.socket.Read(ctx)
	if err != nil {
		return wire.Frame{}, err
	}
	if kind != websocket.MessageBinary {
		return wire.Frame{}, fmt.Errorf("收到非二进制游戏协议")
	}
	return wire.Decode(raw)
}

func (b *loadBot) drain() {
	defer b.socket.CloseNow()
	defer b.cancel()
	for {
		f, err := b.read(b.ctx)
		if err != nil {
			b.failed(fmt.Errorf("读取WebSocket：%w", err))
			return
		}
		if f.Error != 0 {
			b.recordError(f.Error)
			if f.Request == loadAutoActivateRequest && f.ID == p.MsgC2SAutoBattle {
				b.receiveAutoActivation(fmt.Errorf("开启自动战斗返回错误码 %d", f.Error))
			}
			continue
		}
		var decodeErr error
		switch f.ID {
		case p.MsgS2CAttackAccepted:
			var attack pb.AttackAccepted
			decodeErr = proto.Unmarshal(f.Payload, &attack)
			if decodeErr == nil && attack.CasterHandle == b.handle {
				b.attack.Add(1)
			}
		case p.MsgS2CCastAccepted:
			var cast pb.CastAccepted
			decodeErr = proto.Unmarshal(f.Payload, &cast)
			if decodeErr == nil && cast.CasterHandle == b.handle {
				b.cast.Add(1)
			}
		case p.MsgS2CExperienceGrantResult:
			var reward pb.ExperienceGrantResult
			decodeErr = proto.Unmarshal(f.Payload, &reward)
			if decodeErr == nil && reward.RawExperience > 0 {
				b.settlement.Add(1)
			}
		case p.MsgS2CDeathNotice:
			b.death.Add(1)
			b.dead.Store(true)
		case p.MsgS2CHostingState:
			var hosting pb.HostingState
			decodeErr = proto.Unmarshal(f.Payload, &hosting)
			if decodeErr == nil {
				b.auto.Store(hosting.AutoBattleEnabled)
			}
			if f.Request == loadAutoActivateRequest {
				activationErr := decodeErr
				if activationErr == nil && !hosting.AutoBattleEnabled {
					activationErr = fmt.Errorf("服务端未确认开启自动战斗")
				}
				b.receiveAutoActivation(activationErr)
			}
		case p.MsgS2COperationResult:
			var result pb.OperationResult
			decodeErr = proto.Unmarshal(f.Payload, &result)
			if decodeErr == nil && result.ErrorCode != 0 {
				b.recordError(result.ErrorCode)
			}
		case p.MsgS2CHeartbeat:
			if f.Request > b.heartbeatRequest.Load() {
				b.heartbeatRequest.Store(f.Request)
				b.heartbeatOK.Add(1)
			}
		case p.MsgS2CSceneBaseline:
			// 所有帧都从socket及时读取，定期核对自己的位置，减少压测器与被测服务争用CPU。
			if b.baselines.Add(1)%16 == 1 {
				var baseline pb.SceneBaselineChunk
				decodeErr = proto.Unmarshal(f.Payload, &baseline)
				for _, entity := range baseline.Entities {
					if entity.EntityHandle != b.handle || entity.Position == nil {
						continue
					}
					b.positionSamples.Add(1)
					if baseline.SceneEpoch != b.scene || entity.Position.X != b.x || entity.Position.Y != b.y {
						b.changedPosition.Add(1)
					}
					b.dead.Store(entity.Hp <= 0)
					break
				}
			}
		}
		if decodeErr != nil {
			b.recordError(0)
		}
	}
}

func (b *loadBot) heartbeats() {
	const interval = 10 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	started := time.Now()
	for request := uint32(4); ; request++ {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
		}
		if request > 4 && b.heartbeatRequest.Load() != request-1 {
			b.failed(fmt.Errorf("游戏心跳确认超时"))
			b.cancel()
			return
		}
		ctx, stop := context.WithTimeout(b.ctx, interval)
		err := b.send(ctx, p.MsgC2SHeartbeat, request, &pb.Heartbeat{ClientMonotonicMs: uint64(time.Since(started).Milliseconds())})
		stop()
		if err != nil {
			b.failed(err)
			b.cancel()
			return
		}
	}
}

func (b *loadBot) failed(err error) {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	if b.closeReason == "" && !b.intentional.Load() && b.ctx.Err() == nil {
		b.closeReason = err.Error()
	}
}

func (b *loadBot) recordError(code uint32) {
	b.errors.Add(1)
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	if b.errorCodes == nil {
		b.errorCodes = make(map[uint32]uint64)
	}
	b.errorCodes[code]++
}

func (b *loadBot) close() {
	b.intentional.Store(true)
	b.cancel()
	_ = b.socket.CloseNow()
}

func (b *loadBot) snapshot() loadBotSnapshot {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()
	s := loadBotSnapshot{
		Online: b.ctx.Err() == nil && b.closeReason == "", Ready: true, Auto: b.auto.Load(), Dead: b.dead.Load(),
		Attack: b.attack.Load(), Cast: b.cast.Load(), Settlement: b.settlement.Load(), Death: b.death.Load(),
		Errors: b.errors.Load(), HeartbeatOK: b.heartbeatOK.Load(), CloseReason: b.closeReason,
		Baselines: b.baselines.Load(), PositionSamples: b.positionSamples.Load(), ChangedPosition: b.changedPosition.Load(),
		Scene: b.scene, MapID: b.mapID, ZoneWidth: 13, ZoneHeight: 13,
	}
	if b.closeReason != "" {
		s.CloseCount = 1
	}
	if len(b.errorCodes) > 0 {
		s.ErrorCodes = make(map[uint32]uint64, len(b.errorCodes))
		for code, count := range b.errorCodes {
			s.ErrorCodes[code] = count
		}
	}
	return s
}
