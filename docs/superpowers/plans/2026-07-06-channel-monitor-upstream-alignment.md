# Channel Monitor Upstream Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align the local channel monitor implementation with upstream's `jitter_seconds` scheduling behavior while keeping the local feature switch default disabled.

**Architecture:** Add `jitter_seconds` as an admin-only monitor configuration field that flows from database schema through service validation, repository persistence, admin DTOs, and frontend forms. Change runner scheduling from fixed tickers to resettable timers that compute `interval_seconds ± jitter_seconds` on every cycle while preserving the existing feature-switch gate.

**Tech Stack:** Go 1.26 backend, ent code generation, Gin handlers, PostgreSQL migrations, Vue 3, Pinia, TypeScript, Vitest.

## Global Constraints

- Work on branch `codex/channel-monitor-upstream-align`.
- Do not merge all of `upstream/main`.
- Keep `channel_monitor_enabled=false` as the local default.
- Do not rewrite the user-facing channel status page.
- Keep endpoint SSRF validation, API key encryption, response masking, OpenAI `responses` checks, request template snapshots, and error-body preservation intact.
- Use migration number `126_channel_monitor_jitter.sql`, continuing from the local latest migration `125_add_group_allow_image_generation.sql`.
- Keep `jitter_seconds >= 0` and `interval_seconds - jitter_seconds >= 15`.

---

## File Structure

- `backend/migrations/126_channel_monitor_jitter.sql`: Adds the nullable-safe `jitter_seconds` column, default, and check constraint.
- `backend/ent/schema/channel_monitor.go`: Declares the ent field and comment.
- `backend/ent/*channelmonitor*`: Generated ent files after `go generate ./ent`.
- `backend/internal/service/channel_monitor_const.go`: Adds the public error for invalid jitter.
- `backend/internal/service/channel_monitor_types.go`: Adds `JitterSeconds` to monitor models and create/update params.
- `backend/internal/service/channel_monitor_validate.go`: Adds `validateJitter`.
- `backend/internal/service/channel_monitor_service.go`: Validates and applies jitter during create/update.
- `backend/internal/repository/channel_monitor_repo.go`: Persists and loads jitter.
- `backend/internal/handler/admin/channel_monitor_handler.go`: Accepts and returns `jitter_seconds`.
- `backend/internal/service/channel_monitor_runner.go`: Uses timer-based jitter scheduling.
- `backend/internal/service/channel_monitor_validate_test.go`: Covers jitter validation.
- `backend/internal/service/channel_monitor_runner_test.go`: Covers delay bounds and disabled-switch scheduling behavior.
- `frontend/src/api/admin/channelMonitor.ts`: Adds TypeScript API field.
- `frontend/src/components/admin/monitor/MonitorFormDialog.vue`: Adds admin form control and payload wiring.
- `frontend/src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts`: Tests default, edit, and payload behavior.
- `frontend/src/i18n/locales/zh.ts`: Adds Simplified Chinese labels.
- `frontend/src/i18n/locales/en.ts`: Adds English labels.

---

### Task 1: Backend Persistence And Validation

**Files:**
- Create: `backend/migrations/126_channel_monitor_jitter.sql`
- Modify: `backend/ent/schema/channel_monitor.go`
- Modify: `backend/internal/service/channel_monitor_const.go`
- Modify: `backend/internal/service/channel_monitor_types.go`
- Modify: `backend/internal/service/channel_monitor_validate.go`
- Modify: `backend/internal/service/channel_monitor_service.go`
- Modify: `backend/internal/repository/channel_monitor_repo.go`
- Modify: `backend/internal/handler/admin/channel_monitor_handler.go`
- Test: `backend/internal/service/channel_monitor_validate_test.go`

**Interfaces:**
- Consumes: Existing `ChannelMonitor`, `ChannelMonitorCreateParams`, `ChannelMonitorUpdateParams`, and `ChannelMonitorRepository`.
- Produces: `ChannelMonitor.JitterSeconds int`, `ChannelMonitorCreateParams.JitterSeconds int`, `ChannelMonitorUpdateParams.JitterSeconds *int`, `validateJitter(jitterSec int, intervalSec int) error`, and JSON field `jitter_seconds`.

