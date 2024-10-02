package utils

import (
	"fmt"
	"time"
)

func GetCurrentTimeStr() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func OutLog(str string) {
	fmt.Println(str)
}
