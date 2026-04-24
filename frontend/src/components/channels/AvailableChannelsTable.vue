<template>
  <div class="card overflow-hidden">
    <table class="w-full table-fixed border-collapse text-sm">
      <thead>
        <tr class="border-b border-gray-100 bg-gray-50/50 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/50 dark:text-gray-400">
          <th class="w-[180px] px-4 py-3 text-center">{{ columns.name }}</th>
          <th class="w-[220px] px-4 py-3 text-left">{{ columns.description }}</th>
          <th class="w-[140px] px-4 py-3 text-left">{{ columns.platform }}</th>
          <th class="px-4 py-3 text-left">{{ columns.groups }}</th>
          <th class="px-4 py-3 text-left">{{ columns.supportedModels }}</th>
        </tr>
      </thead>

      <tbody v-if="loading">
        <tr>
          <td colspan="5" class="py-12 text-center">
            <Icon name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
          </td>
        </tr>
      </tbody>

      <tbody v-else-if="rows.length === 0">
        <tr>
          <td colspan="5" class="py-12 text-center">
            <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-400" />
            <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
          </td>
        </tr>
      </tbody>

      <tbody
        v-for="(channel, channelIndex) in rows"
        v-else
        :key="`${channel.name}-${channelIndex}`"
        class="border-b-2 border-gray-200 last:border-b-0 dark:border-dark-600"
      >
        <tr
          v-for="(section, sectionIndex) in channel.platforms"
          :key="`${channel.name}-${section.platform}`"
          class="transition-colors hover:bg-gray-50/40 dark:hover:bg-dark-800/40"
          :class="{ 'border-t border-gray-100/70 dark:border-dark-700/50': sectionIndex > 0 }"
        >
          <td
            v-if="sectionIndex === 0"
            :rowspan="channel.platforms.length"
            class="px-4 py-3 text-center align-middle font-medium text-gray-900 dark:text-white"
          >
            {{ channel.name }}
          </td>

          <td
            v-if="sectionIndex === 0"
            :rowspan="channel.platforms.length"
            class="px-4 py-3 align-middle text-xs text-gray-500 dark:text-gray-400"
          >
            <template v-if="channel.description">{{ channel.description }}</template>
            <span v-else>-</span>
          </td>

          <td class="align-top px-4 py-3">
            <span
              :class="[
                'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase',
                platformBadgeClass(section.platform)
              ]"
            >
              <PlatformIcon :platform="section.platform as GroupPlatform" size="xs" />
              {{ section.platform }}
            </span>
          </td>

          <td class="align-top px-4 py-3">
            <div class="flex flex-col gap-1.5">
              <div
                v-if="exclusiveGroups(section).length > 0"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  class="inline-flex items-center gap-0.5 text-[10px] font-medium uppercase text-purple-600 dark:text-purple-400"
                  :title="t('availableChannels.exclusiveTooltip')"
                >
                  <Icon name="shield" size="xs" class="h-3 w-3" />
                  {{ t('availableChannels.exclusive') }}
                </span>
                <GroupBadge
                  v-for="group in exclusiveGroups(section)"
                  :key="`exclusive-${group.id}`"
                  :name="group.name"
                  :platform="group.platform as GroupPlatform"
                  :subscription-type="group.subscription_type as SubscriptionType"
                  :rate-multiplier="group.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[group.id] ?? null"
                />
              </div>

              <div
                v-if="publicGroups(section).length > 0"
                class="flex flex-wrap items-center gap-1.5"
              >
                <span
                  class="inline-flex items-center gap-0.5 text-[10px] font-medium uppercase text-gray-500 dark:text-gray-400"
                  :title="t('availableChannels.publicTooltip')"
                >
                  <Icon name="globe" size="xs" class="h-3 w-3" />
                  {{ t('availableChannels.public') }}
                </span>
                <GroupBadge
                  v-for="group in publicGroups(section)"
                  :key="`public-${group.id}`"
                  :name="group.name"
                  :platform="group.platform as GroupPlatform"
                  :subscription-type="group.subscription_type as SubscriptionType"
                  :rate-multiplier="group.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[group.id] ?? null"
                />
              </div>

              <span v-if="section.groups.length === 0" class="text-xs text-gray-400">-</span>
            </div>
          </td>

          <td class="align-top px-4 py-3">
            <div class="flex flex-wrap gap-1">
              <SupportedModelChip
                v-for="model in section.supported_models"
                :key="`${section.platform}-${model.name}`"
                :model="model"
                :pricing-key-prefix="pricingKeyPrefix"
                :no-pricing-label="noPricingLabel"
                :show-platform="false"
                :platform-hint="section.platform"
              />
              <span v-if="section.supported_models.length === 0" class="text-xs text-gray-400">
                {{ noModelsLabel }}
              </span>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UserAvailableChannel, UserAvailableGroup, UserChannelPlatformSection } from '@/api/channels'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import type { GroupPlatform, SubscriptionType } from '@/types'
import SupportedModelChip from './SupportedModelChip.vue'

defineProps<{
  columns: {
    name: string
    description: string
    platform: string
    groups: string
    supportedModels: string
  }
  rows: UserAvailableChannel[]
  loading: boolean
  pricingKeyPrefix: string
  noPricingLabel: string
  noModelsLabel: string
  emptyLabel: string
  userGroupRates: Record<number, number>
}>()

const { t } = useI18n()

function platformBadgeClass(platform: string): string {
  switch (platform) {
    case 'anthropic':
      return 'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-700 dark:bg-orange-900/20 dark:text-orange-300'
    case 'openai':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
    case 'gemini':
      return 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-700 dark:bg-blue-900/20 dark:text-blue-300'
    case 'antigravity':
      return 'border-violet-200 bg-violet-50 text-violet-700 dark:border-violet-700 dark:bg-violet-900/20 dark:text-violet-300'
    default:
      return 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300'
  }
}

function exclusiveGroups(section: UserChannelPlatformSection): UserAvailableGroup[] {
  return section.groups.filter(group => group.is_exclusive)
}

function publicGroups(section: UserChannelPlatformSection): UserAvailableGroup[] {
  return section.groups.filter(group => !group.is_exclusive)
}
</script>