- [ ] **Step 1: Write the failing validation tests**

Add this test to `backend/internal/service/channel_monitor_validate_test.go`:

```go
func TestValidateChannelMonitorJitter(t *testing.T) {
	t.Run("allows zero jitter", func(t *testing.T) {
		require.NoError(t, validateJitter(0, 60))
	})

	t.Run("allows jitter while minimum delay stays at floor", func(t *testing.T) {
		require.NoError(t, validateJitter(45, 60))
	})

	t.Run("rejects negative jitter", func(t *testing.T) {
		require.ErrorIs(t, validateJitter(-1, 60), ErrChannelMonitorInvalidJitter)
	})

	t.Run("rejects jitter that drops effective interval below floor", func(t *testing.T) {
		require.ErrorIs(t, validateJitter(46, 60), ErrChannelMonitorInvalidJitter)
	})

	t.Run("rejects jitter equal to interval", func(t *testing.T) {
		require.ErrorIs(t, validateJitter(15, 15), ErrChannelMonitorInvalidJitter)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd backend && go test ./internal/service -run 'TestValidateChannelMonitorJitter' -count=1
```

Expected: FAIL with `undefined: validateJitter` or `undefined: ErrChannelMonitorInvalidJitter`.

- [ ] **Step 3: Add migration**

Create `backend/migrations/126_channel_monitor_jitter.sql`:

```sql
-- 126_channel_monitor_jitter.sql
-- Add per-monitor scheduling jitter. A value of 0 preserves fixed interval behavior.

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS jitter_seconds INTEGER NOT NULL DEFAULT 0;

ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_jitter_check;

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_jitter_check
    CHECK (jitter_seconds >= 0 AND interval_seconds - jitter_seconds >= 15);
```

- [ ] **Step 4: Add ent schema field**

In `backend/ent/schema/channel_monitor.go`, add the field immediately after `interval_seconds`:

```go
		field.Int("jitter_seconds").
			Default(0).
			NonNegative().
			Comment("Per-run scheduling jitter in seconds; actual delay is interval_seconds +/- jitter_seconds, and service validation keeps the effective interval >= 15 seconds"),
```

- [ ] **Step 5: Add service error and validation helper**

In `backend/internal/service/channel_monitor_const.go`, add this error after `ErrChannelMonitorInvalidInterval`:

```go
	ErrChannelMonitorInvalidJitter = infraerrors.BadRequest(
		"CHANNEL_MONITOR_INVALID_JITTER", "jitter_seconds must be >= 0 and interval_seconds - jitter_seconds must be >= 15",
	)
```

In `backend/internal/service/channel_monitor_validate.go`, add this function after `validateInterval`:

```go
func validateJitter(jitterSec, intervalSec int) error {
	if jitterSec < 0 || intervalSec-jitterSec < monitorMinIntervalSeconds {
		return ErrChannelMonitorInvalidJitter
	}
	return nil
}
```

- [ ] **Step 6: Wire service model fields**

In `backend/internal/service/channel_monitor_types.go`, add `JitterSeconds` to the three structs:

```go
type ChannelMonitor struct {
	ID                  int64
	Name                string
	Provider            string
	APIMode             string
	Endpoint            string
	APIKey              string
	PrimaryModel        string
	ExtraModels         []string
	GroupName           string
	Enabled             bool
	IntervalSeconds     int
	JitterSeconds       int
	LastCheckedAt       *time.Time
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
	TemplateID          *int64
	ExtraHeaders        map[string]string
	BodyOverrideMode    string
	BodyOverride        map[string]any
	APIKeyDecryptFailed bool
}
```

```go
type ChannelMonitorCreateParams struct {
	Name             string
	Provider         string
	APIMode          string
	Endpoint         string
	APIKey           string
	PrimaryModel     string
	ExtraModels      []string
	GroupName        string
	Enabled          bool
	IntervalSeconds  int
	JitterSeconds    int
	CreatedBy        int64
	TemplateID       *int64
	ExtraHeaders     map[string]string
	BodyOverrideMode string
	BodyOverride     map[string]any
}
```

