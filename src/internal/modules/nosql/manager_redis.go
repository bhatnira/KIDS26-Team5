package nosql

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"antelope/internal/modules/log"
	"antelope/internal/modules/misc"
	"antelope/internal/modules/setting"

	"github.com/redis/go-redis/v9"
)

// UniversalRedisClient is an alias for redis.UniversalClient, re-exported so
// business code only needs to import this package.
type UniversalRedisClient = redis.UniversalClient

var replacer = strings.NewReplacer("_", "", "-", "")

// RedisClient is a reference-counted handle to a shared Redis connection.
// Always call Close when done to release the reference.
// Implements io.Closer.
type RedisClient struct {
	// Client is the underlying Redis client. Use this for all Redis operations.
	Client UniversalRedisClient
	// canonical is the normalized URI used to look up the holder in the map.
	canonical string
	mgr       *Manager
}

// Close decrements the reference count for the underlying connection.
// When the count reaches zero the connection is closed.
func (c *RedisClient) Close() error {
	return c.mgr.closeRedisClient(c.canonical)
}

// GetRedisClient returns a reference-counted RedisClient for the given
// connection string (URI or legacy key=value format).
// Call RedisClient.Close when done so the underlying connection is released
// when no longer referenced.
func (m *Manager) GetRedisClient(connection string) (*RedisClient, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Fast path: original string already in the map (most common case after
	// the first call with the same string).
	// Fix: use holder.canonical instead of holder.aliases[1] to avoid
	// relying on aliases slice ordering.
	if holder, ok := m.RedisConnections[connection]; ok {
		holder.count++
		return &RedisClient{Client: holder.client, canonical: holder.canonical, mgr: m}, nil
	}

	// Normalize to canonical URI and try again.
	uri := ToRedisURI(connection)
	canonical := uri.String()

	// Fix: also check clientname before treating this as a brand-new connection,
	// so that a call with only clientname reuses an existing holder instead of
	// creating a duplicate connection.
	clientName := uri.Query().Get("clientname")

	if holder, ok := m.RedisConnections[canonical]; ok {
		// The canonical URI was already registered under a different original
		// string. Register the new alias so future lookups and Close calls
		// using this string also work.
		holder.aliases = append(holder.aliases, connection)
		m.RedisConnections[connection] = holder
		holder.count++
		return &RedisClient{Client: holder.client, canonical: holder.canonical, mgr: m}, nil
	}

	if clientName != "" {
		if holder, ok := m.RedisConnections[clientName]; ok {
			// Found via clientname — register the new original string as an alias.
			holder.aliases = append(holder.aliases, connection)
			m.RedisConnections[connection] = holder
			holder.count++
			return &RedisClient{Client: holder.client, canonical: holder.canonical, mgr: m}, nil
		}
	}

	// Brand-new connection — build the client.
	// Fix: canonical is stored explicitly in the holder struct, not inferred
	// from aliases index positions.
	aliases := []string{connection, canonical}

	opts := getRedisOptions(uri)
	tlsConfig := getRedisTLSOptions(uri)

	if clientName != "" {
		aliases = append(aliases, clientName)
	}

	var universalClient UniversalRedisClient
	switch uri.Scheme {
	case "redis+sentinels", "rediss+sentinel":
		opts.TLSConfig = tlsConfig
		fallthrough
	case "redis+sentinel":
		universalClient = redis.NewFailoverClient(opts.Failover())
	case "redis+clusters", "rediss+cluster":
		opts.TLSConfig = tlsConfig
		fallthrough
	case "redis+cluster":
		universalClient = redis.NewClusterClient(opts.Cluster())
	case "redis+socket":
		simpleOpts := opts.Simple()
		simpleOpts.Network = "unix"
		simpleOpts.Addr = path.Join(uri.Host, uri.Path)
		universalClient = redis.NewClient(simpleOpts)
	case "rediss":
		opts.TLSConfig = tlsConfig
		fallthrough
	case "redis":
		universalClient = redis.NewClient(opts.Simple())
	default:
		return nil, fmt.Errorf("nosql: unsupported Redis URI scheme %q", uri.Scheme)
	}

	holder := &redisClientHolder{
		client:    universalClient,
		canonical: canonical, // stored explicitly — never rely on aliases ordering
		aliases:   aliases,
		count:     1,
	}
	for _, alias := range aliases {
		m.RedisConnections[alias] = holder
	}

	return &RedisClient{Client: universalClient, canonical: canonical, mgr: m}, nil
}

// closeRedisClient decrements the reference count for the given connection and
// closes the underlying client when the count reaches zero.
// Prefer calling RedisClient.Close instead of this method directly.
func (m *Manager) closeRedisClient(connection string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	holder, ok := m.RedisConnections[connection]
	if !ok {
		return nil
	}

	holder.count--
	if holder.count > 0 {
		return nil
	}

	for _, alias := range holder.aliases {
		delete(m.RedisConnections, alias)
	}
	return holder.client.Close()
}

// Ping verifies that the Redis connection identified by the canonical URI is
// still reachable. ctx can carry a deadline/cancellation.
func (m *Manager) Ping(ctx context.Context, canonical string) error {
	m.mutex.Lock()
	holder, ok := m.RedisConnections[canonical]
	m.mutex.Unlock()

	if !ok {
		return fmt.Errorf("nosql: no active Redis connection for %q", redactRedisURI(canonical))
	}
	if err := holder.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("nosql: Redis ping failed for %q: %w", redactRedisURI(canonical), err)
	}
	return nil
}

