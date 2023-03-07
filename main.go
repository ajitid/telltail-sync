package main

import (
	"github.com/atotto/clipboard"
	"github.com/r3labs/sse/v2"
)

func main() {
	client := sse.NewClient("https://sd.alai-owl.ts.net:1111/events")
	client.EncodingBase64 = true // if not done, only first line of multiline string will be send, see https://github.com/r3labs/sse/issues/62
	client.Subscribe("text", func(msg *sse.Event) {
		clipboard.WriteAll(string(msg.Data))
	})
}
