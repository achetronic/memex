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
  <div class="search-view card">
    <h2>Semantic Search</h2>

    <form class="search-form" @submit.prevent="doSearch">
      <input
        v-model="query"
        type="text"
        placeholder="Ask anything about your documents…"
        :disabled="loading"
      />
      <div class="search-controls">
        <label>
          Results
          <input v-model.number="limit" type="number" min="1" max="20" />
        </label>
        <button type="submit" :disabled="loading || !query.trim()">
          {{ loading ? 'Searching…' : 'Search' }}
        </button>
      </div>
    </form>

    <p v-if="error" class="error">{{ error }}</p>

    <p v-if="searched && !results.length" class="muted">No results found.</p>

    <div v-for="(result, i) in results" :key="result.chunk_id" class="result-card">
      <div class="result-header">
        <span class="result-num">#{{ i + 1 }}</span>
        <span class="result-file">{{ result.filename }}</span>
        <span class="result-score">{{ (result.score * 100).toFixed(1) }}% match</span>
      </div>
      <p class="result-content">{{ result.content }}</p>
      <div class="result-meta">chunk {{ result.chunk_index }} · doc {{ result.document_id }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { search } from '../api.js'

const query = ref('')
const limit = ref(5)
const results = ref([])
const loading = ref(false)
const error = ref('')
const searched = ref(false)

async function doSearch() {
  if (!query.value.trim()) return
  loading.value = true
  error.value = ''
  searched.value = false
  try {
    const data = await search(query.value, limit.value)
    results.value = data.results
    searched.value = true
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.search-view { background: #fff; border-radius: 8px; padding: 1.5rem; box-shadow: 0 1px 4px rgba(0,0,0,0.08); }
h2 { margin-bottom: 1.25rem; font-size: 1.1rem; }

.search-form { display: flex; flex-direction: column; gap: 0.75rem; margin-bottom: 1.5rem; }

.search-form input[type="text"] {
  width: 100%;
  padding: 0.65rem 0.85rem;
  border: 1px solid #ddd;
  border-radius: 6px;
  font-size: 1rem;
}

.search-form input[type="text"]:focus { outline: none; border-color: #4f46e5; }

.search-controls { display: flex; align-items: center; gap: 1rem; }

.search-controls label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.875rem;
  color: #555;
}

.search-controls input[type="number"] {
  width: 60px;
  padding: 0.3rem 0.4rem;
  border: 1px solid #ddd;
  border-radius: 4px;
}

.search-controls button {
  background: #4f46e5;
  color: #fff;
  border: none;
  border-radius: 6px;
  padding: 0.55rem 1.25rem;
  cursor: pointer;
  font-size: 0.9rem;
  transition: background 0.15s;
}

.search-controls button:hover:not(:disabled) { background: #4338ca; }
.search-controls button:disabled { opacity: 0.6; cursor: default; }

.result-card {
  border: 1px solid #eee;
  border-radius: 6px;
  padding: 1rem;
  margin-bottom: 0.75rem;
  transition: box-shadow 0.15s;
}

.result-card:hover { box-shadow: 0 2px 8px rgba(0,0,0,0.08); }

.result-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.5rem;
  font-size: 0.8rem;
}

.result-num { color: #888; }
.result-file { font-family: monospace; font-weight: 600; color: #4f46e5; }
.result-score { margin-left: auto; background: #f0fdf4; color: #16a34a; padding: 0.1rem 0.5rem; border-radius: 4px; font-weight: 600; }

.result-content { font-size: 0.875rem; line-height: 1.6; color: #333; white-space: pre-wrap; }

.result-meta { margin-top: 0.5rem; font-size: 0.75rem; color: #aaa; font-family: monospace; }

.muted { color: #888; font-size: 0.9rem; }
.error { color: #e53e3e; font-size: 0.875rem; }
</style>
