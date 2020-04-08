#/bin/sh
env GOOS=linux GOARCH=amd64 go build -o bin/stressy-linux-amd64 stressy.go
env GOOS=darwin GOARCH=amd64 go build -o bin/stressy-darwin-amd64 stressy.go
env GOOS=freebsd GOARCH=amd64 go build -o bin/stressy-freebsd-amd64 stressy.go
env GOOS=netbsd GOARCH=amd64 go build -o bin/stressy-netbsd-amd64 stressy.go
env GOOS=openbsd GOARCH=amd64 go build -o bin/stressy-openbsd-amd64 stressy.go
