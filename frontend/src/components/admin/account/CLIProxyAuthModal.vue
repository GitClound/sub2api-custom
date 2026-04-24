<template>
  <BaseDialog :show="show" :title="t('admin.accounts.cliproxyAuthTitle')" width="wide" @close="handleClose">
    <div class="space-y-5">
      <div class="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-blue-800 dark:border-blue-800 dark:bg-blue-950/30 dark:text-blue-200">
        {{ t('admin.accounts.cliproxyAuthHint') }}
      </div>

      <div class="grid gap-3 md:grid-cols-2">
        <button class="btn btn-secondary justify-center" type="button" :disabled="busy" @click="openFilePicker">
          {{ t('admin.accounts.cliproxyAuthChooseFiles') }}
        </button>
        <button class="btn btn-secondary justify-center" type="button" :disabled="busy" @click="handleExport">
          {{ t('admin.accounts.cliproxyAuthExport') }}
        </button>
      </div>

      <input ref="fileInput" type="file" class="hidden" accept="application/json,.json" multiple @change="handleFileChange" />

      <div v-if="files.length" class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <div class="mb-2 text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.cliproxyAuthSelected', { count: files.length }) }}
        </div>
        <div class="max-h-36 space-y-1 overflow-auto text-xs text-gray-600 dark:text-dark-300">
          <div v-for="file in files" :key="file.name" class="truncate">{{ file.name }}</div>
        </div>
      </div>

      <div class="grid gap-4 md:grid-cols-2">
        <label class="space-y-1 text-sm">
          <span class="text-gray-700 dark:text-dark-300">{{ t('admin.accounts.concurrency') }}</span>
          <input v-model.number="concurrency" type="number" min="1" class="input" />
        </label>
        <label class="space-y-1 text-sm">
          <span class="text-gray-700 dark:text-dark-300">{{ t('admin.accounts.priority') }}</span>
          <input v-model.number="priority" type="number" min="1" class="input" />
        </label>
      </div>

      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input v-model="skipDefaultGroupBind" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
        <span>{{ t('admin.accounts.cliproxyAuthSkipDefaultGroup') }}</span>
      </label>

      <div v-if="result" class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.dataImportResultSummary', result) }}
        </div>
        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.dataImportErrors') }}
          </div>
          <div class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800">
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} - {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="busy" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button class="btn btn-primary" type="button" :disabled="busy || !files.length" @click="handleImport">
          {{ busy ? t('admin.accounts.dataImporting') : t('admin.accounts.cliproxyAuthImport') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminDataImportResult, CLIProxyAuthFile } from '@/types'

interface Props {
  show: boolean
  selectedIds?: number[]
  filters?: {
    platform?: string
    type?: string
    status?: string
    search?: string
  }
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = withDefaults(defineProps<Props>(), {
  selectedIds: () => [],
  filters: () => ({})
})
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const busy = ref(false)
const files = ref<File[]>([])
const result = ref<AdminDataImportResult | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)
const concurrency = ref(3)
const priority = ref(50)
const skipDefaultGroupBind = ref(true)

const errorItems = computed(() => result.value?.errors || [])

watch(
  () => props.show,
  (open) => {
    if (open) {
      files.value = []
      result.value = null
      concurrency.value = 3
      priority.value = 50
      skipDefaultGroupBind.value = true
      if (fileInput.value) fileInput.value.value = ''
    }
  }
)

const handleClose = () => {
  if (busy.value) return
  emit('close')
}

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  files.value = Array.from(target.files || [])
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') return sourceFile.text()
  const buffer = await sourceFile.arrayBuffer()
  return new TextDecoder().decode(buffer)
}

const loadAuthFiles = async (): Promise<CLIProxyAuthFile[]> => {
  const loaded: CLIProxyAuthFile[] = []
  for (const file of files.value) {
    const text = await readFileAsText(file)
    const parsed = JSON.parse(text)
    if (parsed && parsed.type === 'cliproxy-auth-bundle' && Array.isArray(parsed.files)) {
      loaded.push({ name: file.name, data: parsed })
    } else {
      loaded.push({ name: file.name, data: parsed })
    }
  }
  return loaded
}

const handleImport = async () => {
  if (!files.value.length) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  busy.value = true
  try {
    const authFiles = await loadAuthFiles()
    const res = await adminAPI.accounts.importCLIProxyAuths({
      files: authFiles,
      skip_default_group_bind: skipDefaultGroupBind.value,
      concurrency: concurrency.value,
      priority: priority.value
    })
    result.value = res
    if (res.account_failed > 0 || res.proxy_failed > 0) {
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', importMessageParams(res)))
    } else {
      appStore.showSuccess(t('admin.accounts.cliproxyAuthImportSuccess', { count: res.account_created }))
      emit('imported')
    }
  } catch (error: any) {
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.accounts.dataImportParseFailed'))
    } else {
      appStore.showError(error?.message || t('admin.accounts.cliproxyAuthImportFailed'))
    }
  } finally {
    busy.value = false
  }
}

const importMessageParams = (res: AdminDataImportResult): Record<string, unknown> => ({
  account_created: res.account_created,
  account_failed: res.account_failed,
  proxy_created: res.proxy_created,
  proxy_reused: res.proxy_reused,
  proxy_failed: res.proxy_failed
})

const handleExport = async () => {
  if (busy.value) return
  busy.value = true
  try {
    const payload = await adminAPI.accounts.exportCLIProxyAuths(
      props.selectedIds.length > 0
        ? { ids: props.selectedIds }
        : { filters: props.filters }
    )
    const timestamp = new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d{3}Z$/, 'Z')
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `cliproxy-auths-${timestamp}.json`
    link.click()
    URL.revokeObjectURL(url)
    appStore.showSuccess(t('admin.accounts.cliproxyAuthExported', { count: payload.files.length }))
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.cliproxyAuthExportFailed'))
  } finally {
    busy.value = false
  }
}
</script>
