// Package source defines the Provider interface and houses all authentication
// source implementations (local, LDAP, OIDC). These live in the service layer
// because they depend on domain models and external configuration.
package source

// Provider is a generic interface for authenticating a subject against a
// configured backend. C is the configuration type; S is the subject type
// whose Verify call returns a canonical identity string on success.
type Provider[C any, S any] interface {
	Connect(config C) error
	Verify(subject S) (string, error)
	Configured() bool
	SetConfig(config C) error
	DeleteConfig()
}