```go
type ChannelMonitorUpdateParams struct {
	Name             *string
	Provider         *string
	APIMode          *string
	Endpoint         *string
	APIKey           *string
	PrimaryModel     *string
	ExtraModels      *[]string
	GroupName        *string
	Enabled          *bool
	IntervalSeconds  *int
	JitterSeconds    *int
	TemplateID       *int64
	ClearTemplate    bool
	ExtraHeaders     *map[string]string
	BodyOverrideMode *string
	BodyOverride     *map[string]any
}
```

- [ ] **Step 7: Apply create and update validation**

In `backend/internal/service/channel_monitor_service.go`, add `JitterSeconds` when constructing `ChannelMonitor`:

```go
		JitterSeconds:    p.JitterSeconds,
```

In `validateCreateParams`, after `validateInterval`:

```go
	if err := validateJitter(p.JitterSeconds, p.IntervalSeconds); err != nil {
		return err
	}
```

In `applyMonitorUpdate`, after the interval block:

```go
	if p.JitterSeconds != nil {
		existing.JitterSeconds = *p.JitterSeconds
	}
	if p.IntervalSeconds != nil || p.JitterSeconds != nil {
		if err := validateJitter(existing.JitterSeconds, existing.IntervalSeconds); err != nil {
			return err
		}
	}
```

- [ ] **Step 8: Persist and load jitter**

In `backend/internal/repository/channel_monitor_repo.go`, add `SetJitterSeconds(m.JitterSeconds)` after `SetIntervalSeconds(m.IntervalSeconds)` in both `Create` and `Update`:

```go
		SetIntervalSeconds(m.IntervalSeconds).
		SetJitterSeconds(m.JitterSeconds).
```

In `entToServiceMonitor`, add:

```go
		JitterSeconds:    row.JitterSeconds,
```

- [ ] **Step 9: Expose admin API field**

In `backend/internal/handler/admin/channel_monitor_handler.go`, add request and response fields:

```go
	JitterSeconds    int               `json:"jitter_seconds" binding:"omitempty,min=0,max=3585"`
```

```go
	JitterSeconds    *int               `json:"jitter_seconds" binding:"omitempty,min=0,max=3585"`
```

```go
	JitterSeconds       int                                  `json:"jitter_seconds"`
```

In `channelMonitorToResponse`, add:

```go
		JitterSeconds:       m.JitterSeconds,
```

In `Create` params, add:

```go
		JitterSeconds:    req.JitterSeconds,
```

In `Update` params, add:

```go
		JitterSeconds:    req.JitterSeconds,
```

- [ ] **Step 10: Run validation tests**

Run:

```bash
cd backend && go test ./internal/service -run 'TestValidateChannelMonitorJitter|TestNormalizeChannelMonitorInterval|TestChannelMonitorConstants' -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add backend/migrations/126_channel_monitor_jitter.sql \
  backend/ent/schema/channel_monitor.go \
  backend/internal/service/channel_monitor_const.go \
  backend/internal/service/channel_monitor_types.go \
  backend/internal/service/channel_monitor_validate.go \
  backend/internal/service/channel_monitor_service.go \
  backend/internal/repository/channel_monitor_repo.go \
  backend/internal/handler/admin/channel_monitor_handler.go \
  backend/internal/service/channel_monitor_validate_test.go
git commit -m "feat(channel-monitor): persist scheduling jitter"
```

---

### Task 2: Runner Timer Jitter Scheduling

**Files:**
- Modify: `backend/internal/service/channel_monitor_runner.go`
- Modify: `backend/internal/service/channel_monitor_runner_test.go`

**Interfaces:**
- Consumes: `ChannelMonitor.JitterSeconds`.
- Produces: `scheduledMonitor.nextDelay() time.Duration`, timer-based `runScheduled`, and disabled-switch behavior where tasks can be scheduled while checks remain gated by `channel_monitor_enabled`.

- [ ] **Step 1: Write failing runner tests**

In `backend/internal/service/channel_monitor_runner_test.go`, add a run counter to the stub:

