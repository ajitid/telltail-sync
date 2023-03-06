package main

import (
	"github.com/r3labs/sse/v2"
	"golang.design/x/clipboard"
)

func main() {
	err := clipboard.Init()
	if err != nil {
		panic(err)
	}

	client := sse.NewClient("https://sd.alai-owl.ts.net:1111/events")

	client.Subscribe("text", func(msg *sse.Event) {
		clipboard.Write(clipboard.FmtText, msg.Data)
	})
}
