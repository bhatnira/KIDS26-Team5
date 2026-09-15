// Package agent wraps the trpc-agent-go framework for Antelope's multi-tenant
// chat surface. It owns the runner, the per-user agent factory, the custom
// per-user S3 artifact service, the skill sync logic, and the MCP toolset
// pool. Higher layers (services/agent) consume the package via narrow APIs.
//
// This package intentionally has no dependency on services/* — it is pure
// infrastructure glue between the framework and Antelope's existing modules
// (storage, llmconfig, sse).
package agent
