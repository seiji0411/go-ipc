package nodeipcclient

import (
	"encoding/json"
	"fmt"
	"go-ipc/nodeipcclient/constant"
	"go-ipc/nodeipcclient/message"
	"go-ipc/nodeipcclient/utils"
	"time"

	ipc "github.com/james-barrow/golang-ipc"
)

type BotClient struct {
	server    *message.Client
	Subscribe *message.Server
	Submit    *message.Client
}

var instance *BotClient

func Shared() *BotClient {
	if instance != nil {
		return instance
	}
	instance = &BotClient{}
	return instance
}

func (b *BotClient) Run(channel chan<- message.IPCMessage) error {
	b.server = message.NewClient(constant.BotMainIPC, nil)
	go b.server.Run()
	time.Sleep(time.Second * 2)

	subscribeLog := func(msg *ipc.Message) {
		switch message.MsgType(msg.MsgType) {
		case message.MsgTypeMessage:
			ipcMessage := message.IPCMessage{}
			err := json.Unmarshal(msg.Data, &ipcMessage)
			if err != nil {
				fmt.Println("JSON parse Error: ", err.Error())
				return
			}
			channel <- ipcMessage
		case message.MsgTypeCreateSubmit:
			go b.createSubmitClient(string(msg.Data))
		}
	}
	b.Subscribe = message.NewServer("", subscribeLog)
	go b.Subscribe.Run()

	time.Sleep(time.Second * 2)

	for i := 0; i < 3; i++ {
		fmt.Println(utils.GetCurrentTimeStr() + " Client sending add new bot: " + b.Subscribe.GetPipeName())
		err := b.server.SendAddNewRequest(b.Subscribe.GetPipeName())
		fmt.Println(utils.GetCurrentTimeStr() + " Client sent add new bot")

		if err != nil {
			fmt.Println("Client: Failed to send the request for creating " + err.Error())
		} else {
			break
		}
		if i == 2 {
			break
		}
	}
	b.startClientSchedule()
	time.Sleep(2 * time.Second)
	b.server.Close()

	return nil
}

func (b *BotClient) createSubmitClient(pipeName string) {
	b.Submit = message.NewClient(pipeName, nil)
	go b.Submit.Run()
}

func (b *BotClient) SubmitTxn(data []byte) {
	if b.Submit == nil {
		return
	}
	err := b.Submit.SendData(data)
	if err != nil {
		fmt.Println("Failed to submit txn")
	}
	fmt.Println(utils.GetCurrentTimeStr() + " Client sent submit")
}

func (b *BotClient) startClientSchedule() {
	ticker3Sec := time.NewTicker(3 * time.Second)
	go func() {
		for {
			select {
			case <-ticker3Sec.C:
				go b.Subscribe.SendPing()
			}
		}
	}()
}
