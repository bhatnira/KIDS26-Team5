package runner

import (
	"context"
	"errors"
	"fmt"

	nomad "github.com/hashicorp/nomad/api"
)

// genericRunner is the default Runner implementation. Every built-in
// engine (nextflow, wdl, script) is a genericRunner with a different name;
// the behavior is identical. Engine-specific logic lives in the HCL
// template the caller supplies, not here.
type genericRunner struct {
	name string
}

func (g *genericRunner) Name() string { return g.name }

// Register renders the HCL template (injecting Vars as locals), parses it
// through Nomad's server-side HCL2 parser, optionally overrides the Job
// ID/Name, and registers the job.
func (g *genericRunner) Register(ctx context.Context, nc *nomad.Client, spec RegisterSpec) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if nc == nil {
		return errors.New("runner: nil nomad client")
	}
	if spec.Template == "" {
		return errors.New("runner: empty HCL template")
	}

	rendered, err := RenderHCL(spec.Template, spec.Vars)
	if err != nil {
		return fmt.Errorf("runner: render hcl: %w", err)
	}

	job, err := nc.Jobs().ParseHCLOpts(&nomad.JobsParseRequest{
		JobHCL:       rendered,
		Canonicalize: spec.Canonicalize,
	})
	if err != nil {
		return fmt.Errorf("runner: parse hcl: %w", err)
	}

	if spec.JobID != "" {
		id := spec.JobID
		job.ID = &id
		job.Name = &id
	}

	if _, _, err := nc.Jobs().Register(job, nil); err != nil {
		return fmt.Errorf("runner: register job: %w", err)
	}
	return nil
}

// Dispatch submits a payload to a previously-registered parameterized job.
func (g *genericRunner) Dispatch(ctx context.Context, nc *nomad.Client, spec DispatchSpec) (*nomad.JobDispatchResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if nc == nil {
		return nil, errors.New("runner: nil nomad client")
	}
	if spec.JobID == "" {
		return nil, errors.New("runner: empty job id")
	}
	resp, _, err := nc.Jobs().Dispatch(spec.JobID, spec.Meta, spec.Payload, spec.IDPrefix, nil)
	if err != nil {
		return nil, fmt.Errorf("runner: dispatch: %w", err)
	}
	return resp, nil
}

// Deregister removes a job. When purge is true the job is wiped from
// Nomad's state; otherwise it is marked dead but kept for history.
func (g *genericRunner) Deregister(ctx context.Context, nc *nomad.Client, jobID string, purge bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if nc == nil {
		return errors.New("runner: nil nomad client")
	}
	if jobID == "" {
		return errors.New("runner: empty job id")
	}
	if _, _, err := nc.Jobs().Deregister(jobID, purge, nil); err != nil {
		return fmt.Errorf("runner: deregister: %w", err)
	}
	return nil
}