```go
type channelMonitorRunnerSvcStub struct {
	listCalls int
	runCalls  int
	monitors  []*ChannelMonitor
}

func (s *channelMonitorRunnerSvcStub) RunCheck(context.Context, int64) ([]*CheckResult, error) {
	s.runCalls++
	return nil, nil
}
```

Replace `TestChannelMonitorRunnerStartDefaultDisabledSkipsScheduling` with:

```go
func TestChannelMonitorRunnerStartDefaultDisabledSchedulesButSkipsChecks(t *testing.T) {
	svc := &channelMonitorRunnerSvcStub{
		monitors: []*ChannelMonitor{
			{
				ID:              10,
				Name:            "disabled-startup",
				Enabled:         true,
				IntervalSeconds: 60,
				JitterSeconds:   5,
			},
		},
	}
	runner := newChannelMonitorRunner(svc, &channelMonitorRuntimeStub{
		settings: ChannelMonitorRuntimeSettings{Enabled: false},
	})
	defer runner.Stop()

	runner.Start()
	time.Sleep(20 * time.Millisecond)

	require.Equal(t, 1, svc.listCalls)
	require.Equal(t, 0, svc.runCalls)
	require.Len(t, runner.tasks, 1)
	require.Equal(t, 5*time.Second, runner.tasks[10].jitter)
}
```

Add delay tests:

```go
func TestScheduledMonitorNextDelayNoJitter(t *testing.T) {
	task := &scheduledMonitor{
		interval: 60 * time.Second,
		jitter:   0,
	}

	require.Equal(t, 60*time.Second, task.nextDelay())
}

func TestScheduledMonitorNextDelayWithJitterStaysInRange(t *testing.T) {
	task := &scheduledMonitor{
		interval: 60 * time.Second,
		jitter:   10 * time.Second,
	}

	for i := 0; i < 100; i++ {
		delay := task.nextDelay()
		require.GreaterOrEqual(t, delay, 50*time.Second)
		require.LessOrEqual(t, delay, 70*time.Second)
	}
}

func TestScheduledMonitorNextDelayClampsToMinimum(t *testing.T) {
	task := &scheduledMonitor{
		interval: 15 * time.Second,
		jitter:   20 * time.Second,
	}

	for i := 0; i < 100; i++ {
		delay := task.nextDelay()
		require.GreaterOrEqual(t, delay, 15*time.Second)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd backend && go test ./internal/service -run 'TestChannelMonitorRunnerStartDefaultDisabledSchedulesButSkipsChecks|TestScheduledMonitorNextDelay' -count=1
```

Expected: FAIL with `scheduledMonitor has no field jitter` or `task.nextDelay undefined`.

- [ ] **Step 3: Implement jitter timer**

In `backend/internal/service/channel_monitor_runner.go`, add import:

```go
	"math/rand/v2"
```

Extend `scheduledMonitor`:

```go
type scheduledMonitor struct {
	id       int64
	name     string
	interval time.Duration
	jitter   time.Duration
	cancel   context.CancelFunc
}
```

Add `nextDelay` after the struct:

```go
func (t *scheduledMonitor) nextDelay() time.Duration {
	if t.jitter <= 0 {
		return t.interval
	}
	offset := time.Duration(rand.Int64N(int64(2*t.jitter) + 1))
	delay := t.interval - t.jitter + offset
	if floor := monitorMinIntervalSeconds * time.Second; delay < floor {
		return floor
	}
	return delay
}
```

Replace `Start` with this version so disabled startup still registers tasks while `fire` remains switch-gated:

```go
func (r *ChannelMonitorRunner) Start() {
	if r == nil || r.svc == nil {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), monitorStartupLoadTimeout)
	defer cancel()
	enabled, err := r.svc.ListEnabledMonitors(ctx)
	if err != nil {
		slog.Error("channel_monitor: load enabled monitors failed at startup", "error", err)
		return
	}
	for _, m := range enabled {
		r.Schedule(m)
	}
	slog.Info("channel_monitor: runner started", "scheduled_tasks", len(enabled))
}
```

In `Schedule`, compute jitter after interval:

