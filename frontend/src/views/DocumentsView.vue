<!--
  Copyright 2025 Alby Hernández
  Licensed under the Apache License, Version 2.0
-->

<template>
  <div class="documents-view">

    <!-- Upload -->
    <section class="card upload-card">
      <div class="card-header">
        <h2>Upload</h2>
        <span class="card-hint">PDF, TXT, MD, DOCX supported</span>
      </div>
      <div
        class="drop-zone"
        :class="{ dragging, uploading }"
        @dragover.prevent="dragging = true"
        @dragleave="dragging = false"
        @drop.prevent="onDrop"
        @click="!uploading && $refs.fileInput.click()"
      >
        <div class="drop-zone-inner">
          <div class="drop-icon" :class="{ spin: uploading }">
            <svg v-if="!uploading" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M12 16V4m0 0L8 8m4-4l4 4" stroke-linecap="round" stroke-linejoin="round"/>
              <path d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1" stroke-linecap="round"/>
            </svg>
            <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
              <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" stroke-linecap="round"/>
            </svg>
          </div>
          <p class="drop-text">
            <span v-if="!uploading">Drop a file here or <strong>click to browse</strong></span>
            <span v-else>Uploading…</span>
          </p>
        </div>
        <input ref="fileInput" type="file" hidden @change="onFileChange" />
      </div>
      <div v-if="uploadError"   class="message message-error">{{ uploadError }}</div>
      <div v-if="uploadSuccess" class="message message-success">{{ uploadSuccess }}</div>
    </section>

    <!-- Documents list -->
    <section class="card list-card">
      <div class="card-header">
        <h2>Documents <span class="total-badge" v-if="total > 0">{{ total }}</span></h2>
        <div class="list-actions">
          <select v-model="statusFilter" @change="onFilterChange" class="ctrl-select">
            <option value="">All statuses</option>
            <option value="pending">Pending</option>
            <option value="processing">Processing</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
          </select>
          <select v-model="sortBy" @change="onFilterChange" class="ctrl-select">
            <option value="created_at">By date</option>
            <option value="filename">By name</option>
          </select>
          <button class="ctrl-btn" :class="{ active: sortOrder === 'asc' }" @click="toggleOrder" :title="sortOrder === 'desc' ? 'Oldest first' : 'Newest first'">
            {{ sortOrder === 'desc' ? '↓' : '↑' }}
          </button>
          <select v-model="pageSize" @change="onFilterChange" class="ctrl-select">
            <option :value="10">10 / page</option>
            <option :value="25">25 / page</option>
            <option :value="50">50 / page</option>
          </select>
          <button class="btn-icon" @click="load" title="Refresh">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M4 4v5h5M20 20v-5h-5" stroke-linecap="round" stroke-linejoin="round"/>
              <path d="M4.05 9A9 9 0 1120 15.95" stroke-linecap="round"/>
            </svg>
          </button>
        </div>
      </div>

      <p v-if="loading" class="state-msg">Loading…</p>
      <p v-else-if="!documents.length" class="state-msg">No documents yet. Upload one above.</p>

      <table v-else class="doc-table">
        <thead>
          <tr>
            <th>Filename</th>
            <th>Format</th>
            <th>Status</th>
            <th>Chunks</th>
            <th>Uploaded</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="doc in documents" :key="doc.id">
            <td class="col-filename">{{ doc.filename }}</td>
            <td><span class="badge badge-format">{{ doc.format }}</span></td>
            <td><StatusBadge :status="doc.status" /></td>
            <td class="col-num">{{ doc.chunk_count }}</td>
            <td class="col-date">{{ formatDate(doc.created_at) }}</td>
            <td>
              <button class="btn-delete" @click="remove(doc.id)" title="Delete">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- Pagination -->
      <div v-if="totalPages > 1" class="pagination">
        <button class="page-btn" :disabled="page === 0" @click="goTo(page - 1)">←</button>
        <span class="page-info">{{ page + 1 }} / {{ totalPages }}</span>
        <button class="page-btn" :disabled="page >= totalPages - 1" @click="goTo(page + 1)">→</button>
      </div>
    </section>

  </div>
</template>

<script setup>
import { ref, computed, watch, inject, onMounted } from 'vue'
import { uploadDocument, listDocuments, deleteDocument } from '../api.js'
import StatusBadge from '../components/StatusBadge.vue'

