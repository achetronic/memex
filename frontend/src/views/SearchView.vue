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
  <div class="search-view">

    <section class="card search-card">
      <div class="card-header">
        <h2>Semantic Search</h2>
      </div>

      <form class="search-form" @submit.prevent="doSearch">
        <div class="search-row">
          <input
            v-model="query"
            type="text"
            class="search-input"
            placeholder="Ask anything about your documents…"
            :disabled="loading"
          />
          <div class="search-limit">
            <label>Top</label>
            <input v-model.number="limit" type="number" min="1" max="20" class="limit-input" />
          </div>
          <button type="submit" class="btn-search" :disabled="loading || !query.trim()">
            <svg v-if="!loading" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <circle cx="11" cy="11" r="8"/>
              <path d="M21 21l-4.35-4.35" stroke-linecap="round"/>
            </svg>
            <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="spin">
              <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4" stroke-linecap="round"/>
            </svg>
            {{ loading ? 'Searching…' : 'Search' }}
          </button>
        </div>
      </form>

      <div v-if="error" class="message message-error">{{ error }}</div>
    </section>

    <!-- ── Results ─────────────────────────────────────────────────────────── -->
    <p v-if="searched && !results.length" class="no-results">
      No results found for that query.
    </p>

    <div v-for="(result, i) in results" :key="result.chunk_id" class="result-card">
      <div class="result-header">
        <span class="result-num">#{{ i + 1 }}</span>
        <span class="result-file">{{ result.filename }}</span>
        <span class="result-score" :class="scoreClass(result.score)">
          {{ (result.score * 100).toFixed(1) }}%
        </span>
      </div>
      <p class="result-content">{{ result.content }}</p>
      <div class="result-meta">chunk {{ result.chunk_index }} · {{ result.document_id }}</div>
    </div>

  </div>
</template>

<script setup>
import { ref, watch, inject } from 'vue'
import { search } from '../api.js'

const emit = defineEmits(['unauthorized'])

const namespace = inject('namespace')

const query    = ref('')
const limit    = ref(5)
const results  = ref([])
const loading  = ref(false)
const error    = ref('')
const searched = ref(false)

// Execute a semantic search and display ranked results.
async function doSearch() {
  if (!query.value.trim()) return
  loading.value  = true
  error.value    = ''
  searched.value = false
  try {
    const data     = await search(query.value, limit.value)
    results.value  = data.results
    searched.value = true
  } catch (e) {
    if (e.status === 401) emit('unauthorized')
    else error.value = e.message
  } finally {
    loading.value = false
  }
}

// Map a similarity score to a CSS modifier class for colour-coded display.
function scoreClass(score) {
  if (score >= 0.8) return 'score-high'
  if (score >= 0.5) return 'score-mid'
  return 'score-low'
}
// Clear results when the user switches namespace (injected from App.vue).
watch(namespace, () => { results.value = []; searched.value = false; error.value = '' })
</script>

<style scoped>
.search-view { display: flex; flex-direction: column; gap: 1rem; }

/* ── Card ─────────────────────────────────────────────────────────────────── */
.card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
  overflow: hidden;
}

.card-header {
  display: flex; align-items: center;
  padding: 1.1rem 1.5rem 0.9rem;
  border-bottom: 1px solid var(--border);
}

.card-header h2 {
  font-size: 0.95rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.07em;
  color: var(--ink-muted);
}

/* ── Search form ──────────────────────────────────────────────────────────── */
.search-form { padding: 1.25rem 1.5rem; }

.search-row {
  display: flex; align-items: center; gap: 0.75rem; flex-wrap: wrap;
}

@media (max-width: 600px) {
  .search-form { padding: 1rem; }
  .search-input { min-width: 0; }
  .btn-search { width: 100%; justify-content: center; }
  .card-header { padding: 0.9rem 1rem 0.75rem; }
}

.search-input {
  flex: 1;
  border: 1.5px solid var(--border);
  border-radius: 8px;
  padding: 0.65rem 0.9rem;
  font-size: 0.95rem;
  color: var(--ink);
  background: var(--pink-bg);
  transition: border-color 0.15s, background 0.15s;
}
.search-input:focus {
  outline: none;
  border-color: var(--pink-hot);
  background: #fff;
}
.search-input:disabled { opacity: 0.6; }

.search-limit {
  display: flex; align-items: center; gap: 0.35rem;
  font-size: 0.8rem;
  color: var(--ink-muted);
  white-space: nowrap;
}

.limit-input {
  width: 52px;
  border: 1.5px solid var(--border);
  border-radius: 6px;
  padding: 0.35rem 0.4rem;
  font-size: 0.85rem;
  text-align: center;
  color: var(--ink);
}

.btn-search {
  display: flex; align-items: center; gap: 0.4rem;
  background: var(--pink-hot);
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: 0.65rem 1.15rem;
  font-size: 0.9rem;
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s;
}
.btn-search:hover:not(:disabled) { background: var(--pink-hot-dark); }
.btn-search:disabled { opacity: 0.55; cursor: default; }
.btn-search svg { width: 16px; height: 16px; }
.btn-search .spin { animation: spin 1s linear infinite; }

@keyframes spin { to { transform: rotate(360deg); } }

.message {
  margin: 0 1.5rem 1.25rem;
  padding: 0.65rem 0.9rem;
  border-radius: 6px;
  font-size: 0.85rem;
  font-weight: 500;
}
.message-error { background: #fff1f1; color: var(--red); border: 1px solid #fcc; }

/* ── Results ──────────────────────────────────────────────────────────────── */
.no-results {
  text-align: center;
  padding: 2rem;
  color: var(--ink-muted);
  font-size: 0.9rem;
}

.result-card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 1.1rem 1.25rem;
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.15s, border-color 0.15s;
}

.result-card:hover {
  box-shadow: var(--shadow-md);
  border-color: #d8c8ce;
}

.result-header {
  display: flex; align-items: center; gap: 0.65rem;
  margin-bottom: 0.65rem;
}

.result-num {
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--ink-muted);
  background: var(--pink-mid);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 0.1rem 0.4rem;
}

.result-file {
  font-family: monospace;
  font-size: 0.82rem;
  font-weight: 700;
  color: var(--pink-hot);
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.result-score {
  font-size: 0.78rem;
  font-weight: 700;
  border-radius: 20px;
  padding: 0.15rem 0.55rem;
}
.score-high { background: #f0fdf4; color: #16a34a; border: 1px solid #bbf7d0; }
.score-mid  { background: #fffbeb; color: #b45309; border: 1px solid #fde68a; }
.score-low  { background: #fff1f1; color: var(--red); border: 1px solid #fcc; }

.result-content {
  font-size: 0.875rem;
  line-height: 1.7;
  color: var(--ink);
  white-space: pre-wrap;
}

.result-meta {
  margin-top: 0.65rem;
  font-size: 0.72rem;
  color: var(--ink-muted);
  font-family: monospace;
}
</style>
