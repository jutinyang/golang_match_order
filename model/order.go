package model

import (
	"container/list"
	"fmt"
	"math/big"
	"time"

	"github.com/google/btree"
)

// NewOrderBook 创建新的订单簿
func NewOrderBook(symbol string) *OrderBook {
	return &OrderBook{
		Symbol:        symbol,
		Bids:          btree.New(32), // 32是btree的度，可根据需求调整
		Asks:          btree.New(32),
		PriceLevels:   make(map[string]*PriceLevel),
		OrderMap:      make(map[string]*Order),
		lastMatchTime: time.Now().UnixNano(),
	}
}

// AddOrder 添加订单到订单簿
func (ob *OrderBook) AddOrder(order *Order) error {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	// 检查订单是否已存在
	if _, exists := ob.OrderMap[order.OrderID]; exists {
		return fmt.Errorf("order already exists: %s", order.OrderID)
	}

	// 确定订单簿（买单/卖单）
	tree := ob.Asks
	if order.Side == SideBuy {
		tree = ob.Bids
	}

	// 检查价格层级是否存在
	priceStr := order.Price.String()
	level, exists := ob.PriceLevels[priceStr]
	if !exists {
		// 创建新价格层级
		level = &PriceLevel{
			Price:    order.Price,
			TotalQty: big.NewFloat(0),
			Orders:   list.New(),
			OrderMap: make(map[string]*list.Element),
		}
		ob.PriceLevels[priceStr] = level
		// 添加到btree
		tree.ReplaceOrInsert(&PriceLevelItem{Price: order.Price, Level: level})
	}

	// 更新价格层级总数量
	level.mutex.Lock()
	defer level.mutex.Unlock()
	level.TotalQty.Add(level.TotalQty, order.Remaining)

	// 添加订单到链表尾部（时间优先）
	elem := level.Orders.PushBack(order)
	level.OrderMap[order.OrderID] = elem

	// 添加到全局订单映射
	ob.OrderMap[order.OrderID] = order

	return nil
}

// CancelOrder 取消订单
func (ob *OrderBook) CancelOrder(orderID string) error {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	// 查找订单
	order, exists := ob.OrderMap[orderID]
	if !exists {
		return fmt.Errorf("order not found: %s", orderID)
	}

	// 检查订单状态
	if order.Status != StatusPending && order.Status != StatusPartiallyFilled {
		return fmt.Errorf("order cannot be cancelled: %s, status: %s", orderID, order.Status)
	}

	// 查找价格层级
	priceStr := order.Price.String()
	level, exists := ob.PriceLevels[priceStr]
	if !exists {
		return fmt.Errorf("price level not found: %s", priceStr)
	}

	// 从价格层级中删除订单
	level.mutex.Lock()
	defer level.mutex.Unlock()

	elem, exists := level.OrderMap[orderID]
	if !exists {
		return fmt.Errorf("order not found in price level: %s", orderID)
	}

	// 从链表中删除
	level.Orders.Remove(elem)
	delete(level.OrderMap, orderID)

	// 更新价格层级总数量
	level.TotalQty.Sub(level.TotalQty, order.Remaining)

	// 若价格层级无订单，从btree和映射中删除
	if level.Orders.Len() == 0 {
		delete(ob.PriceLevels, priceStr)
		tree := ob.Asks
		if order.Side == SideBuy {
			tree = ob.Bids
		}
		tree.Delete(&PriceLevelItem{Price: order.Price, Level: level})
	}

	// 更新订单状态
	order.Status = StatusCancelled
	order.UpdateTime = time.Now().UnixNano()

	// 从全局订单映射中删除
	delete(ob.OrderMap, orderID)

	return nil
}
