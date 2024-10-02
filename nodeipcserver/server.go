package nodeipc

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	ipc "github.com/james-barrow/golang-ipc"
	"go-ipc/nodeipcclient/utils"
	"go-ipc/nodeipcserver/constant"
	"go-ipc/nodeipcserver/message"
	"sync"
	"time"
)

type BotClient struct {
	Submit    *message.Server
	Subscribe *message.Client
}

type Server struct {
	botClients map[string]*BotClient
	mutexBot   *sync.RWMutex
	serverMain *message.Server
}

var instance *Server

func Shared() *Server {
	if instance != nil {
		return instance
	}
	instance = &Server{
		botClients: make(map[string]*BotClient),
		mutexBot:   new(sync.RWMutex),
	}
	return instance
}

func (s *Server) Run() {
	log.Info("IpcServer Start")
	fmt.Println(utils.GetCurrentTimeStr(), "IpcServer Start")
	botMainMonitor := func(msg *ipc.Message) {
		switch message.MsgType(msg.MsgType) {
		case message.MsgTypeNewClient:
			go s.addBotClient(string(msg.Data))
		}
	}
	s.serverMain = message.NewServer(constant.BotMainIPC, botMainMonitor)
	go s.serverMain.Run()
	go s.startServerSchedule()
}

func (s *Server) addBotClient(name string) {
	log.Info("IpcServer received new bot client", "client", name)
	fmt.Println(utils.GetCurrentTimeStr(), "IpcServer received new bot client", "client", name)
	subscribeSubmit := func(msg *ipc.Message) {
		switch message.MsgType(msg.MsgType) {
		case message.MsgTypeMessage:
			go submitTransaction(name, msg.Data)
		}
	}
	s.mutexBot.Lock()
	subscribeClient := message.NewClient(name, nil)
	submitName := name + "_submit"
	submitServer := message.NewServer(submitName, subscribeSubmit)
	s.botClients[name] = &BotClient{
		Submit:    submitServer,
		Subscribe: subscribeClient,
	}
	s.mutexBot.Unlock()

	go subscribeClient.Run()
	go submitServer.Run()
	time.Sleep(time.Second * 2)

	log.Info("IpcServer sending create submit to client", "client", name, "submit", submitName)
	fmt.Println(utils.GetCurrentTimeStr(), "IpcServer sending create submit to client", "client", name, "submit", submitName)
	err := subscribeClient.SendCreateSubmit(submitName)
	if err != nil {
		log.Info("IpcServer Failed to send submit create request", "client", name)
		return
	}
}

func submitTransaction(client string, txnData []byte) {
	log.Info("IpcServer received submit Request", "client", client, "len", len(txnData))
	fmt.Println(utils.GetCurrentTimeStr(), "IpcServer received submit Request", "client", client, "len", len(txnData))
	// todo submit
}

func (s *Server) BroadcastLog(data []byte) {
	defer func() {
		log.Info("IpcServer BroadcastLog end", "len", len(data))
		s.mutexBot.RUnlock()
	}()
	s.mutexBot.RLock()
	log.Info("IpcServer BroadcastLog start", "clients", len(s.botClients), "len", len(data))
	fmt.Println(utils.GetCurrentTimeStr(), "IpcServer BroadcastLog start", "clients", len(s.botClients), "len", len(data))
	for _, botClient := range s.botClients {
		go botClient.Subscribe.SendData(data)
	}
}

func (s *Server) checkClientStatus() {
	nowTs := time.Now().UnixMilli()
	var invalidClients []string
	s.mutexBot.RLock()
	for name, botClient := range s.botClients {
		if nowTs > botClient.Subscribe.LastPingTs+10_000 {
			invalidClients = append(invalidClients, name)
		}
	}
	s.mutexBot.RUnlock()

	if len(invalidClients) > 0 {
		s.mutexBot.Lock()
		for _, name := range invalidClients {
			log.Info("IpcServer remove invalid client", "client", name)
			fmt.Println(utils.GetCurrentTimeStr(), "IpcServer remove invalid client", "client", name)
			s.botClients[name].Submit.Close()
			s.botClients[name].Subscribe.Close()
			delete(s.botClients, name)
		}
		s.mutexBot.Unlock()
	}
}

func (s *Server) startServerSchedule() {
	ticker5Sec := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker5Sec.C:
				go s.checkClientStatus()
			}
		}
	}()
}
