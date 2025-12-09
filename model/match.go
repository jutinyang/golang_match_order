package model

import (
	"math/big"
	"time"

	"github.com/google/btree"
)

// MatchOrder 撮合订单
func (ob *OrderBook) MatchOrder(newOrder *Order) []*Trade {
	var trades []*Trade
	remaining := new(big.Float).Copy(newOrder.Remaining) // 新订单剩余数量

	// 确定对手方订单簿和价格比较函数
	var oppositeTree *btree.BTree
	var isMatch func(newPrice, oppositePrice *big.Float) bool

	if newOrder.Side == SideBuy {
		oppositeTree = ob.Asks // 买单匹配卖单簿
		isMatch = func(newPrice, oppositePrice *big.Float) bool {
			// 限价买单：对手方卖价 ≤ 买单价格
			// 市价买单：不限制价格，直接匹配
			return newOrder.IsMarket || newPrice.Cmp(oppositePrice) >= 0
		}
	} else {
		oppositeTree = ob.Bids // 卖单匹配买单簿
		isMatch = func(newPrice, oppositePrice *big.Float) bool {
			// 限价卖单：对手方买价 ≥ 卖单价格
			// 市价卖单：不限制价格，直接匹配
			return newOrder.IsMarket || newPrice.Cmp(oppositePrice) <= 0
		}
	}

	// 遍历对手方订单簿的价格层级（从最优价格开始）
	ob.mutex.RLock() // 读锁保护订单簿结构
	defer ob.mutex.RUnlock()

	for levelItem := oppositeTree.Min(); levelItem != nil; levelItem = oppositeTree.Next(levelItem) {
		item := levelItem.(*PriceLevelItem)
		priceLevel := item.Level

		// 检查价格是否匹配
		if !isMatch(newOrder.Price, priceLevel.Price) {
			break // 价格不匹配，退出撮合
		}

		// 遍历该价格层级的所有订单（时间优先）
		priceLevel.mutex.RLock() // 读锁保护该价格层级
		for orderElem := priceLevel.Orders.Front(); orderElem != nil; {
			restingOrder := orderElem.Value.(*Order)
			nextElem := orderElem.Next() // 提前保存下一个节点，避免删除后失效

			// 跳过已完成的订单
			if restingOrder.Status != StatusPending {
				orderElem = nextElem
				continue
			}

			// 计算可成交数量（取两者剩余数量的最小值）
			matchQty := big.NewFloat(0)
			if remaining.Cmp(restingOrder.Remaining) > 0 {
				matchQty.Copy(restingOrder.Remaining) // 对手方订单完全成交
			} else {
				matchQty.Copy(remaining) // 新订单完全成交
			}

			// 生成成交记录
			trade := &Trade{
				TradeID:      uuid.New().String(),
				Symbol:       newOrder.Symbol,
				Price:        priceLevel.Price,
				Quantity:     matchQty,
				MakerOrderID: restingOrder.OrderID, // 挂单者为Maker
				TakerOrderID: newOrder.OrderID,     // 主动成交者为Taker
				MakerUserID:  restingOrder.UserID,
				TakerUserID:  newOrder.UserID,
				TradeTime:    time.Now().UnixNano(),
				Fee:          calculateFee(matchQty, priceLevel.Price), // 计算手续费（0.1%）
			}
			trades = append(trades, trade)

			// 更新订单剩余数量
			remaining.Sub(remaining, matchQty)
			restingOrder.Remaining.Sub(restingOrder.Remaining, matchQty)
			priceLevel.TotalQty.Sub(priceLevel.TotalQty, matchQty)

			// 更新订单状态
			if restingOrder.Remaining.Sign() == 0 {
				// 对手方订单完全成交
				restingOrder.Status = StatusFilled
				restingOrder.UpdateTime = trade.TradeTime
			} else {
				// 对手方订单部分成交
				restingOrder.Status = StatusPartiallyFilled
				restingOrder.UpdateTime = trade.TradeTime
			}

			// 检查新订单是否完全成交
			if remaining.Sign() == 0 {
				newOrder.Status = StatusFilled
				newOrder.Remaining.Set(remaining)
				newOrder.UpdateTime = trade.TradeTime
				priceLevel.mutex.RUnlock()
				// 处理已完成的对手方订单
				ob.processCompletedOrders(priceLevel)
				goto matchDone // 新订单完全成交，退出撮合
			}

			orderElem = nextElem
		}
		priceLevel.mutex.RUnlock()
		// 处理该价格层级的已完成订单
		ob.processCompletedOrders(priceLevel)
	}

	// 新订单未完全成交，插入订单簿
	if remaining.Sign() > 0 {
		newOrder.Remaining.Set(remaining)
		newOrder.Status = StatusPartiallyFilled
		newOrder.UpdateTime = time.Now().UnixNano()
		// 插入订单簿（需写锁，单独调用）
		ob.AddOrder(newOrder)
	}

matchDone:
	ob.lastMatchTime = time.Now().UnixNano()
	return trades
}

// processCompletedOrders 处理已完成的订单（从订单簿中移除）
func (ob *OrderBook) processCompletedOrders(priceLevel *PriceLevel) {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()

	priceLevel.mutex.Lock()
	defer priceLevel.mutex.Unlock()

	for orderElem := priceLevel.Orders.Front(); orderElem != nil; {
		restingOrder := orderElem.Value.(*Order)
		nextElem := orderElem.Next()

		if restingOrder.Status == StatusFilled || restingOrder.Status == StatusCancelled {
			// 从链表中删除订单
			priceLevel.Orders.Remove(orderElem)
			delete(priceLevel.OrderMap, restingOrder.OrderID)
			delete(ob.OrderMap, restingOrder.OrderID)
		}

		orderElem = nextElem
	}

	// 若价格层级无订单，从btree和映射中删除
	if priceLevel.Orders.Len() == 0 {
		priceStr := priceLevel.Price.String()
		delete(ob.PriceLevels, priceStr)
		// 确定订单簿（买单/卖单）
		tree := ob.Asks
		if restingOrder.Side == SideBuy {
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
