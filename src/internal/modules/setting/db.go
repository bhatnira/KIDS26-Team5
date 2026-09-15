package setting

import "fmt"

type DBConfig struct {
	Host     string `mapstructure:"host" json:"host" yaml:"host"`
	Port     int    `mapstructure:"port" json:"port" yaml:"port"`
	DB       string `mapstructure:"db" json:"db" yaml:"db"`
	Username string `mapstructure:"username" json:"username" yaml:"username"`
	Password string `mapstructure:"password" json:"password" yaml:"password"`
	SSLMode  string `mapstructure:"sslmode" json:"sslmode" yaml:"sslmode"`
}

// sslMode returns the effective SSL mode, defaulting to "disable" if unset.
func (db *DBConfig) sslMode() string {
	if db.SSLMode == "" {
		return "disable"
	}
	return db.SSLMode
}

// Dsn returns a PostgreSQL DSN connection string.
func (db *DBConfig) Dsn() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC",
		db.Host, db.Username, db.Password, db.DB, db.Port, db.sslMode())
}
