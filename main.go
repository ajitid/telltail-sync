package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/r3labs/sse/v2"
)

const Url = "https://sd.alai-owl.ts.net:1111"

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func autoSend(skipSend chan bool) {
	if runtime.GOOS != "linux" {
		fmt.Println("Your ctrl+c / cmd+c will not be automatically send to telltail as this feature is not supported yet for your OS.")
		return
	}

	if !commandExists("clipnotify") {
		fmt.Println("We need `clipnotify` to detect whether if you've copied something. `clipnotify` is only available for X11 systems.")
		fmt.Println("If you are on X11 put `clipnotify` in any of these paths and rerun this program:", os.Getenv("PATH"))
		fmt.Println("Preferably put it in `/usr/local/bin/`.")
		return
	}

	failCount := 0

	for {
		cmd := exec.Command("clipnotify", "-s", "clipboard")
		_, err := cmd.Output()
		if err != nil {
			// It will continue to fail until the GUI is loaded up after boot/login.
			// We'll silently wait for it to succeed.
			// Probably the proper way to solve this is to make changes in the systemd
			// service file such that this program gets invoked even later.
			failCount++
			if failCount >= 22 {
				log.Fatal("waited too long for `clipnotify` to succeed.")
			}
			time.Sleep(2 * time.Second)
			continue
		}

		select {
		case <-skipSend:
		default:
			text, err := clipboard.ReadAll()
			if err != nil {
				log.Fatal("clipboard isn't accessible", err)
			}
			if len(text) == 0 {
				continue
			}
			reader := strings.NewReader(text)
			http.Post(Url+"/set", "text/plain; charset=UTF-8", reader)
		}
	}
}

func autoReceive(skipSend, done chan bool) {
	client := sse.NewClient(Url + "/events")
	client.EncodingBase64 = true // if not done, only first line of multiline string will be send, see https://github.com/r3labs/sse/issues/62

	client.Subscribe("text", func(msg *sse.Event) {
		clipText, err := clipboard.ReadAll()
		if err != nil {
			log.Fatal("clipboard isn't accessible", err)
		}

		telltailText := string(msg.Data)
		if clipText == telltailText {
			return
		}
		skipSend <- true
		clipboard.WriteAll(string(msg.Data))
	})
	done <- true
}

func main() {
	done := make(chan bool)
	skipSend := make(chan bool, 1)
	go autoSend(skipSend)
	go autoReceive(skipSend, done)
	<-done
}
