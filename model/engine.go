package model

import (
	"fmt"
	"sync"
	"time"
)

// NewMatchingEngine 创建新的交易引擎
func NewMatchingEngine() *MatchingEngine {
	return &MatchingEngine{
		OrderBooks: make(map[string]*OrderBook),
		OrderChan:  make(chan *Order, 10000), // 带缓冲的订单通道，避免阻塞
		TradeChan:  make(chan []*Trade, 10000),
		WorkerPool: &sync.Pool{
			New: func() interface{} {
				return make([]*Trade, 0, 100) // 预分配切片容量
			},
		},
		StopChan: make(chan struct{}),
	}
}

// Start 启动交易引擎
func (me *MatchingEngine) Start() {
	// 启动订单处理goroutine
	me.Wg.Add(1)
	go me.orderProcessor()

	// 启动成交处理goroutine
	me.Wg.Add(1)
	go me.tradeProcessor()

	fmt.Println("Matching engine started")
}

// Stop 优雅停止交易引擎（增加超时保护）
func (me *MatchingEngine) Stop() {
	close(me.StopChan)

	// 异步等待协程退出，避免阻塞主线程
	done := make(chan struct{})
	go func() {
		me.Wg.Wait()
		close(done)
	}()

	// 超时控制：1秒内未退出则提示可能死锁
	select {
	case <-done:
		fmt.Println("Matching engine stopped normally")
	case <-time.After(1 * time.Second):
		fmt.Println("Matching engine stopped (timeout: possible deadlock)")
	}
}

// orderProcessor 处理订单请求
func (me *MatchingEngine) orderProcessor() {
	defer me.Wg.Done()

	for {
		select {
		case order := <-me.OrderChan:
			// 获取或创建订单簿
			me.mutex.Lock()
			orderBook, exists := me.OrderBooks[order.Symbol]
			if !exists {
				orderBook = NewOrderBook(order.Symbol)
				me.OrderBooks[order.Symbol] = orderBook
			}
			me.mutex.Unlock()

			// 撮合订单
			trades := orderBook.MatchOrder(order)
			if len(trades) > 0 {
				me.TradeChan <- trades
			}
		case <-me.StopChan:
			return
		}
	}
}

// tradeProcessor 处理成交记录
func (me *MatchingEngine) tradeProcessor() {
	defer me.Wg.Done()

	for {
		select {
		case trades := <-me.TradeChan:
			// 这里可以添加成交后的处理逻辑，如：
			// 1. 发送到Kafka供清算引擎处理
			// 2. 更新行情数据
			// 3. 推送WebSocket通知给用户
			for _, trade := range trades {
				// 修正字段名：Price→TradePrice、Quantity→TradeQty、MakerUserID→BuyUserID、TakerUserID→SellUserID
				fmt.Printf("Trade executed: %s, Price: %s, Quantity: %s, Maker: %s, Taker: %s\n",
					trade.TradeID,
					trade.TradePrice.Text('f', 2), // 原Price→TradePrice
					trade.TradeQty.Text('f', 6),   // 原Quantity→TradeQty
					trade.BuyUserID,               // 原MakerUserID→BuyUserID
					trade.SellUserID,              // 原TakerUserID→SellUserID
				)
			}
			// 归还切片到对象池
			me.WorkerPool.Put(trades[:0])
		case <-me.StopChan:
			return
		}
	}
}
