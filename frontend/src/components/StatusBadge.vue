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
  <span class="status-badge" :class="status">{{ status }}</span>
</template>

<script setup>
defineProps({ status: String })
</script>

<style scoped>
.status-badge {
  display: inline-block;
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: capitalize;
}

.pending    { background: #fef9c3; color: #854d0e; }
.processing { background: #dbeafe; color: #1d4ed8; }
.completed  { background: #dcfce7; color: #166534; }
.failed     { background: #fee2e2; color: #991b1b; }
</style>
