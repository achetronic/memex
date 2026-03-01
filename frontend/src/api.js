// Copyright 2025 Alby Hernández
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * api.js — thin wrapper around the memex REST API.
 *
 * Namespace and API key are stored in sessionStorage and injected into every
 * request automatically. Call setSession() to update them.
 */

const BASE = '/api/v1'

// ── Session ───────────────────────────────────────────────────────────────────

export function getSession() {
  return {
    namespace: sessionStorage.getItem('memex_namespace') || '',
    apiKey:    sessionStorage.getItem('memex_api_key')   || '',
  }
}

export function setSession({ namespace, apiKey }) {
  sessionStorage.setItem('memex_namespace', namespace)
  sessionStorage.setItem('memex_api_key',   apiKey)
}

export function clearSession() {
  sessionStorage.removeItem('memex_namespace')
  sessionStorage.removeItem('memex_api_key')
}

// Builds the headers that every authenticated request must carry.
function authHeaders() {
  const { namespace, apiKey } = getSession()
  const h = {}
  if (namespace) h['X-Memex-Namespace'] = namespace
  if (apiKey)    h['X-Memex-Api-Key']   = apiKey
  return h
}

// ── Info (auth: key only, no namespace required) ─────────────────────────────

/**
 * getServerInfo fetches auth status and allowed namespaces.
 * - Auth disabled: returns full namespace list, no credentials needed.
 * - Auth enabled:  requires X-Memex-Api-Key, returns namespaces for that key.
 *   Throws with status 401 if no key or invalid key.
 * @returns {Promise<{auth_enabled: boolean, namespaces: string[]}>}
 */
export async function getServerInfo() {
  const { apiKey } = getSession()
  const headers = {}
  if (apiKey) headers['X-Memex-Api-Key'] = apiKey
  const res = await fetch(`${BASE}/info`, { headers })
  if (res.status === 401) throw Object.assign(new Error('Unauthorized'), { status: 401 })
  if (!res.ok) throw new Error('Failed to fetch server info')
  return res.json()
}

// ── Documents ────────────────────────────────────────────────────────────────

/**
 * uploadDocument sends a file to the ingestion endpoint.
 * @param {File} file
 * @returns {Promise<Object>} Created document record.
 */
export async function uploadDocument(file) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${BASE}/documents`, {
    method: 'POST',
    headers: authHeaders(),
    body: form,
  })
  if (res.status === 401) throw Object.assign(new Error('Unauthorized'), { status: 401 })
  if (res.status === 403) throw Object.assign(new Error('Access denied for this namespace'), { status: 403 })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Upload failed')
  return data
}

/**
 * listDocuments fetches all documents, optionally filtered by status.
 * @param {string} [status]
 * @returns {Promise<Array>}
 */
export async function listDocuments({ status, sortBy, sortOrder, limit, offset } = {}) {
  const params = new URLSearchParams()
  if (status)    params.set('status',     status)
  if (sortBy)    params.set('sort_by',    sortBy)
  if (sortOrder) params.set('sort_order', sortOrder)
  if (limit)     params.set('limit',      limit)
  if (offset)    params.set('offset',     offset)
  const url = `${BASE}/documents${params.size ? '?' + params : ''}`
  const res = await fetch(url, { headers: authHeaders() })
  if (res.status === 401) throw Object.assign(new Error('Unauthorized'), { status: 401 })
  if (res.status === 403) throw Object.assign(new Error('Access denied for this namespace'), { status: 403 })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Failed to list documents')
  return data
}

/**
 * deleteDocument removes a document and all its chunks.
 * @param {string} id - UUID
 */
export async function deleteDocument(id) {
  const res = await fetch(`${BASE}/documents/${id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  })
  if (res.status === 401) throw Object.assign(new Error('Unauthorized'), { status: 401 })
  if (res.status === 403) throw Object.assign(new Error('Access denied for this namespace'), { status: 403 })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || 'Delete failed')
  }
}

/**
 * search performs a semantic search query.
 * @param {string} query
 * @param {number} [limit]
 * @returns {Promise<{results: Array}>}
 */
export async function search(query, limit) {
  const res = await fetch(`${BASE}/search`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify({ query, limit }),
  })
  if (res.status === 401) throw Object.assign(new Error('Unauthorized'), { status: 401 })
  if (res.status === 403) throw Object.assign(new Error('Access denied for this namespace'), { status: 403 })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Search failed')
  return data
}
