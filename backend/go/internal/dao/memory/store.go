// Package memory 内存 DAO 实现(007-demo-mode)
//
// 在 DEMO_MODE=true 时替代 internal/dao/* 的 GORM 实现,满足同一组
// internal/interface/dao 接口,无需触及 service 层逻辑。
//
// 并发安全:每张"表"一把 sync.RWMutex;写操作持锁,读操作持 RLock。
// 数据不持久化:进程退出即丢失,符合 spec FR-013。
package memory

import (
	"sync"

	"github.com/alchemy-furnace/server/model"
)

// Store 全局内存仓库,演示模式启动时构造,持有所有实体的内存副本
type Store struct {
	muPill     sync.RWMutex
	muAgent    sync.RWMutex
	muChat     sync.RWMutex
	muProvider sync.RWMutex
	muModel    sync.RWMutex
	muPattern  sync.RWMutex
	muAp       sync.RWMutex // agent_pill binding

	pills     map[string]*model.ElixirPill     // key: uuid.String()
	agents    map[string]*model.DaoAgent       // key: uuid.String()
	sessions  map[string]*model.ChatSession    // key: uuid.String()
	messages  map[uint][]*model.ChatMessage    // key: sessionID,内按时间正序
	members   map[uint][]*model.SessionMember  // key: sessionID,内按 SortOrder 正序
	providers map[string]*model.LLMProvider    // key: uuid.String()
	models    map[string]*model.LLMModel       // key: uuid.String()
	patterns  map[uint]*model.LanguagePattern  // key: agentID
	agentPill map[uint]map[uint]*model.AgentPill // key: agentID -> pillID -> binding

	nextPillID     uint
	nextAgentID    uint
	nextSessionID  uint
	nextMessageID  uint
	nextMemberID   uint
	nextProviderID uint
	nextModelID    uint
	nextPatternID  uint
	nextApID       uint
}

// NewStore 构造空内存仓库
func NewStore() *Store {
	return &Store{
		pills:     map[string]*model.ElixirPill{},
		agents:    map[string]*model.DaoAgent{},
		sessions:  map[string]*model.ChatSession{},
		messages:  map[uint][]*model.ChatMessage{},
		members:   map[uint][]*model.SessionMember{},
		providers: map[string]*model.LLMProvider{},
		models:    map[string]*model.LLMModel{},
		patterns:  map[uint]*model.LanguagePattern{},
		agentPill: map[uint]map[uint]*model.AgentPill{},
	}
}

// NewMemorySeed 构造并填充预置 9 条各类数据;数据来源见 seed.go
func NewMemorySeed() *Store {
	s := NewStore()
	loadSeed(s)
	return s
}

// sharedStore 演示模式单例 store。
// 注意:必须用 sync.Once 惰性构造,不能写成包级 var 初始化器 ——
// seed.go 的 UUID 数组在 func init() 里赋值,而包级 var 初始化器先于
// 所有 init() 执行,届时 pillUUID 等仍为零值,会导致 9 条数据共享
// 同一个零 UUID 键、互相覆盖只剩 1 条。惰性构造推迟到运行期(wire
// 装配 DAO 时)调用,此时 init() 已跑完,UUID 已就绪。
var (
	sharedStoreOnce sync.Once
	sharedStore     *Store
)

// SharedStore 返回演示模式单例 store(首次调用时填充种子数据)
func SharedStore() *Store {
	sharedStoreOnce.Do(func() {
		sharedStore = NewMemorySeed()
	})
	return sharedStore
}
