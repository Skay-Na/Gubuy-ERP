# Agent Skills for erpwonchon

本项目由 Vue 3 + Vite 前端与 Go + Gin + GORM 后端构成。已安装以下 OpenCode skill，位于 `.opencode/skills/<name>/SKILL.md`，OpenCode 启动时自动发现。

## 何时用哪个 skill

- **antfu-vue**（Vue 3 核心 API）
  触发：写/改 Vue SFC、`defineProps`/`defineEmits`/`defineModel`、watcher、Transition/Teleport/Suspense/KeepAlive、`<script setup>` 内部 reactivity。
  注意：源 skill 默认偏好 TypeScript；本项目使用 JavaScript，参考其 API 与模式时按需忽略 `lang="ts"`。

- **hyf0-vue-best-practices**（Vue 3 结构化工作流）
  触发：新增页面/组件、规划 Vue 模块改造、code review。
  流程：架构确认 → 核心（reactivity / SFC / 数据流 / 组合式函数）→ 性能。

- **samber-golang-code-style**（Go 代码风格）
  触发：写/审 `erp-backend/**.go`、建立编码规范询问。

- **samber-golang-testing**（Go 测试）
  触发：写表驱动测试、testify suite、testify mock、CI 测试配置、定位 flaky test。

- **samber-golang-error-handling**（Go 错误处理）
  触发：error 创建/包装/日志、samber/oops 引入决策、`errors.Is/As/Join`、HTTP 中间件日志。

- **minimax-fullstack-dev**（全栈需求澄清与选型）
  触发：新增跨前后端模块、确定数据库/认证/集成方式时。

## 项目内常用命令

- 前端：`npm run dev` / `npm run build` / `npm run preview`
- 前端测试：`node --test src/**/*.test.mjs`
- 后端：`cd erp-backend && go test ./...` / `go vet ./...`

## 说明

OpenCode 启动时按 `.opencode/skills/*/SKILL.md` 自动加载；新增/修改后请退出并重启 opencode。
