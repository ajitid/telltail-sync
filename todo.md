cross compile using

GOOS=darwin GOARCH=amd64 go build -o telltail-sync-x64-mac
GOOS=windows GOARCH=amd64 go build -o telltail-sync-win.exe

change fmt to log, (and probably remove time)... but if i'm making log act like fmt, do i really need to use log?