```go
	jitter := time.Duration(m.JitterSeconds) * time.Second
	if jitter < 0 {
		jitter = 0
	}
```

Set it on the task:

```go
	task := &scheduledMonitor{
		id:       m.ID,
		name:     m.Name,
		interval: interval,
		jitter:   jitter,
		cancel:   cancel,
	}
```

Replace ticker logic in `runScheduled` with timer logic:

```go
func (r *ChannelMonitorRunner) runScheduled(ctx context.Context, task *scheduledMonitor) {
	defer r.wg.Done()

	r.fire(ctx, task)
	timer := time.NewTimer(task.nextDelay())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			r.fire(ctx, task)
			timer.Reset(task.nextDelay())
		}
	}
}
```

- [ ] **Step 4: Run runner tests**

Run:

```bash
cd backend && go test ./internal/service -run 'TestChannelMonitorRunner|TestScheduledMonitorNextDelay' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/channel_monitor_runner.go backend/internal/service/channel_monitor_runner_test.go
git commit -m "feat(channel-monitor): add jittered runner scheduling"
```

---

### Task 3: Frontend Admin Form Wiring

**Files:**
- Modify: `frontend/src/api/admin/channelMonitor.ts`
- Modify: `frontend/src/components/admin/monitor/MonitorFormDialog.vue`
- Create: `frontend/src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

**Interfaces:**
- Consumes: Admin API JSON field `jitter_seconds`.
- Produces: `ChannelMonitor.jitter_seconds`, `CreateParams.jitter_seconds`, form field `form.jitter_seconds`, and i18n keys `admin.channelMonitor.form.jitterSeconds` and `admin.channelMonitor.form.jitterSecondsHint`.

- [ ] **Step 1: Write failing form tests**

Create `frontend/src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts`:

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import MonitorFormDialog from '../MonitorFormDialog.vue'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'

const { createMonitor, updateMonitor, listTemplates } = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  updateMonitor: vi.fn(),
  listTemplates: vi.fn().mockResolvedValue({ items: [] })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_default_interval_seconds: 60 },
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    channelMonitorDefaultIntervalSeconds: 60
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: createMonitor,
      update: updateMonitor
    },
    channelMonitorTemplate: {
      list: listTemplates
    }
  }
}))

vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: vi.fn()
  }
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: {
    getUserGroupRates: vi.fn()
  }
}))

vi.mock('@/composables/useChannelMonitorFormat', () => ({
  useChannelMonitorFormat: () => ({
    providerPickerClass: () => 'provider-picker'
  })
}))

const baseMonitor: ChannelMonitor = {
  id: 7,
  name: 'OpenAI monitor',
  provider: 'openai',
  api_mode: 'chat_completions',
  endpoint: 'https://api.example.com',
  api_key_masked: 'sk-***',
  primary_model: 'gpt-4o-mini',
  extra_models: [],
  group_name: '',
  enabled: true,
  interval_seconds: 60,
  jitter_seconds: 12,
  last_checked_at: null,
  created_by: 1,
  created_at: '2026-07-06T00:00:00Z',
  updated_at: '2026-07-06T00:00:00Z',
  primary_status: '',
  primary_latency_ms: null,
  availability_7d: 0,
  extra_models_status: [],
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null
}

function mountDialog(props: { show?: boolean; monitor?: ChannelMonitor | null } = {}) {
  return mount(MonitorFormDialog, {
    props: {
      show: true,
      monitor: null,
      ...props
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Toggle: { template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', true)" />', props: ['modelValue'] },
        Select: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
        ProviderIcon: true
      }
    }
  })
}

describe('MonitorFormDialog jitter', () => {
  beforeEach(() => {
    createMonitor.mockReset()
    updateMonitor.mockReset()
    listTemplates.mockClear()
    createMonitor.mockResolvedValue({})
    updateMonitor.mockResolvedValue({})
  })

  it('defaults jitter_seconds to zero for new monitors', () => {
    const wrapper = mountDialog()

    const input = wrapper.get<HTMLInputElement>('[data-testid="monitor-jitter-input"]')

    expect(input.element.value).toBe('0')
  })

  it('loads existing monitor jitter_seconds when editing', async () => {
    const wrapper = mountDialog({ monitor: baseMonitor })

    await vi.dynamicImportSettled()

    const input = wrapper.get<HTMLInputElement>('[data-testid="monitor-jitter-input"]')
    expect(input.element.value).toBe('12')
  })

  it('submits jitter_seconds in create payload', async () => {
    const wrapper = mountDialog()

    await wrapper.get('input[placeholder="admin.channelMonitor.form.namePlaceholder"]').setValue('Monitor')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.endpointPlaceholder"]').setValue('https://api.example.com')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.apiKeyPlaceholder"]').setValue('sk-test')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.primaryModelPlaceholder"]').setValue('claude-sonnet-4')
    await wrapper.get('[data-testid="monitor-jitter-input"]').setValue(9)
    await wrapper.get('form').trigger('submit.prevent')

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      jitter_seconds: 9
    }))
  })

  it('clamps jitter max when interval is reduced', async () => {
    const wrapper = mountDialog()

    await wrapper.get('[data-testid="monitor-jitter-input"]').setValue(50)
    await wrapper.get('[data-testid="monitor-interval-input"]').setValue(30)

    const input = wrapper.get<HTMLInputElement>('[data-testid="monitor-jitter-input"]')
    expect(input.element.value).toBe('15')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts
```

