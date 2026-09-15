package setting

import (
	"fmt"
	"net/url"
)

type RedisConfig struct {
	Host     string `mapstructure:"host" json:"host" yaml:"host"`
	Port     int    `mapstructure:"port" json:"port" yaml:"port"`
	DB       int    `mapstructure:"db" json:"db" yaml:"db"`
	Password string `mapstructure:"password" json:"password" yaml:"password"`
}

// Dsn returns the host:port address string.
func (r *RedisConfig) Dsn() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// URI returns a canonical Redis URI suitable for use with the nosql Manager.
func (r *RedisConfig) URI() string {
	u := &url.URL{
		Scheme: "redis",
		Host:   fmt.Sprintf("%s:%d", r.Host, r.Port),
		Path:   fmt.Sprintf("/%d", r.DB),
	}
	if r.Password != "" {
		u.User = url.UserPassword("", r.Password)
	}
	return u.String()
}
