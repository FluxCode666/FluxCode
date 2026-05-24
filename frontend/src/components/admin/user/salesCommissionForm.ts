import type { SalesCommissionMode, SalesCommissionTier } from '@/types'

export interface SalesCommissionTierDraft {
  month_sales_from_cny: number
  month_sales_to_cny: number | null
  commission_rate: number
  sort_order: number
}

export interface SalesCommissionFormState {
  isSales: boolean
  salesCommissionMode: SalesCommissionMode
  salesCommissionRate: number
  salesCommissionMinMonthlySales: number
  salesCommissionTiers: SalesCommissionTierDraft[]
}

interface SalesCommissionConfigSource {
  is_sales?: boolean
  sales_commission_mode?: SalesCommissionMode
  sales_commission_rate?: number
  sales_commission_min_monthly_sales?: number
  sales_commission_tiers?: SalesCommissionTier[] | null
}

type TranslateFn = (key: string, params?: Record<string, unknown>) => string

export function createSalesCommissionTierDraft(
  overrides: Partial<SalesCommissionTierDraft> = {}
): SalesCommissionTierDraft {
  return {
    month_sales_from_cny: 0,
    month_sales_to_cny: null,
    commission_rate: 0,
    sort_order: 1,
    ...overrides
  }
}

export function cloneSalesCommissionTiers(
  tiers?: SalesCommissionTier[] | SalesCommissionTierDraft[] | null
): SalesCommissionTierDraft[] {
  if (!tiers?.length) {
    return []
  }
  return tiers.map((tier, index) =>
    createSalesCommissionTierDraft({
      month_sales_from_cny: tier.month_sales_from_cny ?? 0,
      month_sales_to_cny:
        tier.month_sales_to_cny === undefined ? null : tier.month_sales_to_cny,
      commission_rate: tier.commission_rate ?? 0,
      sort_order: tier.sort_order ?? index + 1
    })
  )
}

export function createSalesCommissionFormState(
  source?: SalesCommissionConfigSource
): SalesCommissionFormState {
  return {
    isSales: !!source?.is_sales,
    salesCommissionMode: source?.sales_commission_mode || 'fixed',
    salesCommissionRate: source?.sales_commission_rate || 0,
    salesCommissionMinMonthlySales:
      source?.sales_commission_min_monthly_sales || 0,
    salesCommissionTiers: cloneSalesCommissionTiers(source?.sales_commission_tiers)
  }
}

export function buildSalesCommissionPayload(form: SalesCommissionFormState) {
  return {
    is_sales: form.isSales,
    sales_commission_rate:
      form.salesCommissionMode === 'fixed' ? form.salesCommissionRate : 0,
    sales_commission_mode: form.salesCommissionMode,
    sales_commission_min_monthly_sales:
      form.salesCommissionMode === 'tiered'
        ? form.salesCommissionMinMonthlySales
        : 0,
    sales_commission_tiers:
      form.salesCommissionMode === 'tiered'
        ? normalizeSalesCommissionTiers(form.salesCommissionTiers)
        : []
  }
}

export function validateSalesCommissionForm(
  form: SalesCommissionFormState,
  t: TranslateFn
): string | null {
  if (!form.isSales) {
    return null
  }

  if (form.salesCommissionMode === 'fixed') {
    if (
      typeof form.salesCommissionRate !== 'number' ||
      !Number.isFinite(form.salesCommissionRate) ||
      form.salesCommissionRate <= 0 ||
      form.salesCommissionRate > 100
    ) {
      return t('admin.users.sales.invalidRate')
    }
    return null
  }

  if (
    typeof form.salesCommissionMinMonthlySales !== 'number' ||
    !Number.isFinite(form.salesCommissionMinMonthlySales) ||
    form.salesCommissionMinMonthlySales < 0
  ) {
    return t('admin.users.sales.invalidMinMonthlySales')
  }

  if (!form.salesCommissionTiers.length) {
    return t('admin.users.sales.tiersRequired')
  }

  let previousUpper: number | null = null
  let hasOpenEndedTier = false

  for (const [index, tier] of form.salesCommissionTiers.entries()) {
    const row = index + 1
    if (hasOpenEndedTier) {
      return t('admin.users.sales.openEndedTierMustBeLast', { index: row })
    }
    if (
      typeof tier.month_sales_from_cny !== 'number' ||
      !Number.isFinite(tier.month_sales_from_cny) ||
      tier.month_sales_from_cny < 0
    ) {
      return t('admin.users.sales.invalidTierFrom', { index: row })
    }
    if (
      typeof tier.commission_rate !== 'number' ||
      !Number.isFinite(tier.commission_rate) ||
      tier.commission_rate < 0 ||
      tier.commission_rate > 100
    ) {
      return t('admin.users.sales.invalidTierRate', { index: row })
    }
    if (tier.month_sales_to_cny != null) {
      if (
        typeof tier.month_sales_to_cny !== 'number' ||
        !Number.isFinite(tier.month_sales_to_cny) ||
        tier.month_sales_to_cny <= tier.month_sales_from_cny
      ) {
        return t('admin.users.sales.invalidTierTo', { index: row })
      }
    }
    if (
      previousUpper != null &&
      tier.month_sales_from_cny < previousUpper
    ) {
      return t('admin.users.sales.overlappingTier', { index: row })
    }
    previousUpper = tier.month_sales_to_cny
    hasOpenEndedTier = tier.month_sales_to_cny == null
  }

  return null
}

function normalizeSalesCommissionTiers(
  tiers: SalesCommissionTierDraft[]
): SalesCommissionTier[] {
  return tiers.map((tier, index) => ({
    month_sales_from_cny: tier.month_sales_from_cny,
    month_sales_to_cny: tier.month_sales_to_cny,
    commission_rate: tier.commission_rate,
    sort_order: index + 1
  }))
}
