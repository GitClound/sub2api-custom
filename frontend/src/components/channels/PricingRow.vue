<template>
  <div class="flex items-center justify-between gap-3">
    <span class="text-gray-500 dark:text-gray-400">{{ label }}</span>
    <span class="font-mono text-gray-900 dark:text-white">{{ display }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  value: number | null
  unit: string
  scale: number
}>(), {
  value: null
})

function formatScaled(value: number, scale: number): string {
  const scaled = value * scale
  const small = Math.abs(scaled) > 0 && Math.abs(scaled) < 1
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: small ? 6 : 0,
    maximumFractionDigits: small ? 6 : 4
  }).format(scaled)
}

const display = computed(() => {
  if (props.value == null) {
    return '-'
  }
  return `${formatScaled(props.value, props.scale)} ${props.unit}`
})
</script>
