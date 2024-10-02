package main

import (
	nodeipc "go-ipc/nodeipcserver"
	"log"
	"sync"
	"time"
)

func main() {
	log.Println("starting")

	var wg sync.WaitGroup
	wg.Add(1)
	go nodeipc.Shared().Run()
	go startServerSchedule()
	wg.Wait()
}

func startServerSchedule() {
	ticker5Sec := time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ticker5Sec.C:
				go nodeipc.Shared().BroadcastLog([]byte("Hello Client"))
			}
		}
	}()
}
