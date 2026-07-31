<template>
  <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-700">
    <table class="min-w-full text-left text-sm">
      <thead class="bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-800 dark:text-gray-400">
        <tr><th class="px-3 py-2">模型</th><th class="px-3 py-2">入站协议</th><th class="px-3 py-2">上游协议</th><th class="px-3 py-2">层级</th><th class="px-3 py-2">供应商</th></tr>
      </thead>
      <tbody>
        <tr v-for="item in rows" :key="`${item.provider_id}-${item.logical_model}-${item.ingress_protocol}-${item.tier}`" class="border-t border-gray-100 dark:border-dark-700">
          <td class="px-3 py-2 font-medium">{{ item.logical_model }}</td>
          <td class="px-3 py-2">{{ item.ingress_protocol }}</td>
          <td class="px-3 py-2">{{ item.upstream_protocol }}</td>
          <td class="px-3 py-2"><span :class="item.tier === 'native' ? 'text-emerald-600' : 'text-amber-600'">{{ item.tier }}</span><span v-if="item.adapter" class="ml-1 text-xs text-gray-400">({{ item.adapter }})</span></td>
          <td class="px-3 py-2">{{ item.provider_name }} #{{ item.provider_id }}</td>
        </tr>
        <tr v-if="rows.length === 0"><td colspan="5" class="px-3 py-6 text-center text-gray-500">暂无已声明能力</td></tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import type { GroupProviderCapability } from '@/api/admin/providers'
defineProps<{ rows: GroupProviderCapability[] }>()
</script>
