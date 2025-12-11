package main

import (
	"container/list"
	"fmt"
	"math/big"
	"sync"

	"github.com/google/btree"
)

// ======================== 核心结构体定义 ========================
// Order 订单结构体（简化版）
type Order struct {
	ID       string     // 订单ID
	Price    *big.Float // 价格
	Quantity *big.Float // 数量
}

// PriceLevel 价格层级结构体（同一价格的订单集合）
type PriceLevel struct {
	Price    *big.Float               // 价格
	TotalQty *big.Float               // 该价格的总数量（深度图使用）
	Orders   *list.List               // 同价格订单链表（时间优先）
	OrderMap map[string]*list.Element // 订单ID到链表节点的映射（O(1)删除）
	mutex    sync.RWMutex             // 读写锁，保护该价格层级
}

// NewPriceLevel 创建价格层级
func NewPriceLevel(price *big.Float) *PriceLevel {
	return &PriceLevel{
		Price:    price,
		TotalQty: big.NewFloat(0),
		Orders:   list.New(),
		OrderMap: make(map[string]*list.Element),
	}
}

// AddOrder 给价格层级添加订单
func (pl *PriceLevel) AddOrder(order *Order) {
	pl.mutex.Lock()
	defer pl.mutex.Unlock()

	// 1. 订单加入链表尾部（时间优先）
	elem := pl.Orders.PushBack(order)
	// 2. 记录订单ID到链表节点的映射
	pl.OrderMap[order.ID] = elem
	// 3. 更新总数量
	pl.TotalQty.Add(pl.TotalQty, order.Quantity)
}

// PriceLevelItem 价格层级B树项（适配btree排序）
type PriceLevelItem struct {
	Price *big.Float  // 排序用的价格
	Level *PriceLevel // 关联的实际价格层级
	IsBid bool        // true=买单簿（降序），false=卖单簿（升序）

}

// 实现btree.Item的Less方法（方法名必须是Less）
func (p *PriceLevelItem) Less(than btree.Item) bool {
	other := than.(*PriceLevelItem) // 现在PriceLevelItem实现了Item，断言合法
	if p.IsBid {
		// 买单簿：价格降序（当前价格>其他价格则排前面）
		return p.Price.Cmp(other.Price) > 0
	} else {
		// 卖单簿：价格升序（当前价格<其他价格则排前面）
		return p.Price.Cmp(other.Price) < 0
	}
}

