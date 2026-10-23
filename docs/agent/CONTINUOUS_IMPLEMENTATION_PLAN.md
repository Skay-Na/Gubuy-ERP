# 历史实施计划

本文保留旧阶段的设计与验收记录。低库存监测、补货草案与周期库存计划已于 2026-10-23 按门店运营要求停用；相关计划内容不代表当前系统能力。

# Agent-Native Continuous Implementation Plan

Updated: 2026-10-11 (Asia/Shanghai)

## Objective and guardrails

Deliver the general planner, persistent schedule, and runtime reliability work on the existing Vue/Gin/GORM/MySQL monolith. Reuse the current encrypted AI provider configuration and the `AgentWorkTask`, `AgentWorkTaskStep`, `AgentToolCallAudit`, `RuntimeExecutor`, and `RetailReadService` foundations. Keep all new automatic business behavior opt-in. Do not write to a production database or invoke procurement/payment flows.

The working tree initially had local untracked evaluation artifacts (`output/`, `docs/agent-live-evaluation.json`, `.playwright-cli/`). They are pre-existing and must remain untouched/uncommitted.

## Initial audit

- `AGENT-RUNTIME-UNIFICATION-REPORT.md` accurately reports four registered runtime tools and a deterministic, fixed three-step replenishment workflow. There is no LLM planner, general sales analysis, or persistent user schedule yet.
- `AGENT-NATIVE-PHASE2-REPORT.md` and `AGENT-NATIVE-MIGRATION-REPORT.md` confirm the existing chat agent still has a separate legacy registry/provider path, daily monitor uses a process-local 24-hour ticker, draft review is internal only, and previous browser validation does not cover the requested new flows.
- `agentnative.RuntimeExecutor` enforces a hard-coded permission map and writes tool audits, but does not validate per-tool schemas, tool metadata, durable planner decisions, model usage, or result-context limits. The existing task worker uses a fixed workflow, and task cancellation is only polled between tools.
- `MonitorService.ProcessPending` uses conditional claims and a lease but has no fencing token on subsequent state writes. This needs review before sharing worker behavior with planner and scheduler tasks.
- The active provider selection and decrypted key handling live in the AI settings/controller path. New planner calls must adapt that path instead of adding provider configuration.
- The frontend workbench is a single `AIManager.vue` view and currently lists signals, tasks, steps, drafts, activities, and goals; no schedule API/UI exists.
- GORM `AutoMigrate` is the existing startup schema mechanism. New records should be additive and default-disabled; production migration is out of scope.

## Work breakdown and gates

### Phase 1 — General planner and dynamic execution

1. Add the shared planner protocol, bounded model-call interface, validated decision types, registered tool metadata/schema validation, and planner/model audit rows.
2. Extend deterministic business read services and registered tools for bounded product, stock, sales trend, combined sales/stock, signal, draft, and task reads. Keep writes limited to the existing internal draft behind server permissions and human review.
3. Persist planner/tool decisions and outputs as task steps. Resume from successful step outputs, enforce call/time/context/result/token limits, propagate task cancellation, and preserve deterministic monitor behavior when a provider is unavailable.
4. Add mock-provider tests for dynamic branching, malformed decisions, injection resistance, authorization, cancellation, budget limits, and failure degradation.
5. Connect durable chat requests and monitor signal analysis to the same runtime where feasible; keep ordinary synchronous chat compatible.

**Gate:** an isolated dataset/provider test must prove that planner decisions branch differently after low-stock versus adequate-stock evidence, with every call passing the shared executor and persisted step path.

### Phase 2 — Persistent scheduler and natural-language management

1. Add additive schedule and run-history models, unique `(schedule_id, planned_run_at)` identity, timezone-aware daily inventory schedules, ownership, enable/status fields, bounded missed-run policy, and lease/fencing data.
2. Add a controllable-clock scheduler service with atomic multi-worker claiming and recovery; create real `AgentWorkTask` records and invoke the shared runtime. Keep the scheduler feature flag off by default and prevent duplicate legacy ticker scans.
3. Add owner-scoped list/create/update/pause/resume/cancel/history APIs and safe natural-language schedule intent handling; unsupported or ambiguous management requests must clarify.
4. Add a schedule panel to AIManager using backend state, with history and task links.
5. Test timezone/DST, restart, pause/resume, duplicate workers, cancellation, ownership, and absence of high-risk tools.

**Gate:** two independent scheduler service instances over an isolated database must claim a planned run once and produce one durable task; a schedule must survive service reconstruction.

### Phase 3 — Reliability, safety, and end-to-end acceptance

1. Add fencing/version checks to task claims, side-effect idempotency, scheduler leases, cancellation audit, stale-worker rejection, and explicit reconciliation/manual-review states where execution outcome is ambiguous.
2. Add a separate-process worker crash harness covering planner-before-step, read during execution, post-side-effect/pre-step-commit, and scheduler claim windows. Do not simulate process death by merely editing row state.
3. Add LLM call telemetry and per-task budgets without storing secrets, full prompts, or hidden chain-of-thought; sanitize/bound user and tool data.
4. Add isolated API/browser validation for chat analysis, multi-tool evidence, internal draft review, scheduled execution, pause, cancellation, and actual status rendering.
5. Run the requested backend/frontend generators and checks only where their configured dependencies and isolated DB are available; report each command and result separately.

**Gate:** actual process termination/restart and browser UI flows must be observed. Any unavailable external dependency remains explicitly unpassed.

## Architecture decisions

