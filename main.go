package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/atotto/clipboard"
	"github.com/r3labs/sse/v2"
)

const Url = "https://sd.alai-owl.ts.net:1111"
const device = "100.109.205.35"

var textToRestore string

type payload struct {
	Text   string
	Device string
}

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func autoSend(skipSend, restore chan bool) {
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
			textToRestore = text
			if len(text) == 0 || len(text) > 65536 {
				break
			}
			p := &payload{
				Text:   text,
				Device: device,
			}
			b, err := json.Marshal(p)
			if err != nil {
				log.Fatal("couldn't send the payload to /set")
			}
			r := bytes.NewReader(b)
			http.Post(Url+"/set", "application/json", r)
		}
	}
}

func writeToClipboard(text string, skipSend chan bool) {
	clipText, err := clipboard.ReadAll()
	if err != nil {
		log.Fatal("clipboard isn't accessible", err)
	}
	if text == clipText {
		return
	}
	// ^ We are avoiding unnecessary writes because other programs would be monitoring clipboard for changes as well.

	skipSend <- true
	clipboard.WriteAll(text)
}

func autoReceive(skipSend, restore, done chan bool) {
	client := sse.NewClient(Url + "/events")
	client.EncodingBase64 = true // if not done, only first line of multiline string will be send, see https://github.com/r3labs/sse/issues/62

	client.Subscribe("texts", func(msg *sse.Event) {
		var j payload
		json.Unmarshal(msg.Data, &j)
		if j.Device != device {
			restore <- true
			writeToClipboard(j.Text, skipSend)
		}
	})
	done <- true
}

func restoreOriginal(skipSend, restore chan bool) {
	t := time.AfterFunc(0, func() {})

	for {
		<-restore
		t.Stop()
		t = time.AfterFunc(2*time.Minute, func() {
			writeToClipboard(textToRestore, skipSend)
		})
	}
}

func main() {
	done := make(chan bool)
	skipSend := make(chan bool, 1)
	restore := make(chan bool)
	go autoSend(skipSend, restore)
	go autoReceive(skipSend, restore, done)
	go restoreOriginal(skipSend, restore)
	<-done
}
