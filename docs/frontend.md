# 前端开发文档

## 技术栈

| 技术 | 版本 | 说明 |
|------|------|------|
| React | 18.x | UI 框架 |
| TypeScript | 5.x | 类型系统 |
| Tailwind CSS | 3.4.x | 原子化 CSS |
| shadcn/ui | latest | UI 组件库 |
| Vite | 5.x | 构建工具 |
| React Router | 6.x | 客户端路由 |

## 项目结构

```
frontend/src/
├── App.tsx                 # 路由配置
├── main.tsx                # 应用入口
├── components/             # 共享组件
│   ├── Navbar.tsx          # 导航栏（桌面/H5 双模式）
│   ├── Footer.tsx          # 底部信息
│   ├── Layout.tsx          # 页面布局
│   ├── PillCard.tsx        # 金丹卡片
│   ├── AgentCard.tsx       # 道人卡片
│   ├── BindAgentModal.tsx  # 服用金丹弹窗（选择金丹/权重/顺序）
│   ├── ChatMessage.tsx     # 聊天气泡
│   └── MarkdownRenderer.tsx # Markdown 渲染
├── pages/                  # 页面组件
│   ├── Home.tsx            # 首页（炼丹炉动画）
│   ├── Pills.tsx           # 金丹列表
│   ├── PillDetail.tsx      # 金丹编辑（skill_schema 结构化表单）
│   ├── Agents.tsx          # 道人列表
│   ├── AgentDetail.tsx     # 道人详情（服用金丹、调权重/排序）
│   ├── Chat.tsx            # 论道（对话大厅）
│   └── Settings.tsx        # 设置
├── contexts/               # 状态管理 (Context API)
│   ├── PillContext.tsx     # 金丹状态
│   ├── AgentContext.tsx    # 道人状态
│   └── ChatContext.tsx     # 对话状态
├── services/               # API 服务层
│   ├── api.ts              # 请求封装
│   ├── types.ts            # 类型定义
│   ├── models.ts           # 模型列表
│   ├── pillService.ts      # 金丹 API
│   ├── agentService.ts     # 道人 API（含服用/解绑金丹）
│   └── chatService.ts      # 对话 API（WebSocket）
├── utils/
│   └── format.ts           # 格式化工具
└── styles/
    └── globals.css         # 全局样式
```

## 页面与路由设计

| 路由 | 页面 | 说明 |
|------|------|------|
| `/` | Home | 首页，炼丹炉主殿 |
| `/pills` | Pills | 金丹列表，支持按关键词/内置筛选 |
| `/pills/:id` | PillDetail | 金丹编辑：结构化编辑 skill_schema（表达 DNA、心智模型、决策启发式、示例对话等） |
| `/agents` | Agents | 道人列表 |
| `/agents/:id` | AgentDetail | 道人详情：配置基础性格，服用金丹（BindAgentModal），拖拽调整服用顺序（sort_order），调节权重（weight） |
| `/chat` | Chat | 论道，对话大厅 |
| `/chat/:sessionId` | Chat | 具体对话 |
| `/settings` | Settings | 系统设置 |

### 关键交互

- **服用金丹**：道人详情页通过 `BindAgentModal` 选择金丹并设定初始权重与顺序；已服用列表支持**拖拽排序**（更新 `sort_order`）与权重调节（更新 `weight`）
- **丹性相冲警告**：多颗金丹维度冲突时，后端在 `language_pattern.inner_tensions` 中返回冲突信息，道人详情页/论道页据此展示「丹性相冲」提示
- **金丹编辑**：PillDetail 以结构化表单编辑 skill_schema，无需上传任何文件

## 响应式断点

```javascript
// tailwind.config.js
screens: {
  'sm': '640px',   // 小屏手机
  'md': '768px',   // 平板
  'lg': '1024px',  // 桌面
  'xl': '1280px',  // 大屏桌面
}
```

### H5 适配策略

