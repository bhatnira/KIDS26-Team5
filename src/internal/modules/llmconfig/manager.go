package llmconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"antelope/internal/modules/log"
	"antelope/pkg/secretbox"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	redisKeyPrefix = "llm:configs:"
	maxRetries     = 5
)

// UserLLMConfig represents a single user-defined LLM configuration.
type UserLLMConfig struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	Provider           string  `json:"provider"` // e.g. "openai"
	APIKey             string  `json:"api_key"`
	Model              string  `json:"model"`
	BaseURL            string  `json:"base_url,omitempty"`
	MaxTokens          int     `json:"max_tokens"`
	Temperature        float64 `json:"temperature"`
	ThinkingEnabled    bool    `json:"thinking_enabled"`
	ThinkingEffort     string  `json:"thinking_effort,omitempty"` // low|medium|high|xhigh|max
	IsDefault          bool    `json:"is_default"`
	InsecureSkipVerify bool    `json:"insecure_skip_verify"`
}

// Manager handles per-user LLM configurations stored in Redis.
// Multiple configs per user are supported; one may be marked as default.
//
// Concurrency model (two layers):
//
//  1. Per-user in-process mutex: prevents goroutines within the same pod from
//     racing to Redis simultaneously.  This keeps the common single-replica
//     case cheap — most conflicts are resolved without any Redis round-trip.
//
//  2. Redis WATCH / MULTI / EXEC (optimistic locking): detects concurrent
//     writes from *other pods* between our read and write.  If the key is
//     changed by another node before our EXEC, Redis returns TxFailedErr and
//     we retry up to maxRetries times.
//
// Read-only methods (GetConfigs, GetConfig, GetDefaultConfig) hold no lock;
// stale-read tolerance is acceptable for config lookups.
type Manager struct {
	rdb redis.UniversalClient
	db  *gorm.DB       // durable mirror / source of truth (may be nil in minimal setups)
	box *secretbox.Box // optional at-rest encryption (nil = plaintext)
	// mu stores one *sync.Mutex per userID, created lazily on first mutation.
	//
	// DESIGN (intentional, not a leak): entries are never evicted. Growth is
	// bounded by the number of distinct users who have ever mutated config on
	// this pod (not by request volume), each entry is a single ~tens-of-bytes
	// *sync.Mutex, and a process restart resets it — so even 100k users is a few
	// MB. Sweeping live mutexes would risk deleting one mid-use to save bytes;
	// not worth it. Do not "fix" this as a memory leak.
	mu sync.Map // map[uint]*sync.Mutex
}

func NewManager(rdb redis.UniversalClient, db *gorm.DB, box *secretbox.Box) *Manager {
	return &Manager{rdb: rdb, db: db, box: box}
}

