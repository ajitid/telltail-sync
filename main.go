package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/atotto/clipboard"
	"github.com/r3labs/sse/v2"
)

var (
	url, device string
)

// Expiring text received from telltail won't be possible if we
// aren't notified that user has copied something to the clipboard.
// Expiring simply means clearing out the clipboard.
var expirationPossible bool

type payload struct {
	Text   string
	Device string
}

type fileExistsParams struct {
	cmd, relativePath string
}

func fileExists(params fileExistsParams) bool {
	if params.cmd == "" {
		return false
	}

	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("cannot get current working dir")
	}
	_, err = os.Stat(filepath.Join(cwd, params.relativePath, params.cmd))
	return !errors.Is(err, fs.ErrNotExist)
}

func sendToTelltail(skipSend, expire chan bool) {
	select {
	case <-skipSend:
	default:
		text, err := clipboard.ReadAll()
		if err != nil {
			// TODO this fails a lot on x11, check how we can solve it
			// I suspect xclip, but xsel doesn't at all
			log.Fatal("cannot send clipboard's content to telltail because the clipboard isn't accessible for read\n", err)
		}
		expire <- false
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
		http.Post(url+"/set", "application/json", r)
	}
}

func autoSend(skipSend, expire chan bool) {
	switch runtime.GOOS {
	case "linux":
		cmd := "./clipnotify"
		if !fileExists(fileExistsParams{cmd: cmd}) {
			fmt.Println("We need `clipnotify` to detect whether if you've copied something. But we cannot find it.")
			return
		}

		expirationPossible = true

		failCount := 0

		for {
			cmd := exec.Command(cmd, "-s", "clipboard")
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

			sendToTelltail(skipSend, expire)
		}
	case "windows":
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println("We need `clipnotify.exe` to detect whether if you've copied something. But we cannot resolve your current working directory to find it.")
			return
		}
		clipnotifyDir := filepath.Join(cwd, "clipnotify")
		if _, err := os.Stat(filepath.Join(clipnotifyDir, "clipnotify.exe")); os.IsNotExist(err) {
			fmt.Println("We need `clipnotify.exe` to detect whether if you've copied something. But we cannot find the executable.")
			return
		}

		expirationPossible = true

		for {
			cmd := exec.Command("clipnotify.exe")
			cmd.Dir = clipnotifyDir
			_, err := cmd.Output()
			if err != nil {
				// this should never have happened
				// the only way it could fail if:
				// - either deps for clipnotify have not been installed, or
				// - or the python file to run couldn't be located
				log.Fatal("clipboard notifier failed")
			}

			sendToTelltail(skipSend, expire)
		}
	case "darwin":
		cmd := "./clipnotify"
		if !fileExists(fileExistsParams{cmd: cmd}) {
			fmt.Println("We need `clipnotify` to detect whether if you've copied something. But we cannot find it.")
			return
		}

		expirationPossible = true

		for {
			cmd := exec.Command(cmd)
			_, err := cmd.Output()
			if err != nil {
				log.Fatal("clipboard notifier failed")
			}

			sendToTelltail(skipSend, expire)
		}
	default:
		fmt.Println("Your ctrl+c / cmd+c will not be automatically send to telltail as this feature is not supported yet for your OS.")
	}
}

func writeToClipboard(text string, skipSend chan bool) {
	clipText, err := clipboard.ReadAll()
	if err != nil {
		log.Fatal("unable to write text to clipboard because clipboard isn't accessible for read\n", err)
	}
	/*
		Avoid unnecessary writes, because:
		- Other programs would be monitoring clipboard as well and we shouldn't be sending extraneous events
		- If an image is in the clipboard, image/png would have something but text/plain would be empty. If we override it,
		  we would be overriding it with nothing i.e. we would lose information. This would happen because we're only storing
			text/plain MIME in program's memory.
	*/
	if text == clipText {
		return
	}

	skipSend <- true
	err = clipboard.WriteAll(text)
	if err != nil {
		log.Fatal("unable to write to clipboard\n", err)
	}
}

func autoReceive(skipSend, expire, done chan bool) {
	client := sse.NewClient(url + "/events")
	client.EncodingBase64 = true // if not done, only first line of multiline string will be send, see https://github.com/r3labs/sse/issues/62

	client.Subscribe("texts", func(msg *sse.Event) {
		var j payload
		json.Unmarshal(msg.Data, &j)
		if j.Device != device {
			expire <- true
			writeToClipboard(j.Text, skipSend)
		}
	})
	done <- true
}

func expireClipboardContent(skipSend, expire chan bool) {
	t := time.AfterFunc(0, func() {})

	for {
		e := <-expire
		t.Stop()
		if expirationPossible && e {
			t = time.AfterFunc(2*time.Minute, func() {
				writeToClipboard("", skipSend)
			})
		}
	}
}

func main() {
	flag.StringVar(&url, "url", "", "URL of Telltail, usually looks like https://telltail.tailnet-name.ts.net")
	flag.StringVar(&device, "device", "", "Device ID: pass the device name or IP assigned in Tailscale")
	flag.Parse()

	if len(url) == 0 {
		log.Fatal("`--url` parameter not provided")
	}
	if len(device) == 0 {
		log.Fatal("`--device` parameter not provided")
	}

	done := make(chan bool)
	skipSend := make(chan bool, 1)
	expire := make(chan bool)
	go autoSend(skipSend, expire)
	go autoReceive(skipSend, expire, done)
	go expireClipboardContent(skipSend, expire)
	// This `done` should never happen, because it would mean that somehow
	// sse client stopped listening. If that happens, we'd need to figure out
	// a way to resubscribe it.
	<-done
}
