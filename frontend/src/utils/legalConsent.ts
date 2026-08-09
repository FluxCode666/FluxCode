const OAUTH_LEGAL_CONSENT_PENDING_KEY = 'oauth_legal_terms_consent_pending'

export function markOAuthLegalConsentPending(): void {
  sessionStorage.setItem(OAUTH_LEGAL_CONSENT_PENDING_KEY, '1')
}

export function hasOAuthLegalConsentPending(): boolean {
  return sessionStorage.getItem(OAUTH_LEGAL_CONSENT_PENDING_KEY) === '1'
}

export function clearOAuthLegalConsentPending(): void {
  sessionStorage.removeItem(OAUTH_LEGAL_CONSENT_PENDING_KEY)
}
