.PHONY: build install clean

build:
	GOTOOLCHAIN=auto go build -o ngrokd ./cmd/ngrokd
	GOTOOLCHAIN=auto go build -o ngrokctl ./cmd/ngrokctl

install: build
	sudo ./ngrokd install

clean:
	rm -f ngrokd ngrokctl
