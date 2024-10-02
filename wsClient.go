package main

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gorilla/websocket"
	"net/http"
	"strings"
	"time"
)

func main() {
	fmt.Println("Ws Client started")

	go subscribeNodePending()
	subscribePending()
}

func subscribeLogs() {
	var wsClient, er = ethclient.Dial("ws://localhost:8546/")
	if er != nil {
		e := "Failed to connect to RPC. error: " + er.Error()
		fmt.Println(e)
		return
	}
	logs := make(chan types.Log)
	sub, err := wsClient.SubscribeFilterLogs(context.Background(), ethereum.FilterQuery{}, logs)
	if err != nil {
		e := "Failed to subscribe. error: " + err.Error()
		fmt.Println(e)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case ee := <-sub.Err():
			eee := "Failed to subscribe. error: " + ee.Error()
			fmt.Println(eee)
			break
		case vLog := <-logs:
			fmt.Printf("[%s] TxLog %d %s: %s \n", time.Now().String(), vLog.BlockNumber, vLog.Address, vLog.TxHash)
		}
	}
}

func subscribeNodePending() {
	var wsClient, er = rpc.Dial("ws://localhost:8546/")
	if er != nil {
		e := "Failed to connect to RPC. error: " + er.Error()
		fmt.Println(e)
		return
	}
	var gethClient = gethclient.New(wsClient)
	hashes := make(chan common.Hash)
	sub, err := gethClient.SubscribePendingTransactions(context.Background(), hashes)
	if err != nil {
		e := "Failed to subscribe. error: " + err.Error()
		fmt.Println(e)
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case ee := <-sub.Err():
			eee := "Failed to subscribe. error: " + ee.Error()
			fmt.Println(eee)
			break
		case hash := <-hashes:
			fmt.Printf("[%s] Pending %s \n", time.Now().String(), hash)
		}
	}
}

func subscribePending() {
	dialerGateway := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
	}

	wsPendingSubscriber, _, err := dialerGateway.Dial("ws://localhost:28333/ws", http.Header{"Authorization": []string{"ZjQzNmI3ZDAtMTE0YS00NTAwLWI1NGQtN2UzZTcyMzMxNDdkOmJkZTYzOWYwYmQ0ZDJmYmQ5MzA3ZjBlZWQwNmE4MjMy"}})
	if err != nil {
		panic(fmt.Sprintf("Create Pending Subscription failed error: %s", err.Error()))
	}
	pendingSubRequest := fmt.Sprintf(`{"id": %d, "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash", "tx_contents", "raw_tx"], "filters": "to in [%s]", "blockchain_network": "Polygon-Mainnet"}]}`, 0, strings.Join([]string{"0xa5e0829caced8ffdd4de3c43696c57f7d7a678ff"}, ","))

	err = wsPendingSubscriber.WriteMessage(websocket.TextMessage, []byte(pendingSubRequest))
	if err != nil {
		panic(fmt.Sprintf("Pending Subscription failed error: %s", err.Error()))
	}
	time.Sleep(time.Second * 5)

	for {
		_, nextNotification, e := wsPendingSubscriber.ReadMessage()
		if e != nil {
			ee := fmt.Sprintf("Subscription failed error: %s", e.Error())
			fmt.Println(ee)
		}
		fmt.Printf("[%s] : %s\n", time.Now().String(), nextNotification)
	}
}
