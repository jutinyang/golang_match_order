# CEX现货撮合系统
本项目是一个中心化交易所（CEX）现货交易的**订单撮合系统**，基于Go语言实现，核心遵循「价格优先、时间优先」的撮合规则，支持限价单、市价单的匹配与成交记录生成。


## 目录结构
```
./
├── engine.go   # 撮合引擎入口（订单簿管理、撮合流程调度）
├── match.go    # 核心撮合逻辑（价格层级遍历、订单匹配、成交计算）
├── model.go    # 核心结构体定义（Order、Trade、PriceLevel、OrderBook等）
└── order.go    # 订单创建
```


## 环境要求
- Go版本：1.21及以上
- 依赖包：
  ```bash
  go get github.com/google/btree  # 价格层级的B树索引依赖
  ```


## 快速启动
1. 克隆项目到本地
2. 编译代码：
   ```bash
   go build -o matching-engine
   ```
3. 运行（需结合业务代码调用撮合接口，示例见「使用示例」）


## 核心功能
1. 支持**限价单**、**市价单**的提交与撮合
2. 遵循「价格优先、时间优先」的撮合规则
3. 自动生成成交记录（包含买卖订单ID、价格、数量等信息）
4. 订单状态自动更新（待成交/部分成交/完全成交）
5. 并发安全（价格层级读写锁保护）


## 代码说明
| 文件         | 功能说明                                                                 |
|--------------|--------------------------------------------------------------------------|
| `engine.go`  | 撮合引擎的核心调度：管理订单簿（买单簿/卖单簿）、触发撮合流程、处理完成订单 |
| `match.go`   | 撮合核心逻辑：遍历价格层级、匹配订单、计算成交数量、生成成交记录           |
| `model.go`   | 定义所有核心结构体（订单Order、成交记录Trade、价格层级PriceLevel等）       |
| `order.go`   | 订单创建                   |


## 使用示例（简易）
```go
package main

import (
	"math/big"
	"time"
	"your-project-path/matching"
)

func main() {
	// 1. 初始化订单簿
	ob := &matching.OrderBook{
		BidTree: btree.New(32), // 买单B树索引
		AskTree: btree.New(32), // 卖单B树索引
	}

	// 2. 创建新订单（限价买单）
	newOrder := &matching.Order{
		OrderID:    "order_001",
		UserID:     "user_001",
		Symbol:     "BTC/USDT",
		Side:       matching.SideBuy,
		Price:      big.NewFloat(10000),
		Quantity:   big.NewFloat(5),
		Remaining:  big.NewFloat(5),
		Status:     matching.StatusPending,
		CreateTime: time.Now().UnixNano(),
		IsMarket:   false,
	}

	// 3. 提交订单并触发撮合（需结合业务逻辑实现订单簿插入、撮合调用）
	// ob.SubmitOrder(newOrder)
}
```


## 注意事项
1. **高精度计算**：所有价格、数量均使用`math/big.Float`，禁止用`float64`避免精度丢失
2. **并发安全**：价格层级使用读写锁，高并发场景下需避免长时间持有锁
3. **市价单处理**：市价单价格字段为0，会自动匹配市场最优价格
4. **异常防御**：使用`big.Float`前需判空，避免空指针panic


## 扩展方向
- 支持止损/止盈订单
- 接入监控系统（撮合延迟、成交指标）
- 分布式撮合（按交易对分片）