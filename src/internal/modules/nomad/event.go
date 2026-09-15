package nomad

import (
	"encoding/base64"
	"reflect"
	"time"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/mitchellh/mapstructure"
)

type eventPayload struct {
	Allocation *nomad.Allocation          `mapstructure:"Allocation"`
	Deployment *nomad.Deployment          `mapstructure:"Deployment"`
	Evaluation *nomad.Evaluation          `mapstructure:"Evaluation"`
	Job        *nomad.Job                 `mapstructure:"Job"`
	Node       *nomad.Node                `mapstructure:"Node"`
	NodePool   *nomad.NodePool            `mapstructure:"NodePool"`
	Service    *nomad.ServiceRegistration `mapstructure:"Service"`
}

// ExtendedEvent wraps nomad.Event with safe payload decoding helpers.
type ExtendedEvent nomad.Event

// NewExtendedEvent converts a *nomad.Event to *ExtendedEvent.
func NewExtendedEvent(event *nomad.Event) *ExtendedEvent {
	return (*ExtendedEvent)(event)
}

// ToEvent converts back to the underlying *nomad.Event.
func (e *ExtendedEvent) ToEvent() *nomad.Event {
	return (*nomad.Event)(e)
}

// base64StringToByteSliceHook is a mapstructure decode hook that converts
// base64-encoded strings into []byte. If the string is not valid base64 it
// falls back to a plain []byte conversion.
//
// BUG FIX: the original implementation had to err != nil / err == nil branches
// swapped, causing every base64 payload to be silently discarded.
func base64StringToByteSliceHook() mapstructure.DecodeHookFunc {
	return func(f, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String || t != reflect.TypeFor[[]byte]() {
			return data, nil
		}

		str, ok := data.(string)
		if !ok {
			return data, nil
		}

		// Attempt base64 decoding; return on success.
		if decoded, err := base64.StdEncoding.DecodeString(str); err == nil {
			return decoded, nil
		}

		// Not valid base64 — treat as a plain string and cast to []byte.
		return []byte(str), nil
	}
}

func (e *ExtendedEvent) decodePayload() (*eventPayload, error) {
	var out eventPayload
	cfg := mapstructure.DecoderConfig{
		Result: &out,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeHookFunc(time.RFC3339),
			base64StringToByteSliceHook(),
		),
	}

	dec, err := mapstructure.NewDecoder(&cfg)
	if err != nil {
		return nil, err
	}

	if err := dec.Decode(e.Payload); err != nil {
		return nil, err
	}

	return &out, nil
}

func (e *ExtendedEvent) SafeDeployment() (*nomad.Deployment, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.Deployment, nil
}

func (e *ExtendedEvent) SafeEvaluation() (*nomad.Evaluation, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.Evaluation, nil
}

func (e *ExtendedEvent) SafeAllocation() (*nomad.Allocation, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.Allocation, nil
}

func (e *ExtendedEvent) SafeJob() (*nomad.Job, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.Job, nil
}

func (e *ExtendedEvent) SafeNode() (*nomad.Node, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.Node, nil
}

func (e *ExtendedEvent) SafeNodePool() (*nomad.NodePool, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.NodePool, nil
}

func (e *ExtendedEvent) SafeService() (*nomad.ServiceRegistration, error) {
	out, err := e.decodePayload()
	if err != nil {
		return nil, err
	}
	return out.Service, nil
}
