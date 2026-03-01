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
  <span class="status-badge" :class="status">
    <span class="status-dot"></span>{{ status }}
  </span>
</template>

<script setup>
defineProps({ status: String })
</script>

<style scoped>
.status-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0.2rem 0.55rem;
  border-radius: 20px;
  font-size: 0.72rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.status-dot {
  width: 6px; height: 6px;
  border-radius: 50%;
  flex-shrink: 0;
}

.pending    { background: #fffbeb; color: #b45309; border: 1px solid #fde68a; }
.pending .status-dot    { background: #f9e04b; }

.processing { background: #fff0f3; color: var(--pink-hot); border: 1px solid #f4c0cc; }
.processing .status-dot { background: var(--pink-hot); animation: pulse 1.2s ease-in-out infinite; }

.completed  { background: #f0fdf4; color: #16a34a; border: 1px solid #bbf7d0; }
.completed .status-dot  { background: #4caf50; }

.failed     { background: #fff1f1; color: #991b1b; border: 1px solid #fcc; }
.failed .status-dot     { background: #e8534a; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50%       { opacity: 0.35; }
}
</style>