- Keep one `AgentRuntime`/planner/tool execution path. Trigger sources (chat, monitor, scheduler) supply task intent/capabilities; they do not implement separate tool loops.
- LLM output is an untrusted, size-limited JSON decision. The server resolves tools from a fixed registry, validates strict argument schemas and permissions, and runs them only through `RuntimeExecutor`.
- Tool output, product notes, signal evidence, and user text are data, never system instructions. Persist concise decision summaries and evidence references only.
- Scheduler supports only a real `daily_inventory_check` execution initially. It may create a bounded internal analysis task; it cannot inherit procurement, payment, pricing, or arbitrary write permissions.
- Scheduler and new semantic planner flags default off. The existing deterministic monitor path remains available and must not be duplicated by the new scheduler.
- Use additive GORM schema changes. Do not run migrations or tests against production or the normal Compose database.
- A unique schedule run key and transactional/conditional claim aim for recoverable deduplication, not a claim of external exactly-once side effects.

## Progress checkpoint

Current phase: **Complete — Phases 1–3 implemented and validated; final report written.**

### Completed implementation

- Phase 1: added `AgentPlanner`, strict JSON decisions, bounded dynamic tool loop, durable planner call rows, persisted decisions/tool outputs, recovery from successful tool evidence, task capability checks and existing-provider adapter. Added sales summary/trend, combined inventory/sales and related read/draft/task tools. New general analysis and planner flags default off.
- Phase 2: added additive schedule and run models, owner-scoped APIs, `daily_inventory_check`, timezone-aware next-run calculation, finite missed-window skip, transactional due dispatch, unique run identity and natural-language create/list/history/update/pause/resume/cancel handling. Added scheduler panel and history/task links. The persisted scheduler flag defaults off and bypasses the old daily ticker only when enabled.
- Phase 3: added lease fencing and stale-owner checks, persisted cancellation/activity, context propagation, budget telemetry, prompt/output/tool validation, idempotent draft reconciliation, malicious tool-output permission tests, multi-worker scheduler tests and real subprocess termination/recovery tests. The subprocess harness now kills workers while a read query is blocked, after an internal draft transaction, after Planner decision before step creation, and after Scheduler run/task commit.
- The global chat component signals the AIManager after a Planner task returns, so the schedule/task panel refreshes after natural-language schedule changes. Browser verification confirmed pause/resume immediately updates the displayed schedule.
- Updated the order delivery compatibility mapping test and removed attempts to update the legacy `ai_agent_messages.updated_at` column, which does not exist in that table.

### Validation state

- `ERP_AGENT_NATIVE_TEST_DSN=... go test -count=1 ./agentnative`: passed against disposable MySQL test DB. Covers low-stock versus ample-stock dynamic branches, durable scheduling, two competing scheduler instances, DST, owner-scoped schedule history, budget stop, cancellation propagation, hostile tool output and process crash/recovery.
- `ERP_RETAIL_READ_TEST_DSN=... go test -count=1 ./services`: passed against a separate disposable MySQL test DB.
- `ERP_AUDIT_DB=1 DB_NAME=erp_audit_test ... go test -count=1 ./controllers`: passed against a disposable MySQL test DB, including explicit intent and admin route coverage.
- Full `go test -count=1 ./...`, `go vet ./...`, `npm run build`, `node --test src/**/*.test.mjs`, `go generate .`, and `git diff --check` passed in the final validation pass; exact environments/results are in the testing records and final report.
- Browser validation used the real Vue app, isolated Go backend, disposable `erp_browser_test` MySQL DB and a local OpenAI-compatible Mock Provider. It confirmed persisted cross-module sales/inventory evidence, natural-language schedule create/pause/resume, a real minute-triggered schedule run, a low-stock signal, internal draft creation/review and live workbench refresh. It did not use a real external model/provider.

### Remaining limitations and gates

- A real external LLM/provider, provider rate limits and provider-reported token accounting were unavailable; Mock Provider coverage is not represented as real-model verification.
- No browser test cancels an in-flight task. Worker-side context cancellation is covered by an isolated MySQL integration test; task-cancel auditing is implemented in the authenticated API path.
- Natural-language schedule-history lookup has an owner-scoped registered tool and service test; browser verification used the AIManager history button rather than a chat request for history.
- Product inventory event logs are not yet exposed as a Planner tool. The implemented cross-module path uses current inventory, sales summary/trend, and replenishment context.
- The data model is still single-store/single-tenant (`tenant_id=single`); it is not multi-tenant isolation.
- New GORM schema migration has only been exercised on disposable test DBs. No production migration/deployment was performed. Feature flags stay off by default.
- Final worktree audit found an unrelated startup data migration that sets every product's `main_stock` to zero and enables cloud fulfillment. It is now guarded by `ERP_MIGRATE_UNIFIED_CLOUD_INVENTORY=1`, with `.env.example` defaulting to `0`; it was not run.
- Existing unrelated worktree changes and pre-existing untracked evaluation/browser artifacts are preserved.

### Final checkpoint

- Root report delivered at `AGENT-NATIVE-PHASE3-5-FINAL-REPORT.md`.
- After guarding the unrelated inventory rewrite, the isolated full Go suite and `go vet ./...` were rerun and passed. Frontend build/unit results, `go generate .`, browser E2E, subprocess crash tests, and final diff checks are recorded in the root report and testing records.
- This turn's backend, Vite, and local Mock Provider processes were stopped. Test artifacts and unrelated user worktree files were preserved. Production database, formal procurement/payment, and deployment were not touched.
