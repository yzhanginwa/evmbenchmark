package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
)

type BlockInfo struct {
	Time     int64
	TxCount  int64
	GasUsed  int64
	GasLimit int64
}

type EthereumListener struct {
	wsURL     string
	conn      *websocket.Conn
	limiter   *rateLimiter
	blockStat []BlockInfo
	quit      chan struct{}
	bestTPS   int64
	gasUsedAtBestTPS float64

	pendingBlock  *BlockInfo // header info waiting for tx count
	tpsLineActive bool
	cancelFunc    context.CancelFunc
	closeOnce     sync.Once
}

func newEthereumListener(wsURL string, limiter *rateLimiter, cancel context.CancelFunc) *EthereumListener {
	return &EthereumListener{
		wsURL:      wsURL,
		limiter:    limiter,
		cancelFunc: cancel,
		quit:       make(chan struct{}),
	}
}

func (el *EthereumListener) Connect() error {
	conn, _, err := websocket.DefaultDialer.Dial(el.wsURL, http.Header{})
	if err != nil {
		return fmt.Errorf("dial error: %v", err)
	}
	el.conn = conn
	return nil
}

func (el *EthereumListener) SubscribeNewHeads() error {
	subscribeMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "eth_subscribe",
		"params":  []interface{}{"newHeads"},
	}
	err := el.conn.WriteJSON(subscribeMsg)
	if err != nil {
		return fmt.Errorf("subscribe error: %v", err)
	}

	go el.listenForMessages()

	return nil
}

func (el *EthereumListener) listenForMessages() {
	for {
		_, message, err := el.conn.ReadMessage()
		if err != nil {
			return
		}

		var response map[string]interface{}
		err = json.Unmarshal(message, &response)
		if err != nil {
			log.Println("unmarshal error:", err)
			continue
		}

		if method, ok := response["method"]; ok && method == "eth_subscription" {
			el.handleNewHead(response)
		} else if id, _ := response["id"].(float64); id == 2 {
			el.handleTxCountResponse(response)
		}
		// Ignore subscription confirmation (id:1) and other responses.
	}
}

// handleNewHead extracts block header info from the subscription notification
// and requests the transaction count separately.
func (el *EthereumListener) handleNewHead(response map[string]interface{}) {
	params := response["params"].(map[string]interface{})
	result := params["result"].(map[string]interface{})

	blockNo := result["number"].(string)
	ts, _ := strconv.ParseInt(result["timestamp"].(string)[2:], 16, 64)
	gasUsed, _ := strconv.ParseInt(result["gasUsed"].(string)[2:], 16, 64)
	gasLimit, _ := strconv.ParseInt(result["gasLimit"].(string)[2:], 16, 64)

	el.pendingBlock = &BlockInfo{Time: ts, GasUsed: gasUsed, GasLimit: gasLimit}

	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "eth_getBlockTransactionCountByNumber",
		"params":  []interface{}{blockNo},
	}
	err := el.conn.WriteJSON(request)
	if err != nil {
		log.Println("Failed to send tx count request:", err)
	}
}

// handleTxCountResponse processes the tx count response and completes the block info.
func (el *EthereumListener) handleTxCountResponse(response map[string]interface{}) {
	if el.pendingBlock == nil {
		return
	}

	resultStr, ok := response["result"].(string)
	if !ok || len(resultStr) < 3 {
		return
	}
	txCount, _ := strconv.ParseInt(resultStr[2:], 16, 64)

	block := *el.pendingBlock
	block.TxCount = txCount
	el.pendingBlock = nil

	el.limiter.release(int(txCount))
	el.processBlock(block)
}

func (el *EthereumListener) processBlock(block BlockInfo) {
	el.blockStat = append(el.blockStat, block)

	// keep only the last 60 seconds of blocks
	for len(el.blockStat) > 1 && el.blockStat[len(el.blockStat)-1].Time-el.blockStat[0].Time > 60 {
		el.blockStat = el.blockStat[1:]
	}

	timeSpan := el.blockStat[len(el.blockStat)-1].Time - el.blockStat[0].Time
	if timeSpan <= 30 {
		return
	}

	totalTxCount := int64(0)
	totalGasLimit := int64(0)
	totalGasUsed := int64(0)
	for _, b := range el.blockStat {
		totalTxCount += b.TxCount
		totalGasLimit += b.GasLimit
		totalGasUsed += b.GasUsed
	}
	tps := totalTxCount / timeSpan
	var gasUsedPercent float64
	if totalGasLimit > 0 {
		gasUsedPercent = float64(totalGasUsed) / float64(totalGasLimit)
	}
	if tps > el.bestTPS {
		el.bestTPS = tps
		el.gasUsedAtBestTPS = gasUsedPercent
	}
	fmt.Printf("\rTPS: %-6d  GasUsed: %-6.2f%%", tps, gasUsedPercent*100)
	el.tpsLineActive = true

	if totalTxCount < 100 {
		el.Close()
		return
	}

	// exit early if last 3 blocks are all empty
	if len(el.blockStat) >= 3 {
		for i := 1; i <= 3; i++ {
			if el.blockStat[len(el.blockStat)-i].TxCount != 0 {
				return
			}
		}
		el.Close()
	}
}

func (el *EthereumListener) printSummary() {
	if el.tpsLineActive {
		fmt.Println()
		el.tpsLineActive = false
	}
	fmt.Printf("Best TPS: %d  GasUsed: %.2f%%\n", el.bestTPS, el.gasUsedAtBestTPS*100)
}

func (el *EthereumListener) Close() {
	el.closeOnce.Do(func() {
		el.printSummary()
		el.cancelFunc()
		if el.conn != nil {
			el.conn.Close()
		}
		close(el.quit)
	})
}
