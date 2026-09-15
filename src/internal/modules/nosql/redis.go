package nosql

import (
	"net/url"
	"strconv"
	"strings"
)

// ToRedisURI converts a connection string to a canonical Redis URI.
//
// Supported URI schemes (passed through as-is):
//
//	redis://[username:password@]host[:port][/database][?options]
//	rediss://...                          (TLS)
//	redis+socket://...                    (Unix socket)
//	redis+sentinel://...                  (Sentinel)
//	redis+sentinels://...                 (Sentinel + TLS)
//	redis+cluster://...                   (Cluster)
//	redis+clusters://...                  (Cluster + TLS)
//
// Legacy key=value formats are also accepted (space- or comma-delimited):
//
//	addrs=127.0.0.1:6379 db=0
//	network=tcp,addr=127.0.0.1:6379,password=secret,db=0
func ToRedisURI(connection string) *url.URL {
	uri, err := url.Parse(connection)
	if err == nil && strings.HasPrefix(uri.Scheme, "redis") {
		return uri
	}

	uri, _ = url.Parse("redis://127.0.0.1:6379/0")
	network := "tcp"
	query := uri.Query()

	fields := strings.Fields(connection)
	if len(fields) == 1 {
		fields = strings.Split(connection, ",")
	}

	for _, f := range fields {
		items := strings.SplitN(f, "=", 2)
		if len(items) < 2 {
			continue
		}
		switch strings.ToLower(items[0]) {
		case "network":
			if items[1] == "unix" {
				uri.Scheme = "redis+socket"
			}
			network = items[1]
		case "addrs":
			uri.Host = items[1]
			if strings.Contains(items[1], ",") && network == "tcp" {
				uri.Scheme = "redis+cluster"
			}
		case "addr":
			uri.Host = items[1]
		case "password":
			uri.User = url.UserPassword(uri.User.Username(), items[1])
		case "username":
			password, set := uri.User.Password()
			if !set {
				uri.User = url.User(items[1])
			} else {
				uri.User = url.UserPassword(items[1], password)
			}
		case "db":
			uri.Path = "/" + items[1]
		case "idle_timeout":
			if _, err := strconv.Atoi(items[1]); err == nil {
				query.Add("idle_timeout", items[1]+"s")
			} else {
				query.Add("idle_timeout", items[1])
			}
		default:
			query.Add(items[0], items[1])
		}
	}

	if uri.Scheme == "redis+socket" {
		query.Set("db", uri.Path)
		uri.Path = uri.Host
		uri.Host = ""
	}
	uri.RawQuery = query.Encode()

	return uri
}