// userMu returns the per-user mutex, creating it lazily via LoadOrStore.
func (m *Manager) userMu(userID uint) *sync.Mutex {
	v, _ := m.mu.LoadOrStore(userID, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// ── Read-only methods ─────────────────────────────────────────────────────────

// GetConfigs returns all LLM configs for the given user.
func (m *Manager) GetConfigs(userID uint) ([]UserLLMConfig, error) {
	ctx := context.Background()
	raw, err := m.rdb.Get(ctx, m.key(userID)).Result()
	if errors.Is(err, redis.Nil) {
		// Cache miss — fall back to the durable store and repopulate the cache.
		// This is what makes LLM configs survive a Redis flush/eviction.
		configs, found, dbErr := m.loadFromDB(userID)
		if dbErr != nil {
			return nil, dbErr
		}
		if !found {
			return []UserLLMConfig{}, nil
		}
		// Repopulate the cache, encrypting at rest when a key is configured so the
		// Redis copy matches the encrypted durable Postgres copy (see persistToDB).
		if data, mErr := json.Marshal(configs); mErr == nil {
			if payload, encErr := m.encodeForCache(data); encErr != nil {
				log.L().Warn("failed to encrypt LLM config for Redis cache",
					zap.Uint("userID", userID), zap.Error(encErr))
			} else if sErr := m.rdb.Set(ctx, m.key(userID), payload, 0).Err(); sErr != nil {
				log.L().Warn("failed to repopulate LLM config cache from durable store",
					zap.Uint("userID", userID), zap.Error(sErr))
			}
		}
		return configs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("llmconfig: redis get failed: %w", err)
	}
	return m.decodeFromCache(raw)
}

// GetConfig returns a single config by ID, or nil if not found.
func (m *Manager) GetConfig(userID uint, configID string) (*UserLLMConfig, error) {
	configs, err := m.GetConfigs(userID)
	if err != nil {
		return nil, err
	}
	for i := range configs {
		if configs[i].ID == configID {
			return &configs[i], nil
		}
	}
	return nil, nil
}

// GetDefaultConfig returns the config marked as default, or the first config
// if none is explicitly marked, or nil if the user has no configs.
func (m *Manager) GetDefaultConfig(userID uint) (*UserLLMConfig, error) {
	configs, err := m.GetConfigs(userID)
	if err != nil {
		return nil, err
	}
	for i := range configs {
		if configs[i].IsDefault {
			return &configs[i], nil
		}
	}
	if len(configs) > 0 {
		return &configs[0], nil
	}
	return nil, nil
}

// ── Mutating methods ──────────────────────────────────────────────────────────

// AddConfig appends a new config. A UUID is assigned automatically.
// The first config for a user is automatically made the default.
func (m *Manager) AddConfig(userID uint, cfg UserLLMConfig) (UserLLMConfig, error) {
	cfg.ID = uuid.New().String()

	err := m.mutate(userID, func(configs []UserLLMConfig) ([]UserLLMConfig, error) {
		if len(configs) == 0 {
			cfg.IsDefault = true
		} else if cfg.IsDefault {
			for i := range configs {
				configs[i].IsDefault = false
			}
		}
		return append(configs, cfg), nil
	})
	if err != nil {
		return UserLLMConfig{}, err
	}
	return cfg, nil
}

// UpdateConfig replaces an existing config entry by ID.
func (m *Manager) UpdateConfig(userID uint, configID string, updated UserLLMConfig) error {
	return m.mutate(userID, func(configs []UserLLMConfig) ([]UserLLMConfig, error) {
		for i, c := range configs {
			if c.ID == configID {
				updated.ID = configID
				if updated.IsDefault {
					for j := range configs {
						configs[j].IsDefault = false
					}
				}
				configs[i] = updated
				return configs, nil
			}
		}
		return nil, fmt.Errorf("llmconfig: config %q not found", configID)
	})
}

// DeleteConfig removes a config by ID. If the deleted config was the default,
// the first remaining config is promoted to default.
func (m *Manager) DeleteConfig(userID uint, configID string) error {
	return m.mutate(userID, func(configs []UserLLMConfig) ([]UserLLMConfig, error) {
		out := configs[:0]
		found := false
		for _, c := range configs {
			if c.ID == configID {
				found = true
				continue
			}
			out = append(out, c)
		}
		if !found {
			return nil, fmt.Errorf("llmconfig: config %q not found", configID)
		}
		// Promote first remaining entry to default if needed.
		if len(out) > 0 {
			hasDefault := false
			for _, c := range out {
				if c.IsDefault {
					hasDefault = true
					break
				}
			}
			if !hasDefault {
				out[0].IsDefault = true
			}
		}
		return out, nil
	})
}

// ── Core: atomic read-modify-write ────────────────────────────────────────────

// mutate serialises a read-modify-write on the user's config array using two
// layers of concurrency control:
//
//  1. Per-user in-process mutex — prevents goroutines on this pod from racing
//     each other without hitting Redis at all.
//
//  2. Redis WATCH / MULTI / EXEC — detects concurrent modifications from other
//     pods between our GET and SET.  On TxFailedErr we retry up to maxRetries
//     times.  Any other error (Redis I/O failure or a user-logic error from fn)
//     is returned immediately without retrying.
func (m *Manager) mutate(userID uint, fn func([]UserLLMConfig) ([]UserLLMConfig, error)) error {
	mu := m.userMu(userID)
	mu.Lock()
	defer mu.Unlock()

	key := m.key(userID)
	ctx := context.Background()

	var finalData []byte
	for range maxRetries {
		err := m.rdb.Watch(ctx, func(tx *redis.Tx) error {
			// Read current state inside the WATCH scope.
			raw, err := tx.Get(ctx, key).Result()
			var configs []UserLLMConfig
			switch {
			case errors.Is(err, redis.Nil):
				configs = []UserLLMConfig{}
			case err != nil:
				return fmt.Errorf("llmconfig: redis get failed: %w", err)
			default:
				configs, err = m.decodeFromCache(raw)
				if err != nil {
					return err
				}
			}

			// Apply business logic mutation.
			newConfigs, err := fn(configs)
			if err != nil {
				return err // user-logic error — do not retry
			}

			data, err := json.Marshal(newConfigs)
			if err != nil {
				return fmt.Errorf("llmconfig: marshal failed: %w", err)
			}
			// finalData stays plaintext: persistToDB encrypts it for Postgres
			// separately. Only the Redis copy is encrypted here.
			finalData = data

			cachePayload, err := m.encodeForCache(data)
			if err != nil {
				return fmt.Errorf("llmconfig: encrypt failed: %w", err)
			}

			// MULTI / EXEC: Redis aborts (TxFailedErr) if the key was
			// modified by another client since WATCH.
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				return pipe.Set(ctx, key, cachePayload, 0).Err()
			})
			return err
		}, key)

		if err == nil {
			// Mirror to the durable store (encrypted at rest) so configs survive a
			// Redis flush. Best-effort: Redis already committed via EXEC; a failed
			// mirror is healed by the next mutation and reads still hit Redis.
			if dbErr := m.persistToDB(userID, finalData); dbErr != nil {
				log.L().Warn("failed to persist LLM config to durable store",
					zap.Uint("userID", userID), zap.Error(dbErr))
			}
			return nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			// Another pod changed the key; the local mutex prevented same-pod
			// races, so this must be a cross-pod conflict — retry.
			continue
		}
		// Any other error is not retryable.
		return err
	}

	return fmt.Errorf("llmconfig: mutation for user %d failed after %d retries (concurrent write conflict)", userID, maxRetries)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Manager) key(userID uint) string {
	return fmt.Sprintf("%s%d", redisKeyPrefix, userID)
}

// encodeForCache serialises the configs payload for the Redis cache, encrypting
// it at rest when an encryption key is configured. This keeps the Redis copy
// consistent with the encrypted durable Postgres copy (see persistToDB) so API
// keys aren't exposed via RDB/AOF snapshots, MONITOR, or a Redis memory dump.
func (m *Manager) encodeForCache(data []byte) (string, error) {
	if m.box == nil {
		return string(data), nil
	}
	return m.box.Encrypt(data)
}

// decodeFromCache reverses encodeForCache. When a key is configured it expects
// ciphertext but tolerates legacy plaintext values written before encryption was
// enabled, so a rolling deploy need not flush Redis.
func (m *Manager) decodeFromCache(raw string) ([]UserLLMConfig, error) {
	payload := []byte(raw)
	if m.box != nil {
		if pt, err := m.box.Decrypt(raw); err == nil {
			payload = pt
		}
		// Decrypt failure → fall back to treating raw as legacy plaintext JSON.
	}
	var configs []UserLLMConfig
	if err := json.Unmarshal(payload, &configs); err != nil {
		return nil, fmt.Errorf("llmconfig: unmarshal failed: %w", err)
	}
	return configs, nil
}
