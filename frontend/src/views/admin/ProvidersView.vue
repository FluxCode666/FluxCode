<template>
  <AppLayout><div class="space-y-6 p-4 md:p-6">
    <header><h1 class="text-2xl font-semibold">{{ t('admin.providers.title') }}</h1><p class="mt-1 text-sm text-gray-500">{{ t('admin.providers.description') }}</p></header>
    <div class="grid gap-6 xl:grid-cols-[minmax(280px,360px)_1fr]">
      <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-dark-700"><div class="flex items-center justify-between"><h2 class="font-medium">供应商</h2><button class="btn-primary text-sm" @click="startCreate">新建</button></div><p v-if="loading" class="text-sm text-gray-500">加载中…</p><button v-for="provider in providers" :key="provider.id" class="w-full rounded-lg border p-3 text-left dark:border-dark-700" :class="selected?.id === provider.id ? 'border-primary-500 bg-primary-50 dark:bg-primary-950/30' : ''" @click="select(provider)"><div class="flex items-center justify-between"><span class="font-medium">{{ provider.name }}</span><span class="text-xs" :class="provider.status === 'active' ? 'text-emerald-600' : 'text-amber-600'">{{ provider.status }}</span></div><div class="mt-1 text-xs text-gray-500">{{ provider.base_url }} · {{ provider.capabilities.length }} 能力</div></button></section>
      <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><div v-if="selected" class="mb-4 flex flex-wrap justify-end gap-2"><button class="btn-secondary text-sm" @click="testCapability">测试首个能力</button><button v-if="selected.status !== 'active'" class="btn-primary text-sm" @click="activateProvider">激活</button><button v-else class="btn-secondary text-sm" @click="disableProvider">停用</button></div><ProviderForm v-if="editing" :model-value="form" :groups="groups" :credential-configured="selected?.credential_configured ?? false" :saving="saving" @submit="save" @cancel="editing = false" /><div v-else class="flex min-h-48 items-center justify-center text-sm text-gray-500">选择供应商或点击“新建”</div></section>
    </div>
    <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-700"><div class="mb-3 flex flex-wrap items-center gap-3"><h2 class="font-medium">Group 派生能力与迁移快照</h2><input v-model.number="groupID" type="number" min="1" class="input w-28" placeholder="Group ID" /><button class="btn-secondary text-sm" @click="loadMatrix">刷新</button><button class="btn-primary text-sm" @click="createSnapshot">生成 shadow 快照</button><button class="btn-secondary text-sm" @click="rollback">回滚</button></div><CapabilityMatrix :rows="matrix" /><div v-if="snapshots.length" class="mt-4 space-y-2"><div v-for="snapshot in snapshots" :key="snapshot.id" class="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3 text-sm dark:border-dark-700"><span>v{{ snapshot.version }} · {{ snapshot.status }} · {{ snapshot.shadow_diff.native_routes ?? 0 }} native / {{ snapshot.shadow_diff.conversion_routes ?? 0 }} conversion</span><span class="flex gap-2"><button v-if="snapshot.status === 'review_required' || snapshot.status === 'draft'" class="btn-secondary text-xs" @click="approve(snapshot.version)">批准</button><button v-if="snapshot.status === 'approved'" class="btn-primary text-xs" @click="activate(snapshot.version)">切换</button></span></div></div></section>
  </div></AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import providersAPI, { type GroupProviderCapability, type GroupRouteSnapshot, type Provider, type ProviderWriteRequest } from '@/api/admin/providers'
import groupsAPI from '@/api/admin/groups'
import ProviderForm from '@/components/provider/ProviderForm.vue'
import CapabilityMatrix from '@/components/provider/CapabilityMatrix.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores'
import type { AdminGroup } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()
const providers = ref<Provider[]>([]), selected = ref<Provider | null>(null), editing = ref(false), saving = ref(false), loading = ref(false)
const groups = ref<AdminGroup[]>([])
const emptyForm = (): ProviderWriteRequest => ({ name: '', base_url: '', auth_type: 'bearer', allow_protocol_conversion: false, group_ids: [], endpoints: [], capabilities: [] })
const toForm = (provider?: Provider): ProviderWriteRequest => provider ? ({ name: provider.name, base_url: provider.base_url, auth_type: provider.auth_type, allow_protocol_conversion: provider.allow_protocol_conversion, group_ids: provider.group_ids, concurrency: provider.concurrency, rate_multiplier: provider.rate_multiplier, version: provider.version, endpoints: provider.endpoints.map(item => ({ protocol: item.protocol, wire_profile: item.wire_profile, base_url: item.base_url, path: item.path, headers: item.headers, auth_type: item.auth_type, enabled: item.enabled })), capabilities: provider.capabilities.map(item => ({ logical_model: item.logical_model, logical_model_display: item.logical_model_display, protocol: item.protocol, upstream_model: item.upstream_model, wire_profile: item.wire_profile, feature_profile: item.feature_profile, enabled: item.enabled })) }) : emptyForm()
const form = ref<ProviderWriteRequest>(emptyForm()), groupID = ref<number | null>(null), matrix = ref<GroupProviderCapability[]>([]), snapshots = ref<GroupRouteSnapshot[]>([])
async function load() { loading.value = true; try { [providers.value, groups.value] = await Promise.all([providersAPI.list(), groupsAPI.getAll()]) } finally { loading.value = false } }
function startCreate() { selected.value = null; form.value = emptyForm(); editing.value = true }
function select(provider: Provider) { selected.value = provider; form.value = toForm(provider); editing.value = true }
async function save(input: ProviderWriteRequest) { saving.value = true; try { const item = selected.value ? await providersAPI.update(selected.value.id, input) : await providersAPI.create(input); selected.value = item; form.value = toForm(item); editing.value = true; await load() } finally { saving.value = false } }
async function testCapability() { if (!selected.value?.capabilities[0]) return; try { await providersAPI.test(selected.value.id, { capability_id: selected.value.capabilities[0].id }); appStore.showSuccess('供应商能力测试成功') } catch (error: any) { appStore.showError(error?.message || '供应商能力测试失败') } }
async function activateProvider() { if (!selected.value) return; const item = await providersAPI.activate(selected.value.id, selected.value.version); select(item); await load() }
async function disableProvider() { if (!selected.value) return; const item = await providersAPI.disable(selected.value.id, selected.value.version); select(item); await load() }
async function loadMatrix() { if (groupID.value) { [matrix.value, snapshots.value] = await Promise.all([providersAPI.listGroupCapabilities(groupID.value), providersAPI.listSnapshots(groupID.value)]) } }
async function createSnapshot() { if (groupID.value) { await providersAPI.createShadowSnapshot(groupID.value); await loadMatrix() } }
async function approve(version: number) { if (groupID.value) { await providersAPI.approveSnapshot(groupID.value, version); await loadMatrix() } }
async function activate(version: number) { if (groupID.value) { await providersAPI.activateSnapshot(groupID.value, version); await loadMatrix() } }
async function rollback() { if (groupID.value) { await providersAPI.rollbackSnapshot(groupID.value); await loadMatrix() } }
onMounted(load)
</script>
