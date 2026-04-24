<template>
  <div class="relative inline-block">
    <span
      ref="triggerEl"
      :class="[
        'inline-flex cursor-help items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium transition-colors',
        badgeClass
      ]"
      tabindex="0"
      @mouseenter="onEnter"
      @mouseleave="onLeave"
      @focusin="onEnter"
      @focusout="onLeave"
    >
      <PlatformIcon
        v-if="effectivePlatform"
        :platform="effectivePlatform as GroupPlatform"
        size="xs"
      />
      <span
        v-if="showPlatform && model.platform"
        class="rounded bg-black/5 px-1 text-[10px] uppercase dark:bg-white/10"
      >
        {{ model.platform }}
      </span>
      {{ model.name }}
    </span>

    <Teleport to="body">
      <div
        v-show="show"
        ref="popoverEl"
        role="tooltip"
        class="pointer-events-none fixed z-[99999] w-80 max-w-[min(22rem,calc(100vw-1rem))] rounded-lg border bg-white text-xs shadow-xl dark:bg-dark-800"
        :class="borderClass"
        :style="popoverStyle"
      >
        <div
          class="flex items-center justify-between gap-2 rounded-t-lg border-b px-3 py-2"
          :class="[headerClass, borderClass]"
        >
          <span class="truncate font-semibold">{{ model.name }}</span>
          <span
            v-if="model.platform"
            class="rounded bg-white/70 px-1.5 py-0.5 text-[10px] uppercase tracking-wide dark:bg-dark-900/60"
          >
            {{ model.platform }}
          </span>
        </div>

        <div class="space-y-2 p-3">
          <div v-if="!model.pricing" class="text-gray-500 dark:text-gray-400">
            {{ noPricingLabel }}
          </div>

          <template v-else>
            <div class="flex items-center justify-between gap-3">
              <span class="text-gray-500 dark:text-gray-400">{{ t(prefixKey('billingMode')) }}</span>
              <span class="text-gray-900 dark:text-white">{{ billingModeLabel }}</span>
            </div>

            <template v-if="model.pricing.billing_mode === BILLING_MODE_TOKEN">
              <PricingRow
                :label="t(prefixKey('inputPrice'))"
                :value="model.pricing.input_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="PER_MILLION_SCALE"
              />
              <PricingRow
                :label="t(prefixKey('outputPrice'))"
                :value="model.pricing.output_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="PER_MILLION_SCALE"
              />
              <PricingRow
                :label="t(prefixKey('cacheWritePrice'))"
                :value="model.pricing.cache_write_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="PER_MILLION_SCALE"
              />
              <PricingRow
                :label="t(prefixKey('cacheReadPrice'))"
                :value="model.pricing.cache_read_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="PER_MILLION_SCALE"
              />
              <PricingRow
                v-if="model.pricing.image_output_price != null && model.pricing.image_output_price > 0"
                :label="t(prefixKey('imageOutputPrice'))"
                :value="model.pricing.image_output_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="PER_MILLION_SCALE"
              />
            </template>

            <PricingRow
              v-if="model.pricing.billing_mode === BILLING_MODE_PER_REQUEST && model.pricing.per_request_price != null"
              :label="t(prefixKey('perRequestPrice'))"
              :value="model.pricing.per_request_price"
              :unit="t(prefixKey('unitPerRequest'))"
              :scale="1"
            />

            <PricingRow
              v-if="model.pricing.billing_mode === BILLING_MODE_IMAGE && model.pricing.image_output_price != null"
              :label="t(prefixKey('imageOutputPrice'))"
              :value="model.pricing.image_output_price"
              :unit="t(prefixKey('unitPerRequest'))"
              :scale="1"
            />

            <div
              v-if="model.pricing.intervals && model.pricing.intervals.length > 0"
              class="space-y-1 border-t pt-2"
              :class="borderClass"
            >
              <div class="font-medium text-gray-600 dark:text-gray-300">
                {{ t(prefixKey('intervals')) }}
              </div>
              <div
                v-for="(interval, index) in model.pricing.intervals"
                :key="index"
                class="flex items-center justify-between gap-3"
              >
                <span class="text-gray-500 dark:text-gray-400">
                  {{ interval.tier_label || formatRange(interval.min_tokens, interval.max_tokens) }}
                </span>
                <span class="font-mono text-gray-900 dark:text-white">
                  {{ formatInterval(interval, model.pricing.billing_mode) }}
                </span>
              </div>
            </div>
          </template>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BillingMode, UserPricingInterval, UserSupportedModel } from '@/api/channels'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import PricingRow from './PricingRow.vue'

const BILLING_MODE_TOKEN: BillingMode = 'token'
const BILLING_MODE_PER_REQUEST: BillingMode = 'per_request'
const BILLING_MODE_IMAGE: BillingMode = 'image'
const PER_MILLION_SCALE = 1_000_000

