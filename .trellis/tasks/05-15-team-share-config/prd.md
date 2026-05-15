# 提交团队配置到 Git

## 需求
将 `.claude/`、`.opencode/`、`.trellis/` 三个配置目录和 `AGENTS.md` 提交到 git 仓库，实现团队共享。

## 范围
- `.claude/` — Claude Code agent 配置、skills、hooks、commands、settings
- `.opencode/` — OpenCode 配置、plugins、skills、agents（排除 node_modules）
- `.trellis/` — 项目管理配置、workflow、spec、scripts（排除 .developer/.runtime/ 个人 workspace journal）
- `AGENTS.md` — 通用 agent 指令文件

## 排除项
- `.trellis/.developer`（已被 gitignore）
- `.trellis/.runtime/`（已被 gitignore）
- `.trellis/workspace/*/`（个人工作日志，需追加 gitignore）
- `.opencode/node_modules/`（已被 gitignore）

## 执行步骤
1. 更新 `.trellis/.gitignore`，追加 `workspace/*/` 排除个人 journal
2. `git add` 上述文件
3. `git commit`，message 用 `chore(config): 提交团队配置到 git`
