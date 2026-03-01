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
  <div class="app">

    <!-- ── Auth overlay ───────────────────────────────────────────────────── -->
    <div v-if="needsAuth" class="auth-overlay">
      <div class="auth-hero">
        <img src="./assets/logo.svg" alt="memex" class="auth-hero-logo" />
        <div class="auth-hero-name">memex</div>
        <div class="auth-hero-sub">your knowledge, elephant powered</div>
      </div>
      <div class="auth-dialog">
        <input
          v-model="apiKeyInput"
          type="password"
          placeholder="Paste your API key…"
          @keydown.enter="saveAuth"
          class="auth-input"
          autofocus
        />
        <p v-if="authError" class="auth-error">{{ authError }}</p>
        <button class="btn-primary" @click="saveAuth" :disabled="!apiKeyInput.trim()">
          Continue →
        </button>
      </div>
    </div>

    <!-- ── App shell ──────────────────────────────────────────────────────── -->
    <template v-else>
      <header class="header">
        <div class="header-brand">
          <img src="./assets/logo.svg" alt="memex logo" class="header-logo" />
          <div class="header-text">
            <span class="header-title">memex</span>
            <span class="header-sub">your knowledge, elephant powered</span>
          </div>
        </div>

        <div class="header-controls">

          <!-- Namespace selector — custom dropdown -->
          <div v-if="namespaces.length > 1" class="ns-dropdown" v-click-outside="closeNs">
            <button class="ns-trigger" @click="nsOpen = !nsOpen" :class="{ open: nsOpen }">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="ns-icon">
                <ellipse cx="12" cy="5" rx="9" ry="3"/>
                <path d="M3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5" stroke-linecap="round"/>
                <path d="M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3" stroke-linecap="round"/>
              </svg>
              <span class="ns-current">{{ currentNamespace }}</span>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="ns-chevron" :class="{ rotated: nsOpen }">
                <path d="M6 9l6 6 6-6" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </button>
            <transition name="ns-drop">
              <ul v-if="nsOpen" class="ns-panel">
                <li
                  v-for="ns in namespaces"
                  :key="ns"
                  class="ns-option"
                  :class="{ selected: ns === currentNamespace }"
                  @click="selectNs(ns)"
                >
                  {{ ns }}
                  <svg v-if="ns === currentNamespace" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="ns-check">
                    <path d="M5 13l4 4L19 7" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </li>
              </ul>
            </transition>
          </div>

          <!-- View toggle pill -->
          <div class="view-toggle">
            <div class="toggle-track" :class="view">
              <div class="toggle-pill"></div>
              <button class="toggle-opt" :class="{ active: view === 'documents' }" @click="view = 'documents'" title="Documents">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" stroke-linecap="round" stroke-linejoin="round"/>
                  <path d="M14 2v6h6M16 13H8M16 17H8" stroke-linecap="round"/>
                </svg>
                <span>Docs</span>
              </button>
              <button class="toggle-opt" :class="{ active: view === 'search' }" @click="view = 'search'" title="Search">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="11" cy="11" r="8"/><path d="M21 21l-4.35-4.35" stroke-linecap="round"/>
                </svg>
                <span>Search</span>
              </button>
            </div>
          </div>

          <button v-if="serverConfig?.auth_enabled" class="btn-signout" @click="signOut" title="Sign out">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9" stroke-linecap="round" stroke-linejoin="round"/>
            </svg>
          </button>
        </div>
      </header>

      <main class="main">
        <DocumentsView v-if="view === 'documents'" @unauthorized="onUnauthorized" />
        <SearchView    v-else                       @unauthorized="onUnauthorized" />
      </main>
    </template>

  </div>
</template>

<script setup>
import { ref, computed, onMounted, provide } from 'vue'
import DocumentsView from './views/DocumentsView.vue'
import SearchView    from './views/SearchView.vue'
import { getServerInfo, getSession, setSession, clearSession } from './api.js'

