---
title: Codex OpenAI Stability Upstream Alignment - Plan
type: feat
date: 2026-08-13
topic: codex-openai-stability
artifact_contract: ce-unified-plan/v1
artifact_readiness: requirements-only
product_contract_source: ce-brainstorm
execution: code
---

# Codex OpenAI Stability Upstream Alignment - Plan

## Goal Capsule

- **Objective:** Selectively port the Codex/OpenAI stability behavior introduced in upstream `v0.1.175`, using the latest equivalent behavior on `upstream/main` as the authority.
- **Product authority:** Upstream behavior defines the feature semantics. FluxCode may adapt the implementation to its current architecture but must not add product-level deviations.
- **Open blockers:** None before planning. Planning must map the upstream behavior onto FluxCode's diverged gateway paths and identify existing stronger local behavior that should remain.

---

## Product Contract

### Summary

FluxCode will gain the Codex/OpenAI reliability behavior associated with upstream `v0.1.175` without merging an upstream branch. The work covers OAuth fingerprint convergence, correct failure and failover decisions, scheduling stability, and accurate stream timing and usage interpretation.

### Problem Frame

FluxCode and upstream have diverged substantially, so a branch merge or mechanical sequence of cherry-picks would introduce unrelated behavior and conflict with local gateway extensions. At the same time, FluxCode lacks several upstream protections that prevent excess device/session visibility, empty successful responses, inappropriate account penalties, retry amplification, lost capacity backoff, and misleading stream metrics.

The alignment therefore needs a behavior-level contract. Upstream remains the semantic authority while the implementation is integrated into FluxCode's current request, failover, scheduling, and administration flows.

### Key Decisions