// ======================== 演示逻辑 ========================
func main() {
	// 1. 初始化B树（度为3，适合高频交易场景）
	asksTree := btree.New(3) // 卖单簿（升序）
	bidsTree := btree.New(3) // 买单簿（降序）

	// 2. 模拟创建价格层级 + 包装成PriceLevelItem
	// ---------------- 卖单簿示例（Asks：价格 9900、10000、10100） ----------------
	levelAsk9900 := NewPriceLevel(big.NewFloat(9900))
	levelAsk9900.AddOrder(&Order{ID: "sell1", Price: big.NewFloat(9900), Quantity: big.NewFloat(2)})
	// 卖单簿Item（IsBid=false，升序）
	itemAsk9900 := &PriceLevelItem{
		Price: big.NewFloat(9900),
		Level: levelAsk9900,
		IsBid: false,
	}

	levelAsk10000 := NewPriceLevel(big.NewFloat(10000))
	levelAsk10000.AddOrder(&Order{ID: "sell2", Price: big.NewFloat(10000), Quantity: big.NewFloat(3)})
	itemAsk10000 := &PriceLevelItem{Price: big.NewFloat(10000), Level: levelAsk10000}

	levelAsk10100 := NewPriceLevel(big.NewFloat(10100))
	levelAsk10100.AddOrder(&Order{ID: "sell3", Price: big.NewFloat(10100), Quantity: big.NewFloat(1)})
	itemAsk10100 := &PriceLevelItem{Price: big.NewFloat(10100), Level: levelAsk10100}

	// ---------------- 买单簿示例（Bids：价格 9950、10050、10150） ----------------
	levelBid9950 := NewPriceLevel(big.NewFloat(9950))
	levelBid9950.AddOrder(&Order{ID: "buy1", Price: big.NewFloat(9950), Quantity: big.NewFloat(4)})
	itemBid9950 := &PriceLevelItem{Price: big.NewFloat(9950), Level: levelBid9950}

	levelBid10050 := NewPriceLevel(big.NewFloat(10050))
	levelBid10050.AddOrder(&Order{ID: "buy2", Price: big.NewFloat(10050), Quantity: big.NewFloat(5)})
	itemBid10050 := &PriceLevelItem{Price: big.NewFloat(10050), Level: levelBid10050}

	levelBid10150 := NewPriceLevel(big.NewFloat(10150))
	levelBid10150.AddOrder(&Order{ID: "buy3", Price: big.NewFloat(10150), Quantity: big.NewFloat(2)})

	itemBid10150 := &PriceLevelItem{
		Price: big.NewFloat(10150),
		Level: levelBid10150,
		IsBid: true,
	}

	// 3. 存入B树（注意：卖单簿用LessForAsks，买单簿用LessForBids）
	// 卖单簿：按LessForAsks升序存储
	asksTree.ReplaceOrInsert(itemAsk10000) // 先插10000
	asksTree.ReplaceOrInsert(itemAsk10100) // 再插10100
	asksTree.ReplaceOrInsert(itemAsk9900)  // 最后插9900（验证自动排序）

	// 买单簿：按LessForBids降序存储
	bidsTree.ReplaceOrInsert(itemBid9950)  // 先插9950
	bidsTree.ReplaceOrInsert(itemBid10150) // 再插10150
	bidsTree.ReplaceOrInsert(itemBid10050) // 最后插10050（验证自动排序）

	// 4. 遍历卖单簿（升序：9900 → 10000 → 10100）
	fmt.Println("===== 卖单簿（Asks）- 价格升序遍历 =====")
	asksTree.Ascend(func(item btree.Item) bool {
		pli := item.(*PriceLevelItem)
		// 通过PriceLevelItem访问关联的PriceLevel
		fmt.Printf("价格：%s | 总数量：%s | 订单ID：%s\n",
			pli.Price.Text('f', 0),
			pli.Level.TotalQty.Text('f', 0),
			pli.Level.Orders.Front().Value.(*Order).ID)
		return true
	})

	// 5. 遍历买单簿（降序：10150 → 10050 → 9950）
	fmt.Println("\n===== 买单簿（Bids）- 价格降序遍历 =====")
	bidsTree.Ascend(func(item btree.Item) bool {
		pli := item.(*PriceLevelItem)
		fmt.Printf("价格：%s | 总数量：%s | 订单ID：%s\n",
			pli.Price.Text('f', 0),
			pli.Level.TotalQty.Text('f', 0),
			pli.Level.Orders.Front().Value.(*Order).ID)
		return true
	})

	// 6. 演示：通过PriceLevelItem快速操作PriceLevel的订单
	fmt.Println("\n===== 操作PriceLevel中的订单 =====")
	// 从卖单簿取9900的PriceLevelItem
	var targetItem *PriceLevelItem
	asksTree.Ascend(func(item btree.Item) bool {
		pli := item.(*PriceLevelItem)
		if pli.Price.Cmp(big.NewFloat(9900)) == 0 {
			targetItem = pli
			return false // 找到后停止遍历
		}
		return true
	})
	// 给9900的价格层级新增订单
	newOrder := &Order{ID: "sell4", Price: big.NewFloat(9900), Quantity: big.NewFloat(5)}
	targetItem.Level.AddOrder(newOrder)
	fmt.Printf("给价格9900新增订单sell4后，总数量变为：%s\n", targetItem.Level.TotalQty.Text('f', 0))
}
