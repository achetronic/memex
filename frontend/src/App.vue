<template>
  <div class="app">
    <header class="header">
      <h1>memex</h1>
      <nav>
        <button :class="{ active: view === 'documents' }" @click="view = 'documents'">Documents</button>
        <button :class="{ active: view === 'search' }" @click="view = 'search'">Search</button>
      </nav>
    </header>

    <main class="main">
      <DocumentsView v-if="view === 'documents'" />
      <SearchView v-else />
    </main>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import DocumentsView from './views/DocumentsView.vue'
import SearchView from './views/SearchView.vue'

const view = ref('documents')
</script>

<style>
* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #f5f5f5;
  color: #222;
}

.app { min-height: 100vh; display: flex; flex-direction: column; }

.header {
  background: #1a1a2e;
  color: #fff;
  padding: 1rem 2rem;
  display: flex;
  align-items: center;
  gap: 2rem;
}

.header h1 { font-size: 1.4rem; letter-spacing: 0.05em; }

nav { display: flex; gap: 0.5rem; }

nav button {
  background: transparent;
  border: 1px solid rgba(255,255,255,0.3);
  color: #fff;
  padding: 0.4rem 1rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9rem;
  transition: background 0.15s;
}

nav button.active,
nav button:hover { background: rgba(255,255,255,0.15); }

.main { flex: 1; padding: 2rem; max-width: 960px; margin: 0 auto; width: 100%; }
</style>
