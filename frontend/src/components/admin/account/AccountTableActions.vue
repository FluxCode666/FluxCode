<template>
  <div class="flex flex-wrap items-center gap-3">
    <slot name="before"></slot>
    <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">
      <Icon name="refresh" size="md" :class="[loading ? 'animate-spin' : '']" />
    </button>
    <slot name="after"></slot>
    <div class="relative" ref="moreDropdownRef">
      <button
        type="button"
        class="btn btn-secondary px-2 md:px-3"
        :title="t('common.more')"
        @click="showMoreDropdown = !showMoreDropdown"
      >
        <Icon name="more" size="sm" class="md:mr-1.5" />
        <span class="hidden md:inline">{{ t('common.more') }}</span>
      </button>
      <div
        v-if="showMoreDropdown"
        class="absolute right-0 z-50 mt-2 w-56 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
      >
        <div class="p-2">
          <button
            type="button"
            :class="menuItemClass"
            @click="emitSync"
          >
            <Icon name="sync" size="sm" class="text-gray-500 dark:text-gray-400" />
            <span>{{ t('admin.accounts.syncFromCrs') }}</span>
          </button>
          <slot name="more" :close="closeMoreDropdown" :item-class="menuItemClass"></slot>
        </div>
      </div>
    </div>
    <slot name="beforeCreate"></slot>
    <button @click="$emit('create')" class="btn btn-primary">{{ t('admin.accounts.createAccount') }}</button>
    <slot name="afterCreate"></slot>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps(['loading'])
const emit = defineEmits(['refresh', 'sync', 'create'])

const { t } = useI18n()
const showMoreDropdown = ref(false)
const moreDropdownRef = ref<HTMLElement | null>(null)
const menuItemClass =
  'flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-60 dark:text-gray-200 dark:hover:bg-gray-700'

const closeMoreDropdown = () => {
  showMoreDropdown.value = false
}

const emitSync = () => {
  closeMoreDropdown()
  emit('sync')
}

const handleClickOutside = (event: MouseEvent) => {
  if (moreDropdownRef.value && !moreDropdownRef.value.contains(event.target as Node)) {
    closeMoreDropdown()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
