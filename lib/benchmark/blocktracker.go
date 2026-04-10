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
	Number   int64
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
	cancelFunc      context.CancelFunc
	closeOnce       sync.Once
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
		} else if id == 3 {
			el.handleGasResponse(response)
		}
		// Ignore subscription confirmation (id:1) and other responses.
	}
}

// handleNewHead extracts header info (including gas) from the subscription notification
// and sends a follow-up request for the tx count (id:2).
func (el *EthereumListener) handleNewHead(response map[string]interface{}) {
	params := response["params"].(map[string]interface{})
	result := params["result"].(map[string]interface{})

	blockNo := result["number"].(string)
	blockNum, _ := strconv.ParseInt(blockNo[2:], 16, 64)
	ts, _ := strconv.ParseInt(result["timestamp"].(string)[2:], 16, 64)

	el.pendingBlock = &BlockInfo{Number: blockNum, Time: ts}

	// Request tx count for current block.
	txCountReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "eth_getBlockTransactionCountByNumber",
		"params":  []interface{}{blockNo},
	}
	if err := el.conn.WriteJSON(txCountReq); err != nil {
		log.Println("Failed to send tx count request:", err)
	}

	// Request full previous block for gas info (current block may not be indexed yet).
	if blockNum > 0 {
		prevBlockNo := fmt.Sprintf("0x%x", blockNum-1)
		blockReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "eth_getBlockByNumber",
			"params":  []interface{}{prevBlockNo, false},
		}
		if err := el.conn.WriteJSON(blockReq); err != nil {
			log.Println("Failed to send block request:", err)
		}
	}
}

// handleTxCountResponse processes the tx count and triggers block processing.
func (el *EthereumListener) handleTxCountResponse(response map[string]interface{}) {
	if el.pendingBlock == nil {
		return
	}

	resultStr, ok := response["result"].(string)
	if !ok || len(resultStr) < 3 {
		return
	}
	txCount, _ := strconv.ParseInt(resultStr[2:], 16, 64)

	el.pendingBlock.TxCount = txCount
	block := *el.pendingBlock
	el.pendingBlock = nil

	el.limiter.release(int(txCount))
	el.processBlock(block)
}

// handleGasResponse applies gas info from eth_getBlockByNumber to the matching block in blockStat.
func (el *EthereumListener) handleGasResponse(response map[string]interface{}) {
	result, ok := response["result"].(map[string]interface{})
	if !ok {
		return
	}

	numStr, _ := result["number"].(string)
	blockNum, _ := strconv.ParseInt(numStr[2:], 16, 64)
	gasUsed, _ := strconv.ParseInt(result["gasUsed"].(string)[2:], 16, 64)
	gasLimit, _ := strconv.ParseInt(result["gasLimit"].(string)[2:], 16, 64)

	for i := range el.blockStat {
		if el.blockStat[i].Number == blockNum {
			el.blockStat[i].GasUsed = gasUsed
			el.blockStat[i].GasLimit = gasLimit
			return
		}
	}
}

func (el *EthereumListener) processBlock(block BlockInfo) {
	el.blockStat = append(el.blockStat, block)

	// keep only the last 60 seconds of blocks
	for len(el.blockStat) > 1 && el.blockStat[len(el.blockStat)-1].Time-el.blockStat[0].Time > 60 {
		el.blockStat = el.blockStat[1:]
	}

	timeSpan := el.blockStat[len(el.blockStat)-1].Time - el.blockStat[0].Time
	if timeSpan <= 5 {
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
	fmt.Printf("\rTPS: %-6d  GasUsed: %.2f%%", tps, gasUsedPercent*100)
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