Expected: FAIL because `[data-testid="monitor-jitter-input"]` is missing.

- [ ] **Step 3: Add TypeScript API field**

In `frontend/src/api/admin/channelMonitor.ts`, add:

```ts
  /** 每次调度在 interval 基础上 +/- [0, jitter] 的随机偏移（秒），0 = 固定间隔 */
  jitter_seconds: number
```

after `interval_seconds: number`.

Add to `CreateParams`:

```ts
  jitter_seconds?: number
```

- [ ] **Step 4: Add form control and state**

In `frontend/src/components/admin/monitor/MonitorFormDialog.vue`, replace the interval input block with:

```vue
      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.intervalSeconds') }} <span class="text-red-500">*</span></label>
        <input
          v-model.number="form.interval_seconds"
          data-testid="monitor-interval-input"
          type="number"
          min="15"
          max="3600"
          required
          class="input"
        />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.intervalSecondsHint') }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.channelMonitor.form.jitterSeconds') }}</label>
        <input
          v-model.number="form.jitter_seconds"
          data-testid="monitor-jitter-input"
          type="number"
          min="0"
          :max="maxJitterSeconds"
          class="input"
        />
        <p class="mt-1 text-xs text-gray-400">{{ t('admin.channelMonitor.form.jitterSecondsHint') }}</p>
      </div>
```

Add to `MonitorForm`:

```ts
  jitter_seconds: number
```

Add to `form`:

```ts
  jitter_seconds: 0,
```

Add computed and clamp watcher after `form`:

```ts
const maxJitterSeconds = computed<number>(() => Math.max(0, (Number(form.interval_seconds) || 0) - 15))

watch(() => form.interval_seconds, () => {
  if (form.jitter_seconds > maxJitterSeconds.value) {
    form.jitter_seconds = maxJitterSeconds.value
  }
})
```

Add to `resetForm`:

```ts
  form.jitter_seconds = 0
```

Add to `loadFromMonitor`:

```ts
  form.jitter_seconds = m.jitter_seconds || 0
```

Add to `buildPayload`:

```ts
    jitter_seconds: form.jitter_seconds || 0,
```

- [ ] **Step 5: Add i18n labels**

In `frontend/src/i18n/locales/zh.ts`, after `intervalSecondsHint`:

```ts
        jitterSeconds: '随机抖动 (秒)',
        jitterSecondsHint: '每轮检测在间隔基础上随机提前或延后，最大值为检测间隔减 15 秒；0 表示固定间隔。',
```

In `frontend/src/i18n/locales/en.ts`, after `intervalSecondsHint`:

```ts
        jitterSeconds: 'Jitter (seconds)',
        jitterSecondsHint: 'Randomly runs each check earlier or later within this range. Maximum is interval minus 15 seconds; 0 keeps a fixed interval.',
```