// Directiva v-click-outside: cierra el dropdown al clicar fuera.
const vClickOutside = {
  mounted(el, binding) {
    el._clickOutside = (e) => { if (!el.contains(e.target)) binding.value(e) }
    document.addEventListener('pointerdown', el._clickOutside)
  },
  unmounted(el) {
    document.removeEventListener('pointerdown', el._clickOutside)
  },
}

const view             = ref('documents')
const serverConfig     = ref(null)
const namespaces       = ref([])
const currentNamespace = ref('')
const nsOpen           = ref(false)
const apiKeyInput      = ref('')
const authError        = ref('')
// storedApiKey mirrors sessionStorage so needsAuth computed stays reactive.
const storedApiKey     = ref(getSession().apiKey)

// Provide namespace as a reactive ref so child views can inject and watch it.
provide('namespace', currentNamespace)

// True when auth is required but no key has been validated yet.
const needsAuth = computed(() => {
  if (!serverConfig.value) return false
  return serverConfig.value.auth_enabled && !storedApiKey.value
})

// Bootstrap: fetch server config and restore the last active namespace.
// On 401 the server has auth enabled but no key is stored yet — show the login screen.
// Any other error means the server is unreachable; child views handle that themselves.
onMounted(async () => {
  try {
    const info         = await getServerInfo()
    serverConfig.value = info
    namespaces.value   = info.namespaces || []

    const { namespace } = getSession()
    if (namespace && namespaces.value.includes(namespace)) {
      currentNamespace.value = namespace
    } else if (namespaces.value.length) {
      currentNamespace.value = namespaces.value[0]
      setSession({ namespace: currentNamespace.value, apiKey: getSession().apiKey })
    }
  } catch (err) {
    if (err.status === 401) serverConfig.value = { auth_enabled: true, namespaces: [] }
  }
})

// Write to sessionStorage first so authHeaders() is already correct when
// the reactive ref change triggers watchers in child views.
function selectNs(ns) {
  setSession({ namespace: ns, apiKey: getSession().apiKey })
  currentNamespace.value = ns
  nsOpen.value = false
}

// Used by v-click-outside to close the namespace dropdown.
function closeNs() {
  nsOpen.value = false
}

// Validate the entered key against /info before persisting it.
// On success, populate namespaces and enter the app.
// On 401, reject the key and show an error without storing anything.
async function saveAuth() {
  const key = apiKeyInput.value.trim()
  if (!key) return
  authError.value = ''
  setSession({ namespace: '', apiKey: key })
  try {
    const info         = await getServerInfo()
    serverConfig.value = info
    namespaces.value   = info.namespaces || []
    const first        = namespaces.value[0] ?? ''
    currentNamespace.value = first
    setSession({ namespace: first, apiKey: key })
    storedApiKey.value = key
    apiKeyInput.value  = ''
  } catch (err) {
    clearSession()
    authError.value = err.status === 401
      ? 'Invalid API key. Please try again.'
      : 'Could not reach the server.'
  }
}

// Clear all session state and return to the login screen.
function signOut() {
  clearSession()
  storedApiKey.value = ''
  apiKeyInput.value  = ''
  authError.value    = ''
}

// Called when a child view gets a 401 — treat it the same as a sign-out.
function onUnauthorized() {
  clearSession()
  storedApiKey.value = ''
  authError.value    = ''
}
</script>

<style>
/* ── Reset & base ─────────────────────────────────────────────────────────── */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #f7f0f2;
  color: #1a1218;
  min-height: 100vh;
}

/* ── Design tokens ────────────────────────────────────────────────────────── */
:root {
  --pink-bg:    #fff0f3;
  --pink-mid:   #f7e8ec;
  --pink-hot:   #e05c7a;
  --pink-hot-dark: #c44d68;
  --yellow:     #f9e04b;
  --red:        #e8534a;
  --green:      #4caf50;
  --ink:        #2d1f2a;
  --ink-muted:  #7a6a72;
  --card-bg:    #ffffff;
  --border:     #ecdde3;
  --shadow-sm:  0 1px 3px rgba(45,31,42,0.08);
  --shadow-md:  0 4px 16px rgba(45,31,42,0.10);
  --radius:     10px;
}

