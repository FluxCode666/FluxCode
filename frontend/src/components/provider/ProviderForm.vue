<template>
  <form class="space-y-4" @submit.prevent="submit">
    <div class="grid gap-3 md:grid-cols-2">
      <label class="text-sm">
        名称
        <input v-model="draft.name" class="input mt-1 w-full" required />
      </label>
      <label class="text-sm">
        Base URL
        <input
          v-model="draft.base_url"
          class="input mt-1 w-full"
          required
          placeholder="https://api.example.com"
        />
      </label>
      <label class="text-sm">
        认证方式
        <select v-model="draft.auth_type" class="input mt-1 w-full">
          <option value="bearer">Bearer</option>
          <option value="x-api-key">X-Api-Key</option>
          <option value="none">None</option>
        </select>
      </label>
      <label class="text-sm">
        API Key（仅写入）
        <input
          v-model="draft.api_key"
          type="password"
          class="input mt-1 w-full"
          :placeholder="credentialConfigured ? '已配置；留空保持原密钥' : '输入上游密钥'"
          @input="draft.clear_api_key = false"
        />
      </label>
      <label class="text-sm">
        并发上限
        <input v-model.number="draft.concurrency" type="number" min="1" class="input mt-1 w-full" />
      </label>
      <label class="text-sm">
        供应商成本倍率
        <input
          v-model.number="draft.rate_multiplier"
          type="number"
          min="0"
          step="0.01"
          class="input mt-1 w-full"
        />
      </label>
    </div>

    <GroupSelector v-model="draft.group_ids" :groups="groups" />

    <label v-if="credentialConfigured" class="flex items-center gap-2 text-sm text-red-600">
      <input v-model="draft.clear_api_key" type="checkbox" @change="draft.api_key = undefined" />
      清除已配置的 API Key
    </label>
    <label class="flex items-center gap-2 text-sm">
      <input v-model="draft.allow_protocol_conversion" type="checkbox" />
      允许协议转换（默认关闭，仅使用已验证 Adapter）
    </label>

    <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
      <div class="mb-2 flex items-center justify-between">
        <strong>协议端点</strong>
        <button type="button" class="btn-secondary text-xs" @click="addEndpoint">添加端点</button>
      </div>
      <div
        v-for="(endpoint, index) in draft.endpoints"
        :key="index"
        class="mb-3 grid gap-2 rounded-lg bg-gray-50 p-2 dark:bg-dark-800 md:grid-cols-2 xl:grid-cols-[180px_180px_1fr_1fr_auto]"
      >
        <select v-model="endpoint.protocol" class="input" @change="onEndpointProtocolChange(endpoint)">
          <option v-for="protocol in protocols" :key="protocol" :value="protocol">
            {{ protocol }}
          </option>
        </select>
        <select v-model="endpoint.wire_profile" class="input">
          <option v-for="profile in wireProfiles" :key="profile" :value="profile">
            {{ profile }}
          </option>
        </select>
        <input v-model="endpoint.path" class="input" placeholder="协议路径" />
        <input v-model="endpoint.base_url" class="input" placeholder="继承 Base URL" />
        <span class="flex items-center justify-end gap-3">
          <label class="flex items-center gap-1 text-xs">
            <input v-model="endpoint.enabled" type="checkbox" />启用
          </label>
          <button type="button" class="text-red-500" @click="removeEndpoint(index)">删除</button>
        </span>
      </div>
    </div>

    <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
      <div class="mb-2 flex items-center justify-between">
        <strong>模型能力</strong>
        <button type="button" class="btn-secondary text-xs" @click="addCapability">添加能力</button>
      </div>
      <div
        v-for="(capability, index) in draft.capabilities"
        :key="index"
        class="mb-3 grid gap-2 rounded-lg bg-gray-50 p-2 dark:bg-dark-800 md:grid-cols-2 xl:grid-cols-[1fr_1fr_180px_180px_auto]"
      >
        <input v-model="capability.logical_model" class="input" placeholder="逻辑模型" required />
        <input v-model="capability.upstream_model" class="input" placeholder="上游模型" required />
        <select
          v-model="capability.protocol"
          class="input"
          @change="onCapabilityProtocolChange(capability)"
        >
          <option v-for="protocol in protocols" :key="protocol" :value="protocol">
            {{ protocol }}
          </option>
        </select>
        <select v-model="capability.feature_profile" class="input">
          <option
            v-for="profile in featureProfilesFor(capability.protocol)"
            :key="profile"
            :value="profile"
          >
            {{ profile }}
          </option>
        </select>
        <span class="flex items-center justify-end gap-3">
          <label class="flex items-center gap-1 text-xs">
            <input v-model="capability.enabled" type="checkbox" />启用
          </label>
          <button type="button" class="text-red-500" @click="removeCapability(index)">删除</button>
        </span>
      </div>
    </div>

    <div class="flex justify-end gap-2">
      <button type="button" class="btn-secondary" @click="$emit('cancel')">取消</button>
      <button class="btn-primary" :disabled="saving">
        {{ saving ? '保存中…' : '保存为草稿' }}
      </button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import type {
  ProviderCapabilityWriteRequest,
  ProviderEndpointWriteRequest,
  ProviderFeatureProfile,
  ProviderProtocol,
  ProviderWriteRequest,
  ProviderWireProfile
} from '@/api/admin/providers'
import { PROVIDER_PROTOCOL_DEFAULT_PATHS, PROVIDER_PROTOCOLS } from '@/api/admin/providers'
import type { AdminGroup } from '@/types'

