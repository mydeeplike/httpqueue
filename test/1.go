package main

import (
	"jwseclab.com/mydeeplike/httpqueue"
)

func main() {
	queue := httpqueue.New("./1.db", 5)
	err := queue.GET("http://www.csdn.net/", "DOCTYPE")
	if err != nil {
		panic(err)
	}
	select {}
}
