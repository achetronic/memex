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
