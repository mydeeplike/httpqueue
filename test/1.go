package main

import (
	"github.com/mydeeplike/httpqueue"
)

func main() {
	queue := httpqueue.New("./1.db")
	queue.SetRetrys(5)
	queue.SetRetryDelay(5)
	err := queue.GET("http://www.csdn.net/", "DOCTYPE")
	if err != nil {
		panic(err)
	}
	select {}
}