1. **导航栏**: 桌面端横向导航 → H5 底部固定导航栏
2. **金丹列表**: 桌面端 3 列网格 → H5 单列卡片
3. **道人列表**: 桌面端 4 列网格 → H5 单列卡片
4. **聊天界面**: 桌面端侧边栏+对话区 → H5 底部 Sheet 弹窗选择会话
5. **表格**: 桌面端完整表格 → H5 卡片式列表

## 设计系统

### 色彩

```css
:root {
  --cinnabar: #C23A30;      /* 朱砂红 - 主色 */
  --gold: #D4A843;          /* 金箔黄 - 强调色 */
  --ink: #1A1A2E;           /* 墨黑 - 深色背景 */
  --rice-paper: #F5F0E6;    /* 宣纸米白 - 浅色背景 */
  --jade: #2E8B76;          /* 玉绿 - 次要色 */
  --bronze: #8B6914;        /* 铜色 - 装饰 */
}
```

### 字体

```css
@import url('https://fonts.googleapis.com/css2?family=Noto+Serif+SC:wght@400;600;700&family=Noto+Sans+SC:wght@300;400;500;600&display=swap');

/* 标题字体 */
font-family: 'Noto Serif SC', serif;

/* 正文字体 */
font-family: 'Noto Sans SC', sans-serif;
```

### 动画

```css
/* 漂浮动画 */
@keyframes float {
  0%, 100% { transform: translateY(0); }
  50% { transform: translateY(-10px); }
}

/* 发光脉冲 */
@keyframes glow {
  0%, 100% { box-shadow: 0 0 5px var(--gold); }
  50% { box-shadow: 0 0 20px var(--gold), 0 0 40px var(--cinnabar); }
}

/* 烟雾上升 */
@keyframes smoke {
  0% { opacity: 0.6; transform: translateY(0) scale(1); }
  100% { opacity: 0; transform: translateY(-100px) scale(2); }
}
```

## 状态管理

使用 React Context API + useReducer，分为三个 Context：

### PillContext

```typescript
interface PillState {
  pills: Pill[];             // 金丹列表
  currentPill: Pill | null;  // 当前选中（含 skill_schema）
  loading: boolean;
  error: string | null;
}
```

### AgentContext

```typescript
interface AgentState {
  agents: Agent[];             // 道人列表
  currentAgent: Agent | null;  // 当前选中
  agentPills: AgentPill[];     // 当前道人已服用金丹（含 weight / sort_order）
  loading: boolean;
  error: string | null;
}
```

### ChatContext

```typescript
interface ChatState {
  sessions: ChatSession[];   // 会话列表
  currentSessionId: string | null;
  messages: ChatMessage[];   // 当前会话消息
  streaming: boolean;        // 是否流式输出中
  error: string | null;
}
```

## API 调用示例

```typescript
// 获取金丹列表
const pills = await pillService.list({ page: 1, pageSize: 10, isBuiltin: true });

// 创建金丹（结构化技能包）
const newPill = await pillService.create({
  name: '文言文金丹',
  description: '令道人开口便是之乎者也',
  skill_schema: { /* expression_dna ... */ },
  tags: ['文言文', '古雅'],
  version: '1.0.0',
});

// 服用金丹（含权重与顺序）
await agentService.bindPill(agentId, { pill_id: 1, weight: 1.0, sort_order: 0 });

// 流式对话
for await (const chunk of chatService.streamMessage(sessionId, message)) {
  if (chunk.type === 'chunk') {
    setText(prev => prev + chunk.content);
  }
}
```

## 开发命令

```bash
# 安装依赖
npm install

# 开发服务器
npm run dev

# 构建生产版本
npm run build

# 预览生产版本
npm run preview

# 代码检查
npm run lint
```

## WebSocket 对话流程

```
1. 建立 WebSocket 连接: ws://host/api/v1/chat/ws/:sessionId
2. 发送用户消息: { "content": "消息内容" }
3. 接收流式响应:
   { "type": "chunk", "content": "片段" }
   { "type": "chunk", "content": "片段" }
   ...
   { "type": "done" }
4. 关闭连接
```
