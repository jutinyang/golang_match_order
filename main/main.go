package main

import (
	"demo1/model"
	"fmt"
	"math/big"
	"time"
)

func main() {
	TestMarketOrderMatching2()
}

func TestLimitOrderMatching1() {
	// 创建交易引擎
	engine := model.NewMatchingEngine()
	engine.Start()
	defer engine.Stop()

	// 创建买单（限价45000 USDT，数量1 BTC）
	buyOrder := &model.Order{
		OrderID:    "buy-001",
		UserID:     "user-001",
		Symbol:     "BTC/USDT",
		Side:       model.SideBuy,
		Price:      big.NewFloat(45000),
		Quantity:   big.NewFloat(1),
		Remaining:  big.NewFloat(1),
		Status:     model.StatusPending,
		CreateTime: time.Now().UnixNano(),
		IsMarket:   false,
	}

	// 创建卖单（限价44900 USDT，数量0.5 BTC）
	sellOrder1 := &model.Order{
		OrderID:    "sell-001",
		UserID:     "user-002",
		Symbol:     "BTC/USDT",
		Side:       model.SideSell,
		Price:      big.NewFloat(44900),
		Quantity:   big.NewFloat(0.5),
		Remaining:  big.NewFloat(0.5),
		Status:     model.StatusPending,
		CreateTime: time.Now().UnixNano(),
		IsMarket:   false,
	}

	// 创建卖单（限价45000 USDT，数量0.6 BTC）
	sellOrder2 := &model.Order{
		OrderID:    "sell-002",
		UserID:     "user-003",
		Symbol:     "BTC/USDT",
		Side:       model.SideSell,
		Price:      big.NewFloat(45000),
		Quantity:   big.NewFloat(0.6),
		Remaining:  big.NewFloat(0.6),
		Status:     model.StatusPending,
		CreateTime: time.Now().UnixNano(),
		IsMarket:   false,
	}

	// 先添加卖单到订单簿
	engine.OrderChan <- sellOrder1
	engine.OrderChan <- sellOrder2

	// 再添加买单进行撮合
	engine.OrderChan <- buyOrder

	// 等待撮合完成（实际项目中应使用同步机制）
	time.Sleep(100 * time.Millisecond)

	// 检查结果（实际项目中应使用断言）
	fmt.Println("Buy order status:", buyOrder.Status)
	fmt.Println("Buy order remaining:", buyOrder.Remaining.Text('f', 6))
	fmt.Println("Sell order 1 status:", sellOrder1.Status)
	fmt.Println("Sell order 2 status:", sellOrder2.Status)
	fmt.Println("Sell order 2 remaining:", sellOrder2.Remaining.Text('f', 6))
}

func TestMarketOrderMatching2() {
	// 创建交易引擎
	engine := model.NewMatchingEngine()
	engine.Start()
	defer engine.Stop()

	// 创建多个卖单
	for i := 0; i < 5; i++ {
		sellOrder := &model.Order{
			OrderID:    fmt.Sprintf("sell-%03d", i+1),
			UserID:     fmt.Sprintf("user-%03d", i+1),
			Symbol:     "BTC/USDT",
			Side:       model.SideSell,
			Price:      big.NewFloat(45000 + float64(i)*100), // 价格从45000到45400
			Quantity:   big.NewFloat(0.2),
			Remaining:  big.NewFloat(0.2),
			Status:     model.StatusPending,
			CreateTime: time.Now().UnixNano(),
			IsMarket:   false,
		}
		engine.OrderChan <- sellOrder
	}

	// 创建市价买单（数量0.8 BTC）
	marketBuyOrder := &model.Order{
		OrderID:    "market-buy-001",
		UserID:     "user-100",
		Symbol:     "BTC/USDT",
		Side:       model.SideBuy,
		Price:      big.NewFloat(0), // 市价单价格为0
		Quantity:   big.NewFloat(0.8),
		Remaining:  big.NewFloat(0.8),
		Status:     model.StatusPending,
		CreateTime: time.Now().UnixNano(),
		IsMarket:   true,
	}

	// 发送市价单进行撮合
	engine.OrderChan <- marketBuyOrder

	// 等待撮合完成
	time.Sleep(100 * time.Millisecond)

	fmt.Println("Market buy order status:", marketBuyOrder.Status)
	fmt.Println("Market buy order remaining:", marketBuyOrder.Remaining.Text('f', 6))
}