.app { min-height: 100vh; display: flex; flex-direction: column; }

/* ── Auth overlay ─────────────────────────────────────────────────────────── */
.auth-overlay {
  position: fixed; inset: 0;
  background: var(--pink-bg);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2.5rem;
  z-index: 100;
}

.auth-hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.75rem;
  text-align: center;
}

.auth-hero-logo {
  width: 200px;
  height: 200px;
  filter: drop-shadow(0 8px 32px rgba(224,92,122,0.20));
  animation: float 4s ease-in-out infinite;
}

@keyframes float {
  0%, 100% { transform: translateY(0);    }
  50%       { transform: translateY(-8px); }
}

.auth-hero-name {
  font-size: 2.5rem;
  font-weight: 800;
  letter-spacing: -0.04em;
  color: var(--ink);
  line-height: 1;
}

.auth-hero-sub {
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--ink-muted);
}

.auth-dialog {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: 14px;
  padding: 1.5rem;
  width: 320px;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  box-shadow: var(--shadow-md);
}

.auth-error { color: var(--red); font-size: 0.82rem; }

.auth-input {
  background: var(--card-bg);
  border: 1.5px solid var(--border);
  border-radius: 8px;
  padding: 0.6rem 0.85rem;
  font-size: 0.95rem;
  width: 100%;
  transition: border-color 0.15s;
}
.auth-input:focus { outline: none; border-color: var(--pink-hot); }

/* ── Buttons ──────────────────────────────────────────────────────────────── */
.btn-primary {
  background: var(--pink-hot);
  color: #fff;
  border: none;
  border-radius: 8px;
  padding: 0.6rem 1.25rem;
  font-size: 0.95rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s;
}
.btn-primary:hover:not(:disabled) { background: var(--pink-hot-dark); }
.btn-primary:disabled { opacity: 0.45; cursor: not-allowed; }

/* ── Header ───────────────────────────────────────────────────────────────── */
.header {
  background: var(--pink-bg);
  border-bottom: 1px solid var(--border);
  padding: 0.65rem 1rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  position: sticky; top: 0; z-index: 10;
  flex-wrap: wrap;
}

.header-brand {
  display: flex; align-items: center; gap: 0.65rem;
  text-decoration: none;
  flex-shrink: 0;
}

.header-logo { width: 36px; height: 36px; flex-shrink: 0; }

.header-text {
  display: flex; flex-direction: column; gap: 1px;
}

.header-title {
  font-size: 1.1rem;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--ink);
  line-height: 1;
}

.header-sub {
  font-size: 0.6rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--ink-muted);
  line-height: 1;
}

.header-controls {
  display: flex; align-items: center; gap: 0.5rem;
  flex-wrap: wrap;
}

@media (max-width: 600px) {
  .header { padding: 0.6rem 0.85rem; }
  .header-sub { display: none; }
  .ns-label { display: none; }
  .btn-signout span { display: none; }
}

/* ── Namespace dropdown ───────────────────────────────────────────────────── */
.ns-dropdown { position: relative; }

.ns-trigger {
  display: flex; align-items: center; gap: 0.4rem;
  background: var(--card-bg);
  border: 1.5px solid var(--border);
  border-radius: 8px;
  padding: 0.28rem 0.55rem;
  cursor: pointer;
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--ink);
  transition: border-color 0.15s, box-shadow 0.15s;
  white-space: nowrap;
}
.ns-trigger:hover,
.ns-trigger.open { border-color: var(--pink-hot); box-shadow: 0 0 0 3px rgba(224,92,122,0.12); }

