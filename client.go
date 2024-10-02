package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	nodeipc "go-ipc/nodeipcclient"
	"go-ipc/nodeipcclient/message"
	"math/big"
	"sync"
	"time"
)

func main() {
	fmt.Println("Client starting")

	var wg sync.WaitGroup
	wg.Add(1)

	ipcMessageChan := make(chan message.IPCMessage)
	err := nodeipc.Shared().Run(ipcMessageChan)
	if err != nil {
		e := "Failed to subscribe. error: " + err.Error()
		fmt.Println(e)
		return
	}

	go SubscribeLogsWithIPC(ipcMessageChan)

	//time.Sleep(3 * time.Second)
	//go SubmitTestTx()

	wg.Wait()
}

func SubscribeLogsWithIPC(ipcMessageChan chan message.IPCMessage) {
	for {
		select {
		case ipcMessage := <-ipcMessageChan:
			switch ipcMessage.MessageType {
			case message.MsgTypeBlockStart:
				ipcBlock := message.IPCBlock{}
				_ = json.Unmarshal(ipcMessage.MessageData, &ipcBlock)
				fmt.Printf("New Block %d : %d\n", ipcBlock.BlockNumber, ipcBlock.BlockTimestamp)
			case message.MsgTypeLog:
				txLog := types.Log{}
				_ = txLog.UnmarshalJSON(ipcMessage.MessageData)
				fmt.Printf("[%s] TxLog %d %s: %s \n", time.Now().String(), txLog.BlockNumber, txLog.Address, txLog.TxHash)
			case message.MsgTypeBlockEnd:
			case message.MsgTypePendingTx:
				pendingTx := message.PendingTx{}
				_ = json.Unmarshal(ipcMessage.MessageData, &pendingTx)
				fmt.Printf("[%s] Pending Tx txHash: %s, Logs : %d\n", time.Now().String(), pendingTx.TxHash.String(), len(pendingTx.Logs))
				for _, txLog := range pendingTx.Logs {
					fmt.Printf("-- address: %s, data: %x\n", txLog.Address.String(), txLog.Data)
				}
			}
		}
	}
}

func SubmitTestTx() {
	rpc := "http://15.204.182.214:8545/"
	// Sign
	value := big.NewInt(0)
	toAddress := common.HexToAddress("0xa709BaB70a3af51A7D213743323CB6C2f435f7a2")
	signerAddress := "0x8c1f876a1f0961fBE09c5eF95f5433554054fCAe"
	client, _ := ethclient.Dial(rpc)
	nonce, _ := client.NonceAt(context.Background(), common.HexToAddress(signerAddress), nil)
	chainId := big.NewInt(137)

	//gasPrice := big.NewInt(120_000_000_000)
	maxFee := big.NewInt(200_000_000_000)
	maxTip := big.NewInt(30_000_000_000)
	GasLimit := uint64(1500_000)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainId,
		Nonce:     nonce,
		Gas:       GasLimit,
		GasFeeCap: maxFee,
		GasTipCap: maxTip,
		To:        &toAddress,
		Value:     value,
		Data:      []byte{},
	})

	//tx := gotypes.NewTx(&gotypes.LegacyTx{
	//	Nonce:    nonce,
	//	Gas:      GasLimit,
	//	GasPrice: gasPrice,
	//	To:       &toAddress,
	//	Value:    value,
	//	Data:     _data,
	//})

	signer := types.LatestSignerForChainID(chainId)

	var pk = "ca942c95515c4f9c70d4209668f1785394ce3525063170abff3c567009603c03"
	privateKey, _ := crypto.HexToECDSA(pk)
	signedTx, _ := types.SignTx(tx, signer, privateKey)

	//var data bytes.Buffer
	//err := signedTx.EncodeRLP(&data)
	//if err != nil {
	//	return
	//}

	data, _ := signedTx.MarshalBinary()

	fmt.Printf("[%s]Submited Test tx - 0x%x", time.Now().String(), data)
	nodeipc.Shared().SubmitTxn(data)
}
