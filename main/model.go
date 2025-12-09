package main

import (
	"container/list"
	"math/big"
	"sync"
	"time"

	"github.com/google/btree"
)

// 订单方向
const (
	SideBuy  = "buy"
	SideSell = "sell"
)

// 订单状态
const (
	StatusPending         = "pending"          // 待成交
	StatusPartiallyFilled = "partially_filled" // 部分成交
	StatusFilled          = "filled"           // 完全成交
	StatusCancelled       = "cancelled"        // 已取消
)

// 订单结构体
type Order struct {
	OrderID    string     // 唯一订单ID
	UserID     string     // 用户ID
	Symbol     string     // 交易对（如BTC/USDT）
	Side       string     // 方向：buy/sell
	Price      *big.Float // 价格（高精度，避免浮点数误差）
	Quantity   *big.Float // 原始数量
	Remaining  *big.Float // 剩余数量
	Status     string     // 订单状态
	CreateTime int64      // 创建时间（纳秒级，时间优先）
	UpdateTime int64      // 更新时间
	IsMarket   bool       // 是否为市价单（市价单价格为0）
}

// 成交记录结构体
type Trade struct {
	TradeID      string     // 唯一成交ID
	Symbol       string     // 交易对
	Price        *big.Float // 成交价格
	Quantity     *big.Float // 成交数量
	MakerOrderID string     // 挂单者订单ID（被动成交）
	TakerOrderID string     // 主动成交者订单ID
	MakerUserID  string     // 挂单者用户ID
	TakerUserID  string     // 主动成交者用户ID
	TradeTime    int64      // 成交时间（纳秒级）
	Fee          *big.Float // 交易手续费（Taker支付）
}

// 价格层级结构体（同一价格的订单集合）
type PriceLevel struct {
	Price    *big.Float               // 价格
	TotalQty *big.Float               // 该价格的总数量（深度图使用）
	Orders   *list.List               // 同价格订单链表（时间优先，链表头为最早订单）
	OrderMap map[string]*list.Element // 订单ID到链表节点的映射（O(1)删除）
	mutex    sync.RWMutex             // 读写锁，保护该价格层级
}

// 价格层级比较器（用于btree排序）
type PriceLevelItem struct {
	Price *big.Float
	Level *PriceLevel
}

// Less 方法：买单簿按价格降序，卖单簿按价格升序
func (p *PriceLevelItem) Less(than btree.Item) bool {
	other := than.(*PriceLevelItem)
	// 注意：btree默认升序，买单簿需反转比较结果
	return p.Price.Cmp(other.Price) < 0
}

// 内存订单簿结构体
type OrderBook struct {
	Symbol        string                 // 交易对
	Bids          *btree.BTree           // 买单树（价格降序）
	Asks          *btree.BTree           // 卖单树（价格升序）
	PriceLevels   map[string]*PriceLevel // 价格到PriceLevel的映射（O(1)访问）
	OrderMap      map[string]*Order      // 全局订单ID映射（O(1)查询订单）
	mutex         sync.RWMutex           // 订单簿全局锁（用于跨价格层级操作）
	lastMatchTime int64                  // 最后撮合时间（性能监控）
}

// 交易引擎结构体
type MatchingEngine struct {
	OrderBooks   map[string]*OrderBook // 交易对到订单簿的映射
	OrderChan    chan *Order           // 订单请求通道（带缓冲）
	TradeChan    chan []*Trade         // 成交结果通道
	WorkerPool   *sync.Pool            // 撮合结果处理池
	Wg           sync.WaitGroup        // 等待所有goroutine结束
	StopChan     chan struct{}         // 停止信号
	mutex        sync.RWMutex          // 订单簿全局锁（用于跨价格层级操作）
	OrderCount   int64                 // 总订单数
	TradeCount   int64                 // 总成交数
	MatchLatency time.Duration         // 平均撮合延迟
}
