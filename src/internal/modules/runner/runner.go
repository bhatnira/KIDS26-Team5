// Package runner provides a business-agnostic abstraction over Nomad
// parameterized batch jobs. It knows nothing about database models, ORMs,
// or loggers — callers pass primitives (strings, maps, bytes) only.
//
// The canonical workflow is:
//
//  1. Register a parameterized job once, using a HCL template + a map of
//     variables that get injected as HCL `locals`. One registration can
//     later serve many dispatches.
//  2. Dispatch the job any number of times with a JSON payload (whose
//     schema is entirely up to the caller and the template's run.sh) plus
//     optional Nomad meta and an ID prefix.
//  3. Deregister when the job is retired.
//
// Three built-in engines — "nextflow", "wdl", "script" — are registered by
// NewDefaultRegistry. They all share the same genericRunner implementation;
// the only difference between them is the HCL template string the caller
// passes at Register time. Adding a new engine (e.g. snakemake, cwl) is
// therefore a matter of writing a template and calling Registry.Register —
// no new Go code required.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	nomad "github.com/hashicorp/nomad/api"
)

// RegisterSpec describes a single parameterized-job registration.
//
// Template must be a valid HCL document (typically one of the exported
// constants NextflowHCL / ScriptHCL / WDLHCL, or a user-provided variant).
// Vars are injected as a top-level `locals { ... }` block so the template
// can reference them as `local.<name>`. Values are JSON-marshaled, so any
// type that produces valid HCL when rendered as JSON works (strings,
// numbers, bools, lists, maps).
type RegisterSpec struct {
	JobID        string         // Final Nomad Job ID (also sets Name).
	Template     string         // HCL document; must be valid HCL.
	Vars         map[string]any // Injected as HCL locals; optional.
	Canonicalize bool           // Forwarded to Nomad's ParseHCLOpts.
}

// DispatchSpec describes a single dispatch of a previously-registered
// parameterized job. Payload is opaque bytes — the caller decides the
// schema. BuildJSONPayload is a convenience helper for the common case of
// JSON payloads.
type DispatchSpec struct {
	JobID    string            // ID of the registered parameterized job.
	Meta     map[string]string // Nomad meta (maps to meta_optional/meta_required).
	Payload  []byte            // Opaque payload delivered to dispatch_payload.file.
	IDPrefix string            // Nomad uses this to derive the dispatched job ID.
}

// Runner is the engine abstraction. The default implementation targets
// Nomad; alternative implementations (Kubernetes Jobs, AWS Batch, ...) can
// be registered into the same Registry without changing caller code.
type Runner interface {
	Name() string
	Register(ctx context.Context, nc *nomad.Client, spec RegisterSpec) error
	Dispatch(ctx context.Context, nc *nomad.Client, spec DispatchSpec) (*nomad.JobDispatchResponse, error)
	Deregister(ctx context.Context, nc *nomad.Client, jobID string, purge bool) error
}

// Registry is a thread-safe map of engine name -> Runner.
type Registry struct {
	mu      sync.RWMutex
	runners map[string]Runner
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{runners: make(map[string]Runner)}
}

// NewDefaultRegistry returns a registry preloaded with the three built-in
// engines: "nextflow", "wdl", "script". All three share the same
// genericRunner implementation; the difference is purely which HCL
// template the caller supplies at Register time.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.MustRegister(&genericRunner{name: "nextflow"})
	r.MustRegister(&genericRunner{name: "wdl"})
	r.MustRegister(&genericRunner{name: "script"})
	return r
}

// Register adds a runner. Returns an error if the name is already taken —
// silently overwriting would mask configuration bugs.
func (r *Registry) Register(x Runner) error {
	if x == nil {
		return errors.New("runner: nil runner")
	}
	name := x.Name()
	if name == "" {
		return errors.New("runner: empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.runners[name]; dup {
		return fmt.Errorf("runner: %q already registered", name)
	}
	r.runners[name] = x
	return nil
}

// MustRegister panics on conflict. Intended for init-time registration of
// built-in runners where a duplicate is a programmer error.
func (r *Registry) MustRegister(x Runner) {
	if err := r.Register(x); err != nil {
		panic(err)
	}
}

// Get looks up a runner by engine name.
func (r *Registry) Get(name string) (Runner, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.runners[name]
	if !ok {
		return nil, fmt.Errorf("runner: unknown engine %q", name)
	}
	return x, nil
}

// Available returns the list of registered engine names.
func (r *Registry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.runners))
	for k := range r.runners {
		out = append(out, k)
	}
	return out
}

// BuildJSONPayload is a convenience for callers whose templates expect a
// JSON dispatch payload. It is not required — DispatchSpec.Payload is
// opaque bytes.
func BuildJSONPayload(fields map[string]any) ([]byte, error) {
	return json.Marshal(fields)
}