.ns-icon  { width: 13px; height: 13px; color: var(--pink-hot); flex-shrink: 0; }
.ns-current { max-width: 120px; overflow: hidden; text-overflow: ellipsis; }
.ns-chevron {
  width: 13px; height: 13px; color: var(--ink-muted); flex-shrink: 0;
  transition: transform 0.2s;
}
.ns-chevron.rotated { transform: rotate(180deg); }

.ns-panel {
  position: absolute;
  top: calc(100% + 6px);
  left: 0;
  min-width: 100%;
  background: var(--card-bg);
  border: 1.5px solid var(--border);
  border-radius: 10px;
  box-shadow: var(--shadow-md);
  list-style: none;
  padding: 4px;
  z-index: 50;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ns-option {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0.45rem 0.7rem;
  border-radius: 7px;
  font-size: 0.82rem;
  font-weight: 500;
  color: var(--ink);
  cursor: pointer;
  transition: background 0.12s;
  white-space: nowrap;
  gap: 0.5rem;
}
.ns-option:hover    { background: var(--pink-mid); }
.ns-option.selected { background: var(--pink-bg); font-weight: 700; color: var(--pink-hot); }

.ns-check { width: 14px; height: 14px; color: var(--pink-hot); flex-shrink: 0; }

/* Dropdown open/close transition */
.ns-drop-enter-active { transition: opacity 0.15s, transform 0.15s; }
.ns-drop-leave-active  { transition: opacity 0.10s, transform 0.10s; }
.ns-drop-enter-from,
.ns-drop-leave-to      { opacity: 0; transform: translateY(-4px); }

/* ── View toggle pill ─────────────────────────────────────────────────────── */
.view-toggle { display: flex; align-items: center; }

.toggle-track {
  position: relative;
  display: flex;
  background: var(--pink-mid);
  border: 1.5px solid var(--border);
  border-radius: 10px;
  padding: 3px;
  gap: 2px;
}

.toggle-pill {
  position: absolute;
  top: 3px;
  left: 3px;
  width: calc(50% - 3px);
  height: calc(100% - 6px);
  background: var(--pink-hot);
  border-radius: 7px;
  transition: transform 0.22s cubic-bezier(.4,0,.2,1);
  box-shadow: 0 1px 4px rgba(224,92,122,0.30);
  pointer-events: none;
}

.toggle-track.search .toggle-pill {
  transform: translateX(100%);
}

.toggle-opt {
  position: relative;
  z-index: 1;
  background: transparent;
  border: none;
  cursor: pointer;
  display: flex; align-items: center; gap: 0.3rem;
  padding: 0.3rem 0.7rem;
  border-radius: 6px;
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--ink-muted);
  transition: color 0.18s;
  white-space: nowrap;
  flex: 1;
  justify-content: center;
}
.toggle-opt svg { width: 14px; height: 14px; flex-shrink: 0; }
.toggle-opt.active { color: #fff; }

/* ── Sign out ─────────────────────────────────────────────────────────────── */
.btn-signout {
  background: transparent;
  border: 1.5px solid var(--border);
  color: var(--ink-muted);
  padding: 0.35rem;
  border-radius: 6px;
  cursor: pointer;
  display: flex; align-items: center; justify-content: center;
  transition: border-color 0.15s, color 0.15s;
  flex-shrink: 0;
}
.btn-signout svg { width: 17px; height: 17px; }
.btn-signout:hover { border-color: var(--red); color: var(--red); }

@media (max-width: 600px) {
  .toggle-opt span { display: none; }
  .toggle-opt { padding: 0.3rem 0.5rem; }
}

/* ── Main ─────────────────────────────────────────────────────────────────── */
.main {
  flex: 1;
  padding: 1.5rem;
  max-width: 960px;
  margin: 0 auto;
  width: 100%;
}

@media (max-width: 600px) {
  .main { padding: 1rem 0.75rem; }
}
</style>
