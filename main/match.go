package main

import (
	"math/big"
	"time"

	"github.com/google/btree"
)

// MatchOrder 撮合订单（修复类型不匹配问题）
func (ob *OrderBook) MatchOrder(newOrder *Order) []*Trade {
	var trades []*Trade
	remaining := new(big.Float).Copy(newOrder.Remaining) // 新订单剩余数量
	matchCompleted := false

	// 确定对手方订单簿和价格比较函数
	var oppositeTree *btree.BTree
	var isMatch func(newPrice, oppositePrice *big.Float) bool

	if newOrder.Side == SideBuy {
		oppositeTree = ob.Asks // 买单匹配卖单簿（升序遍历，从最低卖价开始）
		isMatch = func(newPrice, oppositePrice *big.Float) bool {
			return newOrder.IsMarket || newPrice.Cmp(oppositePrice) >= 0
		}
		// 直接调用Ascend方法，传入ItemIterator类型的回调
		oppositeTree.Ascend(func(item btree.Item) bool {
			return ob.traversePriceLevel(item, newOrder, remaining, &trades, &matchCompleted, isMatch)
		})
	} else {
		oppositeTree = ob.Bids // 卖单匹配买单簿（降序遍历，从最高买价开始）
		isMatch = func(newPrice, oppositePrice *big.Float) bool {
			return newOrder.IsMarket || newPrice.Cmp(oppositePrice) <= 0
		}
		// 直接调用Descend方法，传入ItemIterator类型的回调
		oppositeTree.Descend(func(item btree.Item) bool {
			return ob.traversePriceLevel(item, newOrder, remaining, &trades, &matchCompleted, isMatch)
		})
	}

	// 新订单未完全成交，插入订单簿
	if !matchCompleted && remaining.Sign() > 0 {
		newOrder.Remaining.Set(remaining)
		newOrder.Status = StatusPartiallyFilled
		newOrder.UpdateTime = time.Now().UnixNano()
		ob.AddOrder(newOrder)
	}

	ob.lastMatchTime = time.Now().UnixNano()
	return trades
}

// 遍历价格层级：优化锁释放时机，避免读锁未释放时调用写锁逻辑
func (ob *OrderBook) traversePriceLevel(
	item btree.Item,
	newOrder *Order,
	remaining *big.Float,
	trades *[]*Trade,
	matchCompleted *bool,
	isMatch func(*big.Float, *big.Float) bool,
) bool {
	levelItem := item.(*PriceLevelItem)
	priceLevel := levelItem.Level

	if !isMatch(newOrder.Price, priceLevel.Price) {
		return false
	}

	// 手动获取读锁（不使用defer，避免后续操作持续持有）
	priceLevel.mutex.RLock()

	for orderElem := priceLevel.Orders.Front(); orderElem != nil; {
		restingOrder := orderElem.Value.(*Order)
		nextElem := orderElem.Next()

		if restingOrder.Status != StatusPending {
			orderElem = nextElem
			continue
		}

		// 计算成交数量、生成成交记录（逻辑保持不变）
		matchQty := big.NewFloat(0)
		if remaining.Cmp(restingOrder.Remaining) > 0 {
			matchQty.Copy(restingOrder.Remaining)
		} else {
			matchQty.Copy(remaining)
		}

		trade := &Trade{ /* 成交记录逻辑保持不变 */ }
		*trades = append(*trades, trade)

		// 更新剩余数量和订单状态（逻辑保持不变）
		remaining.Sub(remaining, matchQty)
		restingOrder.Remaining.Sub(restingOrder.Remaining, matchQty)
		priceLevel.TotalQty.Sub(priceLevel.TotalQty, matchQty)

		if restingOrder.Remaining.Sign() == 0 {
			restingOrder.Status = StatusFilled
			restingOrder.UpdateTime = trade.TradeTime
		} else {
			restingOrder.Status = StatusPartiallyFilled
			restingOrder.UpdateTime = trade.TradeTime
		}

		// 新订单完全成交：立即释放读锁，再调用processCompletedOrders
		if remaining.Sign() == 0 {
			newOrder.Status = StatusFilled
			newOrder.Remaining.Set(remaining)
			newOrder.UpdateTime = trade.TradeTime

			priceLevel.mutex.RUnlock() // 提前释放读锁
			ob.processCompletedOrders(priceLevel)
			*matchCompleted = true
			return false
		}

		orderElem = nextElem
	}

	// 遍历完成：释放读锁后再处理已完成订单
	priceLevel.mutex.RUnlock()
	ob.processCompletedOrders(priceLevel)
	return true
}

// 处理已完成订单：拆分锁逻辑，避免同时持有多个锁
func (ob *OrderBook) processCompletedOrders(priceLevel *PriceLevel) {
	// 步骤1：仅持有价格层级锁，收集已完成订单
	priceLevel.mutex.Lock()
	var completedOrders []*Order
	var restingOrder *Order
	for orderElem := priceLevel.Orders.Front(); orderElem != nil; {
		restingOrder = orderElem.Value.(*Order)
		nextElem := orderElem.Next()

		// 收集已完成/已取消的订单
		if restingOrder.Status == StatusFilled || restingOrder.Status == StatusCancelled {
			priceLevel.Orders.Remove(orderElem)
			delete(priceLevel.OrderMap, restingOrder.OrderID)
			completedOrders = append(completedOrders, restingOrder)
		}

		orderElem = nextElem
	}
	priceLevel.mutex.Unlock() // 先释放价格层级锁

	// 步骤2：持有订单簿锁，更新全局订单映射
	ob.mutex.Lock()
	for _, order := range completedOrders {
		delete(ob.OrderMap, order.OrderID)
	}
	ob.mutex.Unlock() // 及时释放订单簿锁

	// 步骤3：再次检查价格层级是否为空（需重新加锁）
	priceLevel.mutex.Lock()
	defer priceLevel.mutex.Unlock()
	if priceLevel.Orders.Len() == 0 {
		priceStr := priceLevel.Price.String()
		ob.mutex.Lock()
		delete(ob.PriceLevels, priceStr)
		ob.mutex.Unlock()

		// 从BTree中删除空价格层级
		tree := ob.Asks
		if restingOrder != nil && restingOrder.Side == SideBuy {
			tree = ob.Bids
		}
		tree.Delete(&PriceLevelItem{Price: priceLevel.Price, Level: priceLevel})
	}
}

// calculateFee 计算交易手续费（0.1%，Taker支付）
func calculateFee(quantity, price *big.Float) *big.Float {
	amount := big.NewFloat(0).Mul(quantity, price)
	feeRate := big.NewFloat(0.001) // 0.1%
	return big.NewFloat(0).Mul(amount, feeRate)
}
