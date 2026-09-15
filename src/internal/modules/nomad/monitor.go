package nomad

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"antelope/internal/modules/log"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	leaderLockKey    = "nomad-monitor:leader"
	leaderLockTTL    = 30 * time.Second
	renewInterval    = 10 * time.Second
	electionInterval = 5 * time.Second
)

const (
	renewScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("EXPIRE", KEYS[1], ARGV[2])
else
    return 0
end`

	releaseScript = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end`
)

// ---------------------------------------------------------------------------
// EventMonitor
// ---------------------------------------------------------------------------

// EventMonitor manages leader election via Redis and dispatches Nomad events
// to a pluggable EventHandler. It contains no business logic and imports no
// business-layer models.
type EventMonitor struct {
	nomadClient *nomad.Client
	db          *gorm.DB
	redisClient redis.UniversalClient
	handler     EventHandler

	// 1 = leader, 0 = follower — always accessed via atomic ops. This is the
	// single source of truth for leadership; leaderChangedCh only nudges the
	// renewal loop to re-read it.
	isLeader atomic.Int32

	// leaderChangedCh is a 1-buffered notification channel: a non-blocking
	// send wakes the single long-lived runRenewalLoop to reconcile its renewal
	// ticker with the current isLeader value. It carries no payload on purpose
	// — the loop always re-reads isLeader — so a coalesced or dropped
	// notification is harmless.
	leaderChangedCh chan struct{}

	nodeID string
	ctx    context.Context
	cancel context.CancelFunc

	// mu serializes state transitions (e.g. concurrent calls to
	// releaseLeadership from the renewal loop and Shutdown).
	mu sync.Mutex
}

