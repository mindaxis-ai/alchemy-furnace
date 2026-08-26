"""Research protocol for the Nuwa distillation workflow.

The distiller depends on :class:`ResearchProvider`, not on a search vendor.
Adding Tavily, Bing, Serper, or an enterprise connector therefore does not
change pill or agent services (open/closed principle).

Every provider returns a :class:`ResearchReport` with per-provider
:class:`ResearchAttempt` diagnostics; failures raise :class:`ResearchError`
with stable machine-readable codes. ``ResearchCredentials.api_key`` never
leaks into repr, logs, attempts or exceptions.
"""
from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import asdict, dataclass, field
from enum import Enum
from urllib.parse import urlparse


class EvidenceLevel(str, Enum):
    INSUFFICIENT = "insufficient"
    LIMITED = "limited"
    STANDARD = "standard"


@dataclass(frozen=True)
class ResearchDocument:
    title: str
    url: str
    excerpt: str
    dimension: str


@dataclass(frozen=True)
class ResearchAttempt:
    provider: str
    status: str
    discovered: int
    accepted: int
    reason: str | None


@dataclass
class ResearchReport:
    documents: list[ResearchDocument]
    attempts: list[ResearchAttempt]
    evidence_level: EvidenceLevel
    warnings: list[str] = field(default_factory=list)

    @property
    def total_characters(self) -> int:
        return sum(len(item.excerpt) for item in self.documents)

    @property
    def domain_count(self) -> int:
        return len({urlparse(item.url).hostname for item in self.documents})

    def public_summary(self) -> dict:
        """对外可见元数据：attempts、文档数、域名数、字符数与警告。

        不含正文摘录，不含任何凭证字段，可安全进入错误 details 与日志。
        """
        return {
            "attempts": [asdict(item) for item in self.attempts],
            "document_count": len(self.documents),
            "domain_count": self.domain_count,
            "total_characters": self.total_characters,
            "warnings": list(self.warnings),
        }


class ResearchError(RuntimeError):
    """Stable, machine-readable research failure.

    ``code``/``stage``/``retryable`` travel unchanged to the Go gateway and
    frontend; ``attempts`` carries per-provider diagnostics for logging.
    """

    stage = "research"

    def __init__(self, code: str, message: str, retryable: bool, attempts: list[ResearchAttempt]):
        super().__init__(message)
        self.code = code
        self.message = message
        self.retryable = retryable
        self.attempts = attempts


@dataclass(frozen=True)
class ResearchCredentials:
    model: str
    base_url: str
    api_key: str = field(repr=False)


class ResearchProvider(ABC):
    @abstractmethod
    def collect(
        self,
        subject: str,
        brief: str,
        locale: str = "zh-CN",
        credentials: ResearchCredentials | None = None,
    ) -> ResearchReport:
        """Collect bounded public evidence for one subject, with diagnostics."""
        raise NotImplementedError
