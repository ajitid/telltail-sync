package main

import (
	"github.com/atotto/clipboard"
	"github.com/r3labs/sse/v2"
)

func main() {
	client := sse.NewClient("https://sd.alai-owl.ts.net:1111/events")
	client.Subscribe("text", func(msg *sse.Event) {
		clipboard.WriteAll(string(msg.Data))
	})
}
