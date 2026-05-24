import type { MethodLimit } from '@/types/payment'

export type RechargeAmountValidationCode =
  | 'none'
  | 'config_min'
  | 'config_max'
  | 'method_min'
  | 'method_max'
  | 'selected_method_min'
  | 'selected_method_max'
  | 'no_method'

export interface RechargeAmountValidationResult {
  code: RechargeAmountValidationCode
  min?: number
  max?: number
}

export interface RechargeAmountValidationInput {
  amount: number
  configMin: number
  configMax: number
  methodGlobalMin: number
  methodGlobalMax: number
  methods: Record<string, MethodLimit>
  selectedMethod?: string
}

const isPositiveLimit = (value: number): boolean => Number.isFinite(value) && value > 0

export function resolveRechargeMinAmount(configMin: number, methodGlobalMin: number): number {
  const limits = [configMin, methodGlobalMin].filter(isPositiveLimit)
  if (limits.length === 0) return 0
  return Math.max(...limits)
}

export function resolveRechargeMaxAmount(configMax: number, methodGlobalMax: number): number {
  const limits = [configMax, methodGlobalMax].filter(isPositiveLimit)
  if (limits.length === 0) return 0
  return Math.min(...limits)
}

export function amountFitsMethodLimit(amount: number, limit?: MethodLimit | null): boolean {
  if (!limit || amount <= 0) return true
  if (limit.single_min > 0 && amount < limit.single_min) return false
  if (limit.single_max > 0 && amount > limit.single_max) return false
  return true
}

export function validateRechargeAmount(input: RechargeAmountValidationInput): RechargeAmountValidationResult {
  if (!(input.amount > 0)) return { code: 'none' }

  const effectiveMin = resolveRechargeMinAmount(input.configMin, input.methodGlobalMin)
  const effectiveMax = resolveRechargeMaxAmount(input.configMax, input.methodGlobalMax)

  if (effectiveMin > 0 && input.amount < effectiveMin) {
    return { code: 'config_min', min: effectiveMin }
  }

  if (effectiveMax > 0 && input.amount > effectiveMax) {
    return { code: 'config_max', max: effectiveMax }
  }

  const methodLimits = Object.values(input.methods ?? {})
  if (!methodLimits.some((limit) => amountFitsMethodLimit(input.amount, limit))) {
    if (input.methodGlobalMin > 0 && input.amount < input.methodGlobalMin) {
      return { code: 'method_min', min: input.methodGlobalMin }
    }
    if (input.methodGlobalMax > 0 && input.amount > input.methodGlobalMax) {
      return { code: 'method_max', max: input.methodGlobalMax }
    }
    return { code: 'no_method' }
  }

  if (!input.selectedMethod) {
    return { code: 'none' }
  }

  const selectedLimit = input.methods[input.selectedMethod]
  if (!selectedLimit) {
    return { code: 'none' }
  }

  if (selectedLimit.single_min > 0 && input.amount < selectedLimit.single_min) {
    return { code: 'selected_method_min', min: selectedLimit.single_min }
  }

  if (selectedLimit.single_max > 0 && input.amount > selectedLimit.single_max) {
    return { code: 'selected_method_max', max: selectedLimit.single_max }
  }

  return { code: 'none' }
}
