# 基于 DESIGN.md 重新设计前端 UI

## Goal

将 lingma2api 控制台前端的 UI 从当前的紫色/靛蓝渐变+毛玻璃主题，全面改造为 DESIGN.md 描述的 Claude.com 风格设计系统：暖奶油色画布 + 珊瑚色主色 + 深海军蓝暗色面，配合衬线标题/无衬线正文的排版体系。

## What I Already Know

- 当前配色: 靛蓝/紫罗兰/青色渐变 (`#6366f1` / `#8b5cf6` / `#06b6d4`)，浅色模式为冷色调渐变背景
- 当前组件: 玻璃态卡片 (`bg-card` 带 `backdrop-filter`)，渐变导航栏，渐变按钮
- DESIGN.md 要求: 奶油色 `#faf9f5` 画布，珊瑚色 `#cc785c` 主色，深海军蓝 `#181715` 暗色面，Copernicus 衬线字体标题，Inter/StyreneB 无衬线正文
- 涉及的页面: Dashboard, Requests, Account, Settings, Policies, Models, Logs
- 涉及组件: Layout, StatCard, ExchangeCard, CodeViewer, Pagination, Modal, Skeleton, Spinner 等

## Requirements

1. 替换所有 CSS 变量（颜色、阴影、圆角、间距）为 DESIGN.md 定义的设计令牌
2. 字体体系: 标题使用衬线字体 (Copernicus/Tiempos Headline/Cormorant Garamond)，正文使用无衬线字体 (Inter)
3. 布局保留 sidebar + main-area 结构，但视觉风格全面更新
4. 组件更新:
   - 按钮 → 珊瑚色主按钮，奶油色次按钮（带发丝边框）
   - 卡片 → 奶油卡片 / 深色产品卡片
   - 输入框 → 8px 圆角，发丝边框，聚焦态珊瑚色边框
   - StateCard → 更新为 DESIGN.md 风格
   - Badge → 药丸形状
   - Tab → 珊瑚色 active 指示
5. 暗色模式 → 深海军蓝 `#181715` 基底，暖奶油色文字
6. 配色从冷色渐变改为暖色块式设计（利用奶油/深色对比制造层次，减少阴影使用）
7. 字体替代方案采用 Cormorant Garamond + Inter：标题接近 Copernicus/Tiempos 的衬线编辑感，正文保持 Inter 的屏幕可读性

## Acceptance Criteria

- [ ] CSS 变量全部替换为 DESIGN.md 设计令牌
- [ ] 所有页面视觉一致，无历史样式残留
- [ ] 按钮、卡片、输入框、徽章、标签页等组件符合 DESIGN.md 规格
- [ ] 深色模式正常工作，暗色面使用 `#181715` 基底
- [ ] 响应式布局不破坏
- [ ] 原本未提交的 Material You 主题变更已丢弃，不混入本任务实现

## Technical Approach

- 以 `frontend/src/styles/global.css` 为核心重构设计令牌和通用组件样式，尽量减少页面逻辑改动。
- 保留现有 React 路由、数据加载、业务交互和页面结构，仅对需要体现设计语言的组件 class / 少量文案结构做最小调整。
- 将控制台 UI 映射到 DESIGN.md 的组件语义：sidebar/top-bar 作为控制台版 top-nav，dashboard/status cards 作为 feature-card / product-mockup-card-dark，代码与日志查看区域作为 code-window-card。
- 避免新增复杂依赖；字体优先使用 CSS `@import` Google Fonts 加载 Cormorant Garamond、Inter、JetBrains Mono，并提供系统 fallback。

## Decision (ADR-lite)

**Context**: DESIGN.md 指定 Copernicus / StyreneB，但它们不是可直接使用的公开字体。  
**Decision**: 使用 Cormorant Garamond + Inter 作为公开替代方案，JetBrains Mono 用于代码区域。  
**Consequences**: 能最大程度保留 Claude.com 风格的衬线标题与温暖编辑感，同时不会引入授权字体风险。

## Out of Scope

- 新增页面或功能
- 动画/过渡效果（参考 "Known Gaps"）
- Claude spike-mark 品牌符号（它属于 Anthropic 品牌资产）
- Copernicus/StyreneB 付费字体的加载（使用开源替代品 Cormorant Garamond / Inter）

## Technical Notes

- DESIGN.md 是本任务的主设计来源。
- 当前前端为 Vite + React，主要样式集中在 `frontend/src/styles/global.css`。
- 当前主题基线已从本地未提交 Material You 改动恢复到 git HEAD。