const props = withDefaults(defineProps<{
  model: UserSupportedModel
  pricingKeyPrefix?: string
  noPricingLabel?: string
  showPlatform?: boolean
  platformHint?: string
}>(), {
  pricingKeyPrefix: 'availableChannels.pricing',
  noPricingLabel: '',
  showPlatform: true,
  platformHint: ''
})

const { t } = useI18n()

const effectivePlatform = computed(() => props.model.platform || props.platformHint || '')

function prefixKey(key: string): string {
  return `${props.pricingKeyPrefix}.${key}`
}

function platformClasses(platform: string): { badge: string; border: string; header: string } {
  switch (platform) {
    case 'anthropic':
      return {
        badge: 'border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-700 dark:bg-orange-900/20 dark:text-orange-300',
        border: 'border-orange-200 dark:border-orange-700',
        header: 'bg-orange-50 text-orange-700 dark:bg-orange-900/20 dark:text-orange-200'
      }
    case 'openai':
      return {
        badge: 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300',
        border: 'border-emerald-200 dark:border-emerald-700',
        header: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-200'
      }
    case 'gemini':
      return {
        badge: 'border-blue-200 bg-blue-50 text-blue-700 dark:border-blue-700 dark:bg-blue-900/20 dark:text-blue-300',
        border: 'border-blue-200 dark:border-blue-700',
        header: 'bg-blue-50 text-blue-700 dark:bg-blue-900/20 dark:text-blue-200'
      }
    case 'antigravity':
      return {
        badge: 'border-violet-200 bg-violet-50 text-violet-700 dark:border-violet-700 dark:bg-violet-900/20 dark:text-violet-300',
        border: 'border-violet-200 dark:border-violet-700',
        header: 'bg-violet-50 text-violet-700 dark:bg-violet-900/20 dark:text-violet-200'
      }
    default:
      return {
        badge: 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300',
        border: 'border-gray-200 dark:border-dark-600',
        header: 'bg-gray-50 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
      }
  }
}

const badgeClass = computed(() => platformClasses(effectivePlatform.value).badge)
const borderClass = computed(() => platformClasses(effectivePlatform.value).border)
const headerClass = computed(() => platformClasses(effectivePlatform.value).header)

const billingModeLabel = computed(() => {
  switch (props.model.pricing?.billing_mode) {
    case BILLING_MODE_TOKEN:
      return t(prefixKey('billingModeToken'))
    case BILLING_MODE_PER_REQUEST:
      return t(prefixKey('billingModePerRequest'))
    case BILLING_MODE_IMAGE:
      return t(prefixKey('billingModeImage'))
    default:
      return '-'
  }
})

function formatScaled(value: number | null, scale: number): string {
  if (value == null) {
    return '-'
  }
  const scaled = value * scale
  const small = Math.abs(scaled) > 0 && Math.abs(scaled) < 1
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: small ? 6 : 0,
    maximumFractionDigits: small ? 6 : 4
  }).format(scaled)
}

function formatRange(min: number, max: number | null): string {
  return `(${min}, ${max == null ? '∞' : max}]`
}

function formatInterval(interval: UserPricingInterval, mode: BillingMode): string {
  if (mode === BILLING_MODE_PER_REQUEST || mode === BILLING_MODE_IMAGE) {
    return formatScaled(interval.per_request_price, 1)
  }
  return `${formatScaled(interval.input_price, PER_MILLION_SCALE)} / ${formatScaled(interval.output_price, PER_MILLION_SCALE)}`
}

const show = ref(false)
const triggerEl = ref<HTMLElement | null>(null)
const popoverEl = ref<HTMLElement | null>(null)
const popoverStyle = ref<Record<string, string>>({
  top: '0px',
  left: '0px'
})

function updatePosition() {
  const trigger = triggerEl.value
  if (!trigger) {
    return
  }

  const rect = trigger.getBoundingClientRect()
  const popover = popoverEl.value
  const width = popover?.offsetWidth ?? 320
  const height = popover?.offsetHeight ?? 240
  const margin = 8

  let top = rect.bottom + margin
  if (top + height > window.innerHeight - margin) {
    top = Math.max(margin, rect.top - height - margin)
  }

  let left = rect.left + rect.width / 2 - width / 2
  if (left < margin) {
    left = margin
  }
  if (left + width > window.innerWidth - margin) {
    left = window.innerWidth - margin - width
  }

  popoverStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`
  }
}

function onEnter() {
  show.value = true
  nextTick(() => {
    updatePosition()
    window.addEventListener('scroll', updatePosition, true)
    window.addEventListener('resize', updatePosition)
  })
}

function onLeave() {
  show.value = false
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
}

onBeforeUnmount(() => {
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
})
</script>
