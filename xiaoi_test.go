package xiaoi

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestXiaoi(t *testing.T) {
	parameters := XiaoiParameters{
		Param_key:         "<Your key>",
		Param_secret:      "<Your secret>",
		Param_connections: 5,
		Param_queue_size:  10000,
		Param_timeout:     time.Second,
	}
	var wait sync.WaitGroup
	xiaoi := NewXiaoi(&parameters, func(msg *XiaoiMsg) {
		fmt.Printf("%+v\n", msg)
		wait.Done()
	})

	xiaoi.Post("Test1", "我爱你")
	wait.Add(1)

	wait.Wait()
}
