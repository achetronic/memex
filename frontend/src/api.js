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
 * All functions return the parsed JSON response or throw on error.
 */

const BASE = '/api/v1'

/**
 * uploadDocument sends a file to the ingestion endpoint.
 * @param {File} file
 * @returns {Promise<Object>} Created document record.
 */
export async function uploadDocument(file) {
  const form = new FormData()
  form.append('file', file)
  const res = await fetch(`${BASE}/documents`, { method: 'POST', body: form })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Upload failed')
  return data
}

/**
 * listDocuments fetches all documents, optionally filtered by status.
 * @param {string} [status] - One of: pending, processing, completed, failed
 * @returns {Promise<Array>}
 */
export async function listDocuments(status) {
  const url = status ? `${BASE}/documents?status=${status}` : `${BASE}/documents`
  const res = await fetch(url)
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Failed to list documents')
  return data
}

/**
 * getDocument fetches a single document by ID.
 * @param {string} id - UUID
 * @returns {Promise<Object>}
 */
export async function getDocument(id) {
  const res = await fetch(`${BASE}/documents/${id}`)
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Document not found')
  return data
}

/**
 * deleteDocument removes a document and all its chunks.
 * @param {string} id - UUID
 * @returns {Promise<void>}
 */
export async function deleteDocument(id) {
  const res = await fetch(`${BASE}/documents/${id}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error || 'Delete failed')
  }
}

/**
 * search performs a semantic search query.
 * @param {string} query - Natural language query
 * @param {number} [limit] - Max results to return
 * @returns {Promise<{results: Array}>}
 */
export async function search(query, limit) {
  const res = await fetch(`${BASE}/search`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, limit }),
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'Search failed')
  return data
}