// redactRedisURI masks the password in a canonical Redis URI so it can be safely
// embedded in error messages and logs. url.URL.String() (used to build the
// canonical form) emits the password in plaintext — e.g. redis://:secret@h:6379/0;
// url.URL.Redacted() replaces it with "xxxxx" while preserving host/port/db for
// debuggability. Falls back to a static string if the URI cannot be parsed.
func redactRedisURI(canonical string) string {
	if u, err := url.Parse(canonical); err == nil {
		return u.Redacted()
	}
	return "redis://<redacted>"
}

// InitRedis initialises a Redis client from the application config and verifies
// the connection with a PING. It panics on failure so that the server does not
// start with a broken Redis connection.
//
// The returned RedisClient must be closed on server shutdown:
//
//	client := InitRedis(cfg)
//	defer client.Close()
func InitRedis(cfg setting.RedisConfig) *RedisClient {
	uri := cfg.URI()

	client, err := GetManager().GetRedisClient(uri)
	if err != nil {
		panic(fmt.Errorf("nosql: InitRedis failed: %w", err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := GetManager().Ping(ctx, client.canonical); err != nil {
		panic(fmt.Errorf("nosql: InitRedis ping failed: %w", err))
	}

	log.L().Info("connected to Redis successfully")
	return client
}

// getRedisOptions builds a redis.UniversalOptions from a parsed Redis URI.
func getRedisOptions(uri *url.URL) *redis.UniversalOptions {
	opts := &redis.UniversalOptions{}

	if password, ok := uri.User.Password(); ok {
		opts.Password = password
		opts.Username = uri.User.Username()
	} else if uri.User.Username() != "" {
		opts.Password = uri.User.Username()
	}

	for k, v := range uri.Query() {
		switch replacer.Replace(strings.ToLower(k)) {
		case "addr":
			opts.Addrs = append(opts.Addrs, v...)
		case "addrs":
			opts.Addrs = append(opts.Addrs, strings.Split(v[0], ",")...)
		case "username":
			opts.Username = v[0]
		case "password":
			opts.Password = v[0]
		case "database", "db":
			opts.DB, _ = strconv.Atoi(v[0])
		case "maxretries":
			opts.MaxRetries, _ = strconv.Atoi(v[0])
		case "minretrybackoff":
			opts.MinRetryBackoff = valToTimeDuration(v)
		case "maxretrybackoff":
			opts.MaxRetryBackoff = valToTimeDuration(v)
		case "timeout":
			if timeout := valToTimeDuration(v); timeout != 0 {
				if opts.DialTimeout == 0 {
					opts.DialTimeout = timeout
				}
				if opts.ReadTimeout == 0 {
					opts.ReadTimeout = timeout
				}
			}
		case "dialtimeout":
			opts.DialTimeout = valToTimeDuration(v)
		case "readtimeout":
			opts.ReadTimeout = valToTimeDuration(v)
		case "writetimeout":
			opts.WriteTimeout = valToTimeDuration(v)
		case "poolsize":
			opts.PoolSize, _ = strconv.Atoi(v[0])
		case "minidleconns":
			opts.MinIdleConns, _ = strconv.Atoi(v[0])
		case "pooltimeout":
			opts.PoolTimeout = valToTimeDuration(v)
		case "maxredirects":
			opts.MaxRedirects, _ = strconv.Atoi(v[0])
		case "readonly":
			opts.ReadOnly, _ = strconv.ParseBool(v[0])
		case "routebylatency":
			opts.RouteByLatency, _ = strconv.ParseBool(v[0])
		case "routerandomly":
			opts.RouteRandomly, _ = strconv.ParseBool(v[0])
		case "sentinelmasterid", "mastername":
			opts.MasterName = v[0]
		case "sentinelusername":
			opts.SentinelUsername = v[0]
		case "sentinelpassword":
			opts.SentinelPassword = v[0]
		}
	}

	if uri.Host != "" {
		opts.Addrs = append(opts.Addrs, strings.Split(uri.Host, ",")...)
	}

	if uri.Path != "" && uri.Scheme != "redis+socket" {
		if db, err := strconv.Atoi(uri.Path[1:]); err == nil {
			opts.DB = db
		}
	}

	return opts
}

func getRedisTLSOptions(uri *url.URL) *tls.Config {
	tlsConfig := &tls.Config{}

	for _, key := range []string{"skipverify", "insecureskipverify"} {
		if val := uri.Query().Get(key); val != "" {
			if skip, err := strconv.ParseBool(val); err == nil {
				tlsConfig.InsecureSkipVerify = skip
			}
		}
	}

	return tlsConfig
}

func valToTimeDuration(vs []string) (result time.Duration) {
	var err error
	for _, v := range vs {
		result, err = misc.ParseDuration(v)
		if err != nil {
			var val int
			val, err = strconv.Atoi(v)
			result = time.Duration(val)
		}
		if err == nil {
			return result
		}
	}
	return result
}
