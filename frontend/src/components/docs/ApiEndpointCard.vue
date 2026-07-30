<template>
  <article
    :id="endpoint.id"
    :aria-labelledby="titleId"
    data-testid="api-endpoint-card"
    tabindex="-1"
    class="scroll-mt-24 rounded-xl border border-gray-200 bg-white p-5 outline-none shadow-sm dark:border-dark-600 dark:bg-dark-800 sm:p-6"
  >
    <header class="border-b border-gray-100 pb-5 dark:border-dark-700">
      <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <span
              class="inline-flex rounded-full px-2.5 py-1 font-mono text-xs font-semibold"
              :class="methodClass(endpoint.method)"
            >{{ endpoint.method }}</span>
            <code class="break-all font-mono text-sm text-gray-600 dark:text-dark-300">{{ baseUrl }}{{ endpoint.path }}</code>
          </div>
          <h3 :id="titleId" class="mt-3 text-base font-semibold text-gray-900 dark:text-white">
            {{ endpoint.title }}
          </h3>
          <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">
            {{ endpoint.description }}
          </p>
        </div>
      </div>
    </header>

    <div class="mt-5 space-y-5">
      <section :aria-labelledby="requestParametersTitleId">
        <h4 :id="requestParametersTitleId" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('apiDocs.endpointDocumentation.requestParameters') }}
        </h4>
        <p :id="requestDescriptionId" data-testid="api-endpoint-request-description" class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">
          <span class="font-medium text-gray-800 dark:text-dark-100">{{ t('apiDocs.endpointDocumentation.requestDescription') }}：</span>
          {{ endpoint.requestDescription }}
        </p>
        <div class="mt-3 overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="min-w-[860px] w-full border-collapse text-left text-sm" :aria-describedby="requestDescriptionId">
            <thead class="bg-gray-50 text-xs font-medium uppercase tracking-wide text-gray-500 dark:bg-dark-900/70 dark:text-dark-400">
              <tr>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.parameter') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.location') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.required') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.type') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.defaultValueLabel') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.description') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-600">
              <tr v-for="parameter in endpoint.requestParameters" :key="parameter.location + parameter.name">
                <th scope="row" class="whitespace-nowrap px-4 py-3 align-top font-medium text-gray-900 dark:text-white">
                  <code class="font-mono text-[13px]">{{ parameter.name }}</code>
                </th>
                <td class="whitespace-nowrap px-4 py-3 align-top text-gray-600 dark:text-dark-300">{{ parameter.location }}</td>
                <td class="whitespace-nowrap px-4 py-3 align-top text-gray-600 dark:text-dark-300">
                  {{ parameter.required ? t('apiDocs.endpointDocumentation.yes') : t('apiDocs.endpointDocumentation.no') }}
                </td>
                <td class="whitespace-nowrap px-4 py-3 align-top text-gray-600 dark:text-dark-300">
                  <code class="font-mono text-[13px]">{{ parameter.type }}</code>
                </td>
                <td data-testid="api-endpoint-default-value" class="whitespace-nowrap px-4 py-3 align-top text-gray-600 dark:text-dark-300">
                  <code v-if="parameter.defaultValue !== undefined" class="font-mono text-[13px]">{{ parameter.defaultValue }}</code>
                  <span v-else>-</span>
                </td>
                <td class="px-4 py-3 align-top leading-6 text-gray-600 dark:text-dark-300">
                  {{ parameter.description }}
                  <span v-if="parameter.example" class="ml-2 whitespace-nowrap text-xs text-gray-500 dark:text-dark-400">
                    {{ t('apiDocs.endpointDocumentation.exampleValue', { value: parameter.example }) }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section :aria-labelledby="responseParametersTitleId">
        <h4 :id="responseParametersTitleId" class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('apiDocs.endpointDocumentation.responseParameters') }}
        </h4>
        <div class="mt-3 overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-600">
          <table class="min-w-[620px] w-full border-collapse text-left text-sm">
            <thead class="bg-gray-50 text-xs font-medium uppercase tracking-wide text-gray-500 dark:bg-dark-900/70 dark:text-dark-400">
              <tr>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.parameter') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.type') }}</th>
                <th scope="col" class="px-4 py-3">{{ t('apiDocs.endpointDocumentation.description') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 dark:divide-dark-600">
              <tr v-for="parameter in endpoint.responseParameters" :key="parameter.name">
                <th scope="row" class="whitespace-nowrap px-4 py-3 align-top font-medium text-gray-900 dark:text-white">
                  <code class="font-mono text-[13px]">{{ parameter.name }}</code>
                </th>
                <td class="whitespace-nowrap px-4 py-3 align-top text-gray-600 dark:text-dark-300">
                  <code class="font-mono text-[13px]">{{ parameter.type }}</code>
                </td>
                <td class="px-4 py-3 align-top leading-6 text-gray-600 dark:text-dark-300">
                  {{ parameter.description }}
                  <span v-if="parameter.example" class="ml-2 whitespace-nowrap text-xs text-gray-500 dark:text-dark-400">
                    {{ t('apiDocs.endpointDocumentation.exampleValue', { value: parameter.example }) }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <div class="grid gap-5 xl:grid-cols-2">
        <CodeBlock :label="t('apiDocs.endpointDocumentation.requestExample')" :code="endpoint.requestExample" />
        <CodeBlock :label="t('apiDocs.endpointDocumentation.responseExample')" :code="endpoint.responseExample" />
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import CodeBlock from '@/components/docs/CodeBlock.vue'
import type { ApiDocumentationMethod, ApiEndpointDocumentation } from './apiEndpointDocumentation'

const props = defineProps<{
  endpoint: ApiEndpointDocumentation
  baseUrl: string
}>()

const { t } = useI18n()
const titleId = computed(() => props.endpoint.id + '-title')
const requestParametersTitleId = computed(() => props.endpoint.id + '-request-parameters-title')
const requestDescriptionId = computed(() => props.endpoint.id + '-request-description')
const responseParametersTitleId = computed(() => props.endpoint.id + '-response-parameters-title')

function methodClass(method: ApiDocumentationMethod): string {
  const classes: Record<ApiDocumentationMethod, string> = {
    GET: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/25 dark:text-emerald-300',
    POST: 'bg-blue-50 text-blue-700 dark:bg-blue-900/25 dark:text-blue-300',
    PUT: 'bg-amber-50 text-amber-700 dark:bg-amber-900/25 dark:text-amber-300',
    DELETE: 'bg-red-50 text-red-700 dark:bg-red-900/25 dark:text-red-300'
  }
  return classes[method]
}
</script>
