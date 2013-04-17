all: gls
	go build gls.go

install: gls
	cp gls /usr/local/bin/gls