const props = withDefaults(
  defineProps<{
    modelValue: ProviderWriteRequest
    groups?: AdminGroup[]
    saving?: boolean
    credentialConfigured?: boolean
  }>(),
  { groups: () => [], saving: false, credentialConfigured: false }
)
const emit = defineEmits<{ (event: 'submit', value: ProviderWriteRequest): void; (event: 'cancel'): void }>()
const protocols = PROVIDER_PROTOCOLS
const wireProfiles: ProviderWireProfile[] = [
  'canonical_v1',
  'newapi_messages_v1',
  'siliconflow_messages_v1'
]
const conversationalFeatureProfiles: ProviderFeatureProfile[] = [
  'text_v1',
  'stream_text_v1',
  'function_tools_v1'
]
const blank = (): ProviderWriteRequest => ({ name: '', base_url: '', auth_type: 'bearer', allow_protocol_conversion: false, group_ids: [], endpoints: [], capabilities: [] })
const draft = reactive<ProviderWriteRequest>(blank())
watch(() => props.modelValue, value => Object.assign(draft, JSON.parse(JSON.stringify(value))), { immediate: true, deep: true })
function addEndpoint() { draft.endpoints.push({ protocol: 'chat_completions', wire_profile: 'canonical_v1', path: '/v1/chat/completions', enabled: true }) }
function removeEndpoint(index: number) { draft.endpoints.splice(index, 1) }
function addCapability() { draft.capabilities.push({ logical_model: '', upstream_model: '', protocol: 'chat_completions', feature_profile: 'text_v1', enabled: true }) }
function removeCapability(index: number) { draft.capabilities.splice(index, 1) }
function onEndpointProtocolChange(endpoint: ProviderEndpointWriteRequest) {
  endpoint.path = defaultProtocolPath(endpoint.protocol)
  endpoint.wire_profile = 'canonical_v1'
}
function onCapabilityProtocolChange(capability: ProviderCapabilityWriteRequest) {
  capability.feature_profile = capability.protocol === 'embeddings' ? 'embeddings_v1' : 'text_v1'
}
function featureProfilesFor(protocol: ProviderProtocol): ProviderFeatureProfile[] {
  return protocol === 'embeddings' ? ['embeddings_v1'] : conversationalFeatureProfiles
}
function defaultProtocolPath(protocol: ProviderProtocol): string {
  return PROVIDER_PROTOCOL_DEFAULT_PATHS[protocol]
}
function submit() { emit('submit', JSON.parse(JSON.stringify(draft))) }
</script>
