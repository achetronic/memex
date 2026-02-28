<!--
  Copyright 2025 Alby Hernández

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->

<template>
  <div class="documents-view">
    <section class="upload-section card">
      <h2>Upload Document</h2>
      <div
        class="drop-zone"
        :class="{ dragging }"
        @dragover.prevent="dragging = true"
        @dragleave="dragging = false"
        @drop.prevent="onDrop"
        @click="$refs.fileInput.click()"
      >
        <span v-if="!uploading">Drop a file here or click to browse</span>
        <span v-else>Uploading…</span>
        <input ref="fileInput" type="file" hidden @change="onFileChange" />
      </div>
      <p v-if="uploadError" class="error">{{ uploadError }}</p>
      <p v-if="uploadSuccess" class="success">{{ uploadSuccess }}</p>
    </section>

    <section class="list-section card">
      <div class="list-header">
        <h2>Documents</h2>
        <div class="filters">
          <select v-model="statusFilter" @change="load">
            <option value="">All</option>
            <option value="pending">Pending</option>
            <option value="processing">Processing</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
          </select>
          <button class="btn-secondary" @click="load">↻ Refresh</button>
        </div>
      </div>

      <p v-if="loading" class="muted">Loading…</p>
      <p v-else-if="!documents.length" class="muted">No documents found.</p>

      <table v-else>
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
            <td class="filename">{{ doc.filename }}</td>
            <td><span class="badge">{{ doc.format }}</span></td>
            <td><StatusBadge :status="doc.status" /></td>
            <td>{{ doc.chunk_count }}</td>
            <td>{{ formatDate(doc.created_at) }}</td>
            <td>
              <button class="btn-danger-sm" @click="remove(doc.id)">Delete</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { uploadDocument, listDocuments, deleteDocument } from '../api.js'
import StatusBadge from '../components/StatusBadge.vue'

const documents = ref([])
const loading = ref(false)
const statusFilter = ref('')
const dragging = ref(false)
const uploading = ref(false)
const uploadError = ref('')
const uploadSuccess = ref('')

async function load() {
  loading.value = true
  try {
    documents.value = await listDocuments(statusFilter.value)
  } finally {
    loading.value = false
  }
}

async function upload(file) {
  uploading.value = true
  uploadError.value = ''
  uploadSuccess.value = ''
  try {
    const doc = await uploadDocument(file)
    uploadSuccess.value = `"${doc.filename}" queued for ingestion (ID: ${doc.id})`
    await load()
  } catch (e) {
    uploadError.value = e.message
  } finally {
    uploading.value = false
  }
}

async function remove(id) {
  if (!confirm('Delete this document and all its chunks?')) return
  try {
    await deleteDocument(id)
    await load()
  } catch (e) {
    alert(e.message)
  }
}

function onDrop(e) {
  dragging.value = false
  const file = e.dataTransfer.files[0]
  if (file) upload(file)
}

function onFileChange(e) {
  const file = e.target.files[0]
  if (file) upload(file)
}

function formatDate(iso) {
  return new Date(iso).toLocaleString()
}

onMounted(load)
</script>

<style scoped>
.documents-view { display: flex; flex-direction: column; gap: 1.5rem; }

.card { background: #fff; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 4px rgba(0,0,0,0.08); }

.upload-section h2,
.list-section h2 { margin-bottom: 1rem; font-size: 1.1rem; }

.drop-zone {
  border: 2px dashed #ccc;
  border-radius: 6px;
  padding: 2rem;
  text-align: center;
  cursor: pointer;
  color: #888;
  transition: border-color 0.15s, background 0.15s;
}

.drop-zone.dragging { border-color: #4f46e5; background: #f0efff; }
.drop-zone:hover { border-color: #4f46e5; }

.list-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }

.filters { display: flex; gap: 0.5rem; align-items: center; }

select {
  border: 1px solid #ddd;
  border-radius: 4px;
  padding: 0.35rem 0.6rem;
  font-size: 0.875rem;
}

table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
th { text-align: left; padding: 0.5rem; border-bottom: 2px solid #eee; color: #555; font-weight: 600; }
td { padding: 0.5rem; border-bottom: 1px solid #f0f0f0; }

.filename { font-family: monospace; font-size: 0.8rem; max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.badge { background: #e8e8e8; border-radius: 4px; padding: 0.1rem 0.4rem; font-size: 0.75rem; font-family: monospace; }

.btn-secondary {
  background: #f0f0f0;
  border: 1px solid #ddd;
  border-radius: 4px;
  padding: 0.35rem 0.75rem;
  cursor: pointer;
  font-size: 0.875rem;
}

.btn-danger-sm {
  background: transparent;
  border: 1px solid #e53e3e;
  color: #e53e3e;
  border-radius: 4px;
  padding: 0.2rem 0.5rem;
  cursor: pointer;
  font-size: 0.75rem;
}

.btn-danger-sm:hover { background: #fff5f5; }

.muted { color: #888; font-size: 0.9rem; }
.error { color: #e53e3e; margin-top: 0.5rem; font-size: 0.875rem; }
.success { color: #38a169; margin-top: 0.5rem; font-size: 0.875rem; }
</style>