- **Use current upstream semantics for the selected `v0.1.175` capabilities.** (session-settled: user-directed — chosen over freezing behavior at the `v0.1.175` tag: same-feature corrections on current `upstream/main` should be retained.) Governs R1, R4-R12.
- **Keep the upstream fingerprint derivation and sharing boundary unchanged.** (session-settled: user-directed — chosen over adding FluxCode's API Key dimension: the port must preserve upstream behavior.) Governs R2-R4.
- **Default unconfigured OpenAI OAuth accounts to `session` convergence.** (session-settled: user-directed — chosen over opt-in or new-account-only activation: existing and new accounts should follow the upstream default.) Governs R2-R3.
- **Port by behavior rather than by branch history.** (session-settled: user-directed — chosen over merging upstream branches: unrelated upstream changes must not enter this work.) Governs R1 and R15.

### Requirements

**Upstream alignment boundary**

- R1. The implementation must reproduce the selected stability behavior as it exists on the pinned latest `upstream/main` baseline, limited to capabilities introduced or corrected within the `v0.1.175` stability scope.
- R2. Every OpenAI OAuth account without an explicit fingerprint setting must use the upstream `session` convergence mode, including accounts created before this feature is deployed.
- R3. Administrators must be able to select the upstream `off`, `device`, `session`, and `full` fingerprint modes when creating, editing, or bulk-editing eligible accounts.
- R4. Outbound fingerprint identifiers and metadata must follow the upstream derivation, fallback, and sharing rules without adding API Key or user identity to the seeds.

**Failure classification and failover**

- R5. An upstream Responses stream that terminates as completed without usable output must be treated as a failed attempt eligible for failover, not recorded as a successful zero-output response.
- R6. Read failures in OpenAI OAuth image streams must trigger the same upstream-defined failover behavior as equivalent gateway stream failures.
- R7. Deterministic client-side failures from the native Responses path must retain their client-error semantics and must not be normalized into retryable gateway failures.
- R8. HTML-form 403 responses from upstream infrastructure must not cause an OpenAI account to be penalized as an account-level authentication or quota failure.
- R9. Authentication failures encountered during OpenAI passthrough pool execution must receive the upstream-defined retry opportunity before the gateway fails over to another account.
- R10. If a compact streaming response has committed transport headers but produces no usable event payload, the client must receive a terminal failure rather than remain attached to an incomplete stream.
- R11. Invalid reasoning item identifiers on the OpenAI API Key passthrough path must be removed according to upstream behavior before forwarding.

**Scheduling, timing, and usage correctness**

- R12. Codex scheduling evaluation must preserve exponential capacity backoff, ignore or reset stale snapshots as upstream does, retain valid usage percentages, and cache explicitly unset threshold settings correctly.
- R13. Responses and WebSocket timing must measure first visible output according to upstream rules, excluding terminal-only events while preserving the upstream no-delta fallback.
- R14. OpenAI usage parsing must honor upstream precedence for supported top-level and nested usage envelopes so scheduling and usage records are based on the intended values.

**Change containment**

- R15. The migration must use targeted semantic adaptation and must not merge `upstream/main`, merge the release branch, or import unrelated changes that happen to share touched files.
- R16. Existing FluxCode behavior that is outside this contract must remain unchanged unless it directly conflicts with an upstream semantic requirement above; a direct conflict is resolved in favor of upstream behavior and recorded during planning.

### Actors

- A1. **Administrator:** Controls fingerprint convergence for OpenAI OAuth accounts and expects safe defaults for existing accounts.
- A2. **Gateway client:** Expects valid responses, terminal errors, and stable streaming behavior without hangs or retry amplification.
- A3. **OpenAI OAuth account pool:** Supplies upstream capacity while receiving correct retry, failover, scheduling, and penalty decisions.
- A4. **Operations and billing telemetry:** Consumes timing and usage values that must reflect visible output and the authoritative upstream usage envelope.

### Key Flows

- F1. OAuth fingerprint convergence
  - **Trigger:** An eligible OpenAI OAuth request is prepared for upstream forwarding.
  - **Actors:** A1, A2, A3
  - **Steps:** Resolve the account's mode; derive the upstream-defined identifier set once; apply the same values to supported headers and request metadata; forward the request.
  - **Outcome:** Upstream observes the device, session, thread, turn, and window identity defined by R2-R4.

- F2. Recoverable upstream attempt failure
  - **Trigger:** A selected account produces an empty completed response, image stream read failure, or retryable pool authentication failure.
  - **Actors:** A2, A3
  - **Steps:** Classify the condition using R5, R6, and R9; avoid recording a false success; continue through the permitted retry or failover path.
  - **Outcome:** The client receives a valid result from another permitted attempt or a terminal error after the retry policy is exhausted.

- F3. Non-retryable upstream or transport outcome
  - **Trigger:** The upstream returns a deterministic client error, infrastructure HTML 403, or committed compact stream without usable payload.
  - **Actors:** A2, A3
  - **Steps:** Apply the condition-specific rules in R7, R8, and R10; preserve the correct client-facing outcome; avoid an incorrect account-level penalty.
  - **Outcome:** The request terminates predictably without retry amplification, client hanging, or avoidable account removal.

- F4. Scheduling and telemetry update
  - **Trigger:** A Codex usage snapshot or stream event is processed after an upstream attempt.
  - **Actors:** A3, A4
  - **Steps:** Parse usage using R14; evaluate freshness and thresholds using R12; calculate visible-output timing using R13; persist or cache the resulting state through existing FluxCode flows.
  - **Outcome:** Scheduling, operations, and usage records use stable and semantically correct values.

### Acceptance Examples

- AE1. **Covers R2-R4.** Given an existing OpenAI OAuth account with no fingerprint setting, when a Codex request is forwarded, then `session` mode is used and the identifiers match the upstream account-level and client-session derivation rules without an API Key component.
- AE2. **Covers R3.** Given an administrator selects any of `off`, `device`, `session`, or `full`, when the account is saved and later edited or bulk-edited, then the selected upstream mode is preserved and applied.
- AE3. **Covers R5.** Given a Responses attempt emits a completed event with no usable output, when another eligible account is available, then the first attempt is not billed or recorded as a successful zero-output result and failover proceeds.
- AE4. **Covers R7.** Given the native Responses upstream returns a deterministic 400-class client error, when the gateway handles it, then the client receives the appropriate client error and the request is not amplified through retryable 502 handling.
- AE5. **Covers R8.** Given an upstream proxy or protection layer returns an HTML 403 page, when account error handling runs, then the account is not marked abnormal or rate-limited solely because of that response.
- AE6. **Covers R10.** Given compact streaming has committed headers but emits no usable event payload, when the attempt terminates, then the client receives a terminal failure event and does not hang indefinitely.
- AE7. **Covers R12.** Given a Codex account already has an exponential capacity backoff and receives a new qualifying capacity failure, when scheduling state is updated, then the backoff is not reset to its initial duration.
- AE8. **Covers R13-R14.** Given terminal-only events precede no visible delta and usage exists in a supported nested envelope, when telemetry is recorded, then TTFT follows the upstream fallback rule and usage uses the upstream precedence result.

### Success Criteria

- The selected upstream behavior is traceable to a pinned `upstream/main` commit and covered by focused regression tests in the FluxCode paths that implement it.
- Existing local tests outside the affected OpenAI gateway, account configuration, scheduling, and telemetry behavior continue to pass without requiring unrelated product changes.
- A reviewer can map each imported behavior to an upstream source commit and distinguish semantic adaptation from unrelated upstream drift.

### Scope Boundaries

- Upstream response-model billing, billing hardening, and service-tier account-cost correction are deferred to the separate billing-safety work unit.
- Large-file multipart backup upload and restore are deferred to the separate backup work unit.
- Other `v0.1.175` changes, including security-audit navigation, Gemini schema normalization, API Key form validation, operations memory formatting, and unrelated administration polish, are outside this plan.
- Later capabilities added to `upstream/main` are outside this plan unless they are direct corrections to one of R2-R14.

<!-- ce-section: work-relationships -->
### How This Work Fits Together

This plan owns only the Codex/OpenAI stability work from the broader `v0.1.175` migration request. The following breakdown is contextual and may be revised by later plans.

- **Billing safety**
  - Shares the OpenAI usage and gateway billing boundary with R14.
  - Depends on stable usage interpretation but remains independently deliverable and testable.
- **Large-file backup and restore**
  - Can proceed independently of this stability work.
  - Shares only the broader release-alignment objective, not runtime behavior.

### Dependencies / Assumptions

- The semantic source is the latest fetched `upstream/main` commit that still contains the selected `v0.1.175` behaviors; planning must pin the exact commit rather than relying on a moving branch name.
- FluxCode's existing OpenAI extensions remain authoritative where R1-R16 do not specify conflicting behavior.
- Upstream's default `session` mode intentionally changes unconfigured existing OAuth accounts at deployment time; no data migration is required to opt them in.

### Outstanding Questions

**Deferred to Planning**

- Which FluxCode forwarding paths share enough structure to implement each upstream behavior once, and which require path-specific adaptations?
- Which existing FluxCode tests already prove stronger equivalent behavior and can replace a direct port of an upstream test?
- Which direct conflicts with local extensions must be documented as intentional upstream-semantic overrides under R16?

### Sources / Research

- Upstream release baseline: tag `v0.1.175`, released 2026-08-12.
- Latest semantic baseline inspected during brainstorming: `upstream/main` at `fbfdcef8184ae4b2e224d5cfc47cf1d0e3742710`.
- Fingerprint convergence source: upstream commit `c0ab3a00ea733cc0559a5a949c28fb5d9d7c5d16`.
- Existing FluxCode alignment conventions: `docs/superpowers/specs/2026-07-13-gpt56-end-to-end-upstream-alignment-design.md`, `docs/superpowers/specs/2026-07-06-channel-monitor-upstream-alignment-design.md`, and `docs/superpowers/specs/2026-07-02-openai-codex-image-bridge-upstream-alignment-design.md`.
