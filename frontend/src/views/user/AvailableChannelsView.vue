<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="relative w-full sm:w-80">
            <Icon
              name="search"
              size="md"
              class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('availableChannels.searchPlaceholder')"
              class="input pl-10"
            />
          </div>

          <div class="flex justify-end">
            <button
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('common.refresh')"
              @click="loadChannels"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <AvailableChannelsTable
          :columns="columnLabels"
          :rows="filteredChannels"
          :loading="loading"
          :user-group-rates="userGroupRates"
          pricing-key-prefix="availableChannels.pricing"
          :no-pricing-label="t('availableChannels.noPricing')"
          :no-models-label="t('availableChannels.noModels')"
          :empty-label="t('availableChannels.empty')"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserAvailableChannel } from '@/api/channels'
import userChannelsAPI from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import AvailableChannelsTable from '@/components/channels/AvailableChannelsTable.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const loading = ref(false)
const searchQuery = ref('')

const columnLabels = computed(() => ({
  name: t('availableChannels.columns.name'),
  description: t('availableChannels.columns.description'),
  platform: t('availableChannels.columns.platform'),
  groups: t('availableChannels.columns.groups'),
  supportedModels: t('availableChannels.columns.supportedModels')
}))

const filteredChannels = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) {
    return channels.value
  }

  return channels.value
    .map(channel => {
      const nameHit = channel.name.toLowerCase().includes(query)
      const descriptionHit = (channel.description || '').toLowerCase().includes(query)
      if (nameHit || descriptionHit) {
        return channel
      }

      const matchingPlatforms = channel.platforms.filter(section =>
        section.platform.toLowerCase().includes(query) ||
        section.groups.some(group => group.name.toLowerCase().includes(query)) ||
        section.supported_models.some(model => model.name.toLowerCase().includes(query))
      )

      if (matchingPlatforms.length === 0) {
        return null
      }

      return {
        ...channel,
        platforms: matchingPlatforms
      }
    })
    .filter((channel): channel is UserAvailableChannel => channel !== null)
})

function extractApiErrorMessage(error: unknown, fallback: string): string {
  if (!error || typeof error !== 'object') {
    return fallback
  }

  const apiError = error as {
    message?: string
    response?: {
      data?: {
        message?: string
        error?: string
        detail?: string
      }
    }
  }

  return (
    apiError.response?.data?.message ||
    apiError.response?.data?.error ||
    apiError.response?.data?.detail ||
    apiError.message ||
    fallback
  )
}

async function loadChannels() {
  loading.value = true
  try {
    const [channelList, rates] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch(() => ({} as Record<number, number>))
    ])
    channels.value = channelList
    userGroupRates.value = rates
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadChannels)
</script>
