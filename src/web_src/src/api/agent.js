import { request } from '@/service/http'

// ── Workspace ─────────────────────────────────────────────────────────────────

export function fetchAgentWorkspaceConfig() {
  return request.Get('/agent/workspace-config')
}

/**
 * Save the workspace config.
 *
 * Credential sentinel (daytona_api_key):
 *   ''         → keep current
 *   '<clear>'  → wipe
 *   any other  → set to that value
 */
export function saveAgentWorkspaceConfig({
  bucket,
  daytona_api_key,
  daytona_api_url,
}) {
  return request.Put('/agent/workspace-config', {
    bucket,
    daytona_api_key,
    daytona_api_url,
  })
}

// ── MCP servers ───────────────────────────────────────────────────────────────

export function fetchMCPConfigs() {
  return request.Get('/agent/mcp-configs')
}

export function addMCPConfig(data) {
  return request.Post('/agent/mcp-configs', data)
}

export function updateMCPConfig(id, data) {
  return request.Put(`/agent/mcp-configs/${id}`, data)
}

export function deleteMCPConfig(id) {
  return request.Delete(`/agent/mcp-configs/${id}`)
}

// ── Skills ────────────────────────────────────────────────────────────────────

// The built-in library is ~700 skills, so searching, domain filtering and
// paging are all server-side. params: { search, domain, scope, limit, offset }.
export function fetchAgentSkills(params = {}) {
  return request.Get('/agent/skills', { params })
}

export function fetchAgentSkill(name) {
  return request.Get(`/agent/skills/${encodeURIComponent(name)}`)
}

// ── Artifacts ─────────────────────────────────────────────────────────────────

export function fetchListArtifacts(sessionId) {
  return request.Get(`/agent/sessions/${sessionId}/artifacts`)
}

/**
 * Get a presigned download URL for an artifact.
 *
 * version<0 (or omitted) → latest. The wildcard path supports filenames
 * containing slashes (the framework treats them as a single logical key).
 */
export function fetchDownloadArtifact(sessionId, name, version) {
  const params = {}
  if (typeof version === 'number' && version >= 0) {
    params.version = version
  }
  // Wildcard route: /agent/sessions/:id/artifacts/*key
  return request.Get(
    `/agent/sessions/${sessionId}/artifacts/${encodeURIComponent(name)}`,
    { params }
  )
}