const emit = defineEmits(['unauthorized'])

// Namespace injected from App.vue — reload when it changes.
const namespace = inject('namespace')

const documents     = ref([])
const total         = ref(0)
const loading       = ref(false)
const statusFilter  = ref('')
const sortBy        = ref('created_at')
const sortOrder     = ref('desc')
const pageSize      = ref(10)
const page          = ref(0)
const dragging      = ref(false)
const uploading     = ref(false)
const uploadError   = ref('')
const uploadSuccess = ref('')

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

// Resets pagination and reloads — called whenever a filter or sort option changes.
function onFilterChange() { page.value = 0; load() }
// Toggle between ascending and descending order.
function toggleOrder()    { sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'; onFilterChange() }
// Jump to a specific page.
function goTo(p)          { page.value = p; load() }

// Centralised API error handler.
// 401 bubbles up to App.vue to force re-login; 403 shows a namespace error.
function handleApiError(e) {
  if (e.status === 401) { emit('unauthorized'); return }
  uploadError.value = e.status === 403 ? 'No access to this namespace.' : e.message
}

// Reload the current page of documents with the active filters.
async function load() {
  loading.value = true
  try {
    const result = await listDocuments({
      status:    statusFilter.value,
      sortBy:    sortBy.value,
      sortOrder: sortOrder.value,
      limit:     pageSize.value,
      offset:    page.value * pageSize.value,
    })
    documents.value = result.documents
    total.value     = result.total
  } catch (e) {
    handleApiError(e)
  } finally {
    loading.value = false
  }
}

// Upload a file: compute dedup on the server (409), enqueue on success.
async function upload(file) {
  uploading.value     = true
  uploadError.value   = ''
  uploadSuccess.value = ''
  try {
    const doc = await uploadDocument(file)
    uploadSuccess.value = `"${doc.filename}" queued for ingestion.`
    page.value = 0
    await load()
  } catch (e) {
    if (e.status === 409) uploadError.value = e.message
    else handleApiError(e)
  } finally {
    uploading.value = false
  }
}

// Delete a document and all its chunks. Step back one page if the last item on a page was removed.
async function remove(id) {
  if (!confirm('Delete this document and all its chunks?')) return
  try {
    await deleteDocument(id)
    if (documents.value.length === 1 && page.value > 0) page.value--
    await load()
  } catch (e) {
    if (e.status === 401) emit('unauthorized')
    else alert(e.message)
  }
}

// File input helpers — both delegate to upload().
function onDrop(e)      { dragging.value = false; const f = e.dataTransfer.files[0]; if (f) upload(f) }
function onFileChange(e) { const f = e.target.files[0]; if (f) upload(f) }

// Format an ISO date string to a compact locale-aware representation.
function formatDate(iso) {
  return new Date(iso).toLocaleString(undefined, {
    day: '2-digit', month: 'short', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

// Reset to page 0 and reload when namespace changes (injected from App.vue).
watch(namespace, () => { page.value = 0; load() })
onMounted(load)
</script>

<style scoped>
.documents-view { display: flex; flex-direction: column; gap: 1.5rem; }

.card { background: var(--card-bg); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: var(--shadow-sm); overflow: hidden; }
.card-header { display: flex; align-items: center; justify-content: space-between; padding: 1.1rem 1.5rem 0.9rem; border-bottom: 1px solid var(--border); }
.card-header h2 { font-size: 0.95rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.07em; color: var(--ink-muted); display: flex; align-items: center; gap: 0.5rem; }
.card-hint { font-size: 0.78rem; color: var(--ink-muted); }

.total-badge { background: var(--pink-hot); color: #fff; border-radius: 20px; padding: 0.1rem 0.5rem; font-size: 0.7rem; font-weight: 700; }

.drop-zone { margin: 1.25rem 1.5rem; border: 2px dashed var(--border); border-radius: 8px; cursor: pointer; transition: border-color 0.2s, background 0.2s; background: var(--pink-bg); }
.drop-zone:hover, .drop-zone.dragging { border-color: var(--pink-hot); background: #fff0f3; }
.drop-zone.uploading { cursor: default; border-color: var(--pink-hot); opacity: 0.7; }
.drop-zone-inner { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 0.6rem; padding: 2rem 1rem; }
.drop-icon { width: 36px; height: 36px; color: var(--pink-hot); }
.drop-icon.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
.drop-text { font-size: 0.9rem; color: var(--ink-muted); text-align: center; }
.drop-text strong { color: var(--pink-hot); font-weight: 600; }

.message { margin: 0 1.5rem 1.25rem; padding: 0.65rem 0.9rem; border-radius: 6px; font-size: 0.85rem; font-weight: 500; }
.message-error   { background: #fff1f1; color: var(--red);  border: 1px solid #fcc; }
.message-success { background: #f0fdf4; color: #16a34a;     border: 1px solid #bbf7d0; }

.list-actions { display: flex; align-items: center; gap: 0.4rem; flex-wrap: wrap; }

.ctrl-select { border: 1.5px solid var(--border); border-radius: 6px; padding: 0.3rem 0.5rem; font-size: 0.78rem; color: var(--ink); background: var(--card-bg); cursor: pointer; }
.ctrl-btn { border: 1.5px solid var(--border); border-radius: 6px; padding: 0.3rem 0.55rem; font-size: 0.85rem; background: var(--card-bg); color: var(--ink-muted); cursor: pointer; transition: background 0.15s; }
.ctrl-btn.active { background: var(--pink-mid); color: var(--ink); }

@media (max-width: 600px) {
  .card-header { flex-direction: column; align-items: flex-start; gap: 0.6rem; padding: 0.9rem 1rem 0.75rem; }
  .list-actions { width: 100%; }
  .ctrl-select, .ctrl-btn, .btn-icon { flex: 1; min-width: 0; }
  .doc-table th:nth-child(4),
  .doc-table td:nth-child(4),
  .doc-table th:nth-child(5),
  .doc-table td:nth-child(5) { display: none; }
  .col-filename { max-width: 140px; }
  .drop-zone-inner { padding: 1.25rem 1rem; }
  .message { margin: 0 1rem 1rem; }
}

.btn-icon { background: transparent; border: 1.5px solid var(--border); border-radius: 6px; padding: 0.3rem; cursor: pointer; display: flex; align-items: center; color: var(--ink-muted); transition: border-color 0.15s, color 0.15s; }
.btn-icon:hover { border-color: var(--pink-hot); color: var(--pink-hot); }
.btn-icon svg { width: 15px; height: 15px; }

.state-msg { padding: 2rem 1.5rem; color: var(--ink-muted); font-size: 0.9rem; text-align: center; }

.doc-table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
.doc-table th { text-align: left; padding: 0.6rem 1rem; font-size: 0.72rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.06em; color: var(--ink-muted); border-bottom: 1px solid var(--border); background: var(--pink-bg); }
.doc-table td { padding: 0.7rem 1rem; border-bottom: 1px solid #f5ecef; color: var(--ink); vertical-align: middle; }
.doc-table tr:last-child td { border-bottom: none; }
.doc-table tr:hover td { background: var(--pink-bg); }

.col-filename { font-family: monospace; font-size: 0.82rem; max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
.col-num  { color: var(--ink-muted); text-align: center; }
.col-date { color: var(--ink-muted); font-size: 0.78rem; white-space: nowrap; }

.badge { display: inline-block; border-radius: 4px; padding: 0.15rem 0.45rem; font-size: 0.72rem; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; }
.badge-format { background: #fff0f3; color: var(--pink-hot); border: 1px solid #f4c0cc; }

.btn-delete { background: transparent; border: 1.5px solid transparent; border-radius: 6px; padding: 0.3rem; cursor: pointer; display: flex; align-items: center; color: var(--ink-muted); transition: border-color 0.15s, color 0.15s; }
.btn-delete:hover { border-color: var(--red); color: var(--red); }
.btn-delete svg { width: 15px; height: 15px; }

.pagination { display: flex; align-items: center; justify-content: center; gap: 0.75rem; padding: 1rem; border-top: 1px solid var(--border); }
.page-btn { background: var(--card-bg); border: 1.5px solid var(--border); border-radius: 6px; padding: 0.35rem 0.75rem; cursor: pointer; font-size: 0.9rem; color: var(--ink); transition: border-color 0.15s; }
.page-btn:hover:not(:disabled) { border-color: var(--pink-hot); color: var(--pink-hot); }
.page-btn:disabled { opacity: 0.35; cursor: default; }
.page-info { font-size: 0.82rem; color: var(--ink-muted); min-width: 60px; text-align: center; }
</style>