// NewEventMonitor constructs an EventMonitor and ensures the infrastructure
// DB schema is up to date before returning.
//
// Migration is protected by a Redis distributed lock so that only one instance
// executes DDL when multiple nodes start concurrently.
func NewEventMonitor(
	nomadClient *nomad.Client,
	db *gorm.DB,
	redisClient redis.UniversalClient,
	handler EventHandler,
) (*EventMonitor, error) {
	if handler == nil {
		return nil, errors.New("nomad.NewEventMonitor: handler must not be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	nodeID := generateNodeID()

	// Run infrastructure schema migration with distributed coordination.
	if err := runMigration(ctx, db, redisClient); err != nil {
		cancel()
		return nil, fmt.Errorf("nomad.NewEventMonitor: schema migration: %w", err)
	}

	return &EventMonitor{
		nomadClient:     nomadClient,
		db:              db,
		redisClient:     redisClient,
		handler:         handler,
		nodeID:          nodeID,
		ctx:             ctx,
		cancel:          cancel,
		leaderChangedCh: make(chan struct{}, 1),
	}, nil
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// StartWithHA starts the leader-election loop and the event-processing loop.
// It blocks until Shutdown is called.
func (em *EventMonitor) StartWithHA() error {
	log.L().Info("starting event monitor", zap.String("node_id", em.nodeID))

	go em.runElectionLoop()
	// A single long-lived renewal loop owns the renew ticker for the whole
	// process lifetime; it renews only while isLeader == 1, driven by
	// leaderChangedCh. Starting it exactly once here (rather than spawning one
	// per acquisition) makes the goroutine's lifecycle obvious and removes the
	// spawn-on-acquire timing fragility.
	go em.runRenewalLoop()

	for {
		select {
		case <-em.ctx.Done():
			return nil
		default:
			if em.isLeader.Load() == 1 {
				log.L().Info("node is leader, watching events", zap.String("node_id", em.nodeID))
				if err := em.watchEvents(); err != nil {
					log.L().Error("watchEvents error; releasing leadership",
						zap.String("node_id", em.nodeID), zap.Error(err))
					em.releaseLeadership()
				}
			} else {
				log.L().Info("node is follower, waiting", zap.String("node_id", em.nodeID))
				select {
				case <-em.ctx.Done():
					return nil
				case <-time.After(renewInterval):
				}
			}
		}
	}
}

// Shutdown stops all background goroutines and releases the leadership lock.
func (em *EventMonitor) Shutdown() {
	em.cancel()
	em.releaseLeadership()
}

// ---------------------------------------------------------------------------
// Leader Election
// ---------------------------------------------------------------------------

func (em *EventMonitor) runElectionLoop() {
	ticker := time.NewTicker(electionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-em.ctx.Done():
			return
		case <-ticker.C:
			if em.isLeader.Load() == 0 {
				em.tryAcquireLeadership()
			}
			// Renewal is handled exclusively by runRenewalLoop to avoid
			// double-renewal between this goroutine and the renewal goroutine.
		}
	}
}

func (em *EventMonitor) tryAcquireLeadership() {
	acquired, err := em.redisClient.SetNX(em.ctx, leaderLockKey, em.nodeID, leaderLockTTL).Result()
	if err != nil {
		log.L().Warn("redis SetNX error during election", zap.Error(err))
		return
	}
	if !acquired {
		return
	}

	em.isLeader.Store(1)
	log.L().Info("node became leader", zap.String("node_id", em.nodeID))

	em.signalLeadershipChange()
}

// signalLeadershipChange wakes runRenewalLoop to reconcile its ticker with the
// current isLeader value. Non-blocking: with a 1-slot buffer a pending
// notification already guarantees the loop will re-read the latest state, so a
// dropped send is harmless. Safe to call while holding em.mu.
func (em *EventMonitor) signalLeadershipChange() {
	select {
	case em.leaderChangedCh <- struct{}{}:
	default:
	}
}

// runRenewalLoop is the single, long-lived goroutine that renews the leader
// lease. It exists for the whole process lifetime (started once by StartWithHA)
// and renews only while this node is leader. The pure renewalGate decides when
// the renew ticker should run (driven by isLeader + leaderChangedCh); this loop
// owns the real ticker and the Redis renewal call and applies the gate's
// actions. The gate's edge-triggered logic — unit-tested in renewal_gate_test —
// guarantees frequent "still leader" notifications can't Reset the ticker and
// push the next renewal past the lease TTL.
func (em *EventMonitor) runRenewalLoop() {
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	ticker.Stop() // start idle; renew only once we become leader
	gate := renewalGate{}

	apply := func(a tickerAction) {
		switch a {
		case tickerStart:
			ticker.Reset(renewInterval)
		case tickerStop:
			ticker.Stop()
		}
	}

	for {
		select {
		case <-em.ctx.Done():
			return

		case <-em.leaderChangedCh:
			apply(gate.reconcile(em.isLeader.Load() == 1))

		case <-ticker.C:
			// A tick can still arrive just after we stopped the ticker; re-read
			// the source of truth before renewing.
			if em.isLeader.Load() == 0 {
				gate.onRenewFailed()
				ticker.Stop()
				continue
			}
			if !em.renewLeadership() {
				// renewLeadership already called releaseLeadership (which flips
				// isLeader and signals); stop locally too so we don't fire again
				// before the notification is processed.
				gate.onRenewFailed()
				ticker.Stop()
			}
		}
	}
}

func (em *EventMonitor) renewLeadership() bool {
	result, err := em.redisClient.Eval(
		em.ctx,
		renewScript,
		[]string{leaderLockKey},
		em.nodeID,
		int(leaderLockTTL.Seconds()),
	).Result()
	if err != nil {
		log.L().Error("leadership renewal error", zap.Error(err))
		em.releaseLeadership()
		return false
	}

	if result.(int64) != 1 {
		log.L().Warn("leadership lost (lock owned by another node)",
			zap.String("node_id", em.nodeID))
		em.releaseLeadership()
		return false
	}

	return true
}

// confirmLeadership verifies via Redis that this node still owns the leader lock
// right now. A negative result means the lease was lost (e.g. it expired while
// this process was stalled by a GC pause or partitioned from Redis) and another
// node may have taken over; processing must stop immediately. On a transient
// Redis error it trusts the cached flag rather than dropping leadership on a blip
// — the renewal loop remains the backstop.
//
// This is defense-in-depth: the authoritative guard against double-applying an
// event is the idempotency log (infraProcessedEvent.event_id uniqueIndex, see
// handleEventSafely), which makes a duplicate mutation impossible even under a
// brief split-brain. confirmLeadership merely shrinks the window during which a
// deposed leader does wasted, duplicate work.
func (em *EventMonitor) confirmLeadership() bool {
	ctx, cancel := context.WithTimeout(em.ctx, 3*time.Second)
	defer cancel()
	owner, err := em.redisClient.Get(ctx, leaderLockKey).Result()
	if err != nil {
		return em.isLeader.Load() == 1
	}
	return owner == em.nodeID
}

func (em *EventMonitor) releaseLeadership() {
	em.mu.Lock()
	defer em.mu.Unlock()

	if em.isLeader.Swap(0) == 0 {
		return // already a follower
	}

	log.L().Info("releasing leadership", zap.String("node_id", em.nodeID))

	// Tell the renewal loop to stop renewing. Non-blocking; safe under em.mu.
	em.signalLeadershipChange()

	if _, err := em.redisClient.Eval(
		context.Background(), // fresh ctx — em.ctx may be canceled
		releaseScript,
		[]string{leaderLockKey},
		em.nodeID,
	).Result(); err != nil {
		log.L().Warn("could not delete leader key (may have already expired)", zap.Error(err))
	}
}

// ---------------------------------------------------------------------------
// Event Processing
// ---------------------------------------------------------------------------

func (em *EventMonitor) watchEvents() error {
	lastIndex := em.getLastCheckpointIndex()

	topics := em.handler.Topics()
	if len(topics) == 0 {
		log.L().Warn("handler returned empty topics; nothing to subscribe to")
		<-em.ctx.Done()
		return nil
	}

	eventsCh, err := em.nomadClient.EventStream().Stream(em.ctx, topics, lastIndex, nil)
	if err != nil {
		return fmt.Errorf("open event stream: %w", err)
	}

	log.L().Info("event stream open", zap.Uint64("start_index", lastIndex))

	for {
		select {
		case <-em.ctx.Done():
			return nil

		case batch, ok := <-eventsCh:
			if !ok {
				log.L().Info("event stream closed")
				return nil
			}
			if batch.Err != nil {
				log.L().Error("error batch received, continuing", zap.Error(batch.Err))
				continue
			}

			// Re-confirm leadership against Redis before mutating on this batch.
			// Without it a deposed leader keeps processing until its renewal loop
			// notices (up to renewInterval later); this bounds that window to a
			// single batch. Double-apply is still prevented by the idempotency
			// log regardless — see confirmLeadership / handleEventSafely.
			if !em.confirmLeadership() {
				log.L().Warn("leadership not confirmed against Redis; releasing and stopping",
					zap.String("node_id", em.nodeID))
				em.releaseLeadership()
				return nil
			}

			for i := range batch.Events {
				if em.isLeader.Load() == 0 {
					log.L().Info("lost leadership mid-batch; stopping",
						zap.String("node_id", em.nodeID))
					return nil
				}
				em.handleEventSafely(&batch.Events[i])
			}
		}
	}
}

func (em *EventMonitor) handleEventSafely(event *nomad.Event) {
	eventID := em.eventID(event)

	// 1. Idempotency check.
	processed, err := em.isEventProcessed(eventID)
	if err != nil {
		log.L().Error("idempotency check failed",
			zap.String("event_id", eventID), zap.Error(err))
		return
	}
	if processed {
		log.L().Info("event already processed, skipping", zap.String("event_id", eventID))
		return
	}

	// 2. Open transaction.
	tx := em.db.Begin()
	if tx.Error != nil {
		log.L().Error("begin transaction failed", zap.Error(tx.Error))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.L().Error("panic in handler; transaction rolled back", zap.Any("recover", r))
		}
	}()

	// 3. Delegate to business handler.
	jobID, err := em.handler.Handle(em.ctx, tx, event)
	if err != nil {
		tx.Rollback()
		log.L().Error("handler error; transaction rolled back",
			zap.String("event_id", eventID), zap.Error(err))
		return
	}

	// 4. Record idempotency marker (same transaction as business mutation).
	if err := em.markEventProcessed(tx, event, eventID, jobID); err != nil {
		tx.Rollback()
		log.L().Error("markEventProcessed failed; transaction rolled back",
			zap.String("event_id", eventID), zap.Error(err))
		return
	}

	// 5. Commit.
	if err := tx.Commit().Error; err != nil {
		log.L().Error("commit failed", zap.String("event_id", eventID), zap.Error(err))
		return
	}

	// 6. Persist checkpoint (best-effort, outside transaction).
	if err := em.saveCheckpoint(event.Index, eventID); err != nil {
		log.L().Error("saveCheckpoint failed (non-fatal)",
			zap.String("event_id", eventID), zap.Error(err))
	}

	log.L().Info("event processed successfully", zap.String("event_id", eventID))
}

// ---------------------------------------------------------------------------
// Persistence helpers — infra-layer models only, no business imports
// ---------------------------------------------------------------------------

func (em *EventMonitor) isEventProcessed(eventID string) (bool, error) {
	var count int64
	err := em.db.Model(&infraProcessedEvent{}).
		Where("event_id = ?", eventID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("isEventProcessed: %w", err)
	}
	return count > 0, nil
}

func (em *EventMonitor) markEventProcessed(tx *gorm.DB, event *nomad.Event, eventID, jobID string) error {
	record := &infraProcessedEvent{
		EventID:     eventID,
		EventIndex:  event.Index,
		EventType:   event.Type,
		JobID:       jobID,
		ProcessedAt: time.Now(),
	}
	return tx.Create(record).Error
}

func (em *EventMonitor) saveCheckpoint(index uint64, eventID string) error {
	cp := infraCheckPoint{
		ID:          1,
		LastIndex:   index,
		LastEventID: eventID,
		UpdatedAt:   time.Now(),
	}
	return em.db.Save(&cp).Error
}

func (em *EventMonitor) getLastCheckpointIndex() uint64 {
	var cp infraCheckPoint
	if err := em.db.First(&cp, 1).Error; err != nil {
		log.L().Info("no checkpoint found, starting from index 0")
		return 0
	}
	log.L().Info("resuming from checkpoint", zap.Uint64("last_index", cp.LastIndex))
	return cp.LastIndex
}

func (em *EventMonitor) eventID(event *nomad.Event) string {
	return fmt.Sprintf("%s-%d-%s", event.Topic, event.Index, event.Key)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func generateNodeID() string {
	hostname, _ := os.Hostname()
	return fmt.Sprintf("nomad-monitor:node:%s-%d", hostname, time.Now().UnixNano())
}