- [ ] **Step 6: Run frontend tests**

Run:

```bash
cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts src/views/admin/__tests__/ChannelMonitorView.spec.ts
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/api/admin/channelMonitor.ts \
  frontend/src/components/admin/monitor/MonitorFormDialog.vue \
  frontend/src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts \
  frontend/src/i18n/locales/zh.ts \
  frontend/src/i18n/locales/en.ts
git commit -m "feat(channel-monitor): expose jitter in admin form"
```

---

### Task 4: Generate Ent Code And Verify End-To-End

**Files:**
- Modify: `backend/ent/channelmonitor.go`
- Modify: `backend/ent/channelmonitor/channelmonitor.go`
- Modify: `backend/ent/channelmonitor/where.go`
- Modify: `backend/ent/channelmonitor_create.go`
- Modify: `backend/ent/channelmonitor_query.go`
- Modify: `backend/ent/channelmonitor_update.go`
- Modify: `backend/ent/migrate/schema.go`
- Modify: `backend/ent/mutation.go`
- Modify: other `backend/ent/*` files changed by `go generate ./ent`

**Interfaces:**
- Consumes: `backend/ent/schema/channel_monitor.go`.
- Produces: generated ent APIs `SetJitterSeconds`, `JitterSeconds`, `channelmonitor.FieldJitterSeconds`, and SQL migration metadata.

- [ ] **Step 1: Generate ent code**

Run:

```bash
cd backend && go generate ./ent
```

Expected: command exits with code 0 and generated ent files include `JitterSeconds`.

- [ ] **Step 2: Verify generated field exists**

Run:

```bash
rg -n "JitterSeconds|jitter_seconds|FieldJitterSeconds" backend/ent
```

Expected: output includes:

```text
backend/ent/channelmonitor.go
backend/ent/channelmonitor/channelmonitor.go
backend/ent/channelmonitor_create.go
backend/ent/channelmonitor_update.go
backend/ent/migrate/schema.go
```

- [ ] **Step 3: Run backend tests**

Run:

```bash
cd backend && go test ./internal/service -run 'ChannelMonitor|Jitter' -count=1
```

Expected: PASS.

Run:

```bash
cd backend && go test ./internal/repository -run 'ChannelMonitor|migration' -count=1
```

Expected: PASS.

Run:

```bash
cd backend && go test ./internal/handler ./internal/server/routes -run 'ChannelMonitor' -count=1
```

Expected: PASS.

- [ ] **Step 4: Run frontend tests and typecheck**

Run:

```bash
cd frontend && npm run test -- --run src/components/admin/monitor/__tests__/MonitorFormDialog.jitter.spec.ts src/views/admin/__tests__/ChannelMonitorView.spec.ts
```

Expected: PASS.

Run:

```bash
cd frontend && npm run type-check
```

Expected: PASS.

- [ ] **Step 5: Confirm default closed behavior**

Run:

```bash
rg -n "SettingKeyChannelMonitorEnabled: +\"false\"|channel_monitor_enabled: false|channelMonitorEnabled.*false" backend/internal/service frontend/src/stores
```

Expected: output shows the backend default setting remains `"false"` and frontend fallback remains `channel_monitor_enabled: false`.

- [ ] **Step 6: Review diff scope**

Run:

```bash
git diff --stat HEAD~3..HEAD
git diff --stat
git status --short
```

Expected: committed and uncommitted diff is limited to channel monitor backend, channel monitor frontend, ent generation, migration, tests, and i18n. `git status --short` may still show unrelated pre-existing untracked files:

```text
?? docs/plans/2026-06-22-openai-image-generation-api.md
?? frontend/pnpm-workspace.yaml
```

- [ ] **Step 7: Commit generated code and verification fixes**

```bash
git add backend/ent backend/migrations/126_channel_monitor_jitter.sql \
  backend/internal/service backend/internal/repository backend/internal/handler/admin/channel_monitor_handler.go \
  frontend/src/api/admin/channelMonitor.ts frontend/src/components/admin/monitor frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "chore(channel-monitor): regenerate ent for jitter"
```
