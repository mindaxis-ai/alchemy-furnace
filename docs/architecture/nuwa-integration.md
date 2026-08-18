# Nuwa 蒸馏集成

本项目参考 [nuwa-skill](https://github.com/alchaincyf/nuwa-skill) 的公开方法论，
把人物资料研究拆为写作、访谈、决策案例、表达习惯、批评与时间线六个维度，再由已配置的
OpenAI 兼容模型提炼心智模型、决策启发式、表达 DNA、价值观、反模式和诚实边界。

上游是 Agent Skill，不是可直接导入的运行时 SDK。因此集成采用端口与适配器结构：

- `ResearchProvider` 是资料收集端口；默认提供无需密钥的公网搜索适配器，后续可增加
  Tavily、Bing 或企业知识库，而不修改金丹与道人业务。
- `NuwaDistillationService` 只消费标准化资料并产生可编辑草稿，不直接保存数据。
- Go 网关解析用户在设置中配置的正式模型凭证，Python 引擎不使用 Mock 模式。
- 网页内容只作为不可信证据文本处理；抓取限制协议、地址、大小、超时与数量。

方法论来源采用 MIT 许可证，许可信息见仓库根目录 `THIRD_PARTY_NOTICES.md`。
