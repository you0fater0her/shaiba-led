.PHONY: build build-pi run tidy

tidy:
	go mod tidy

# Сборка для ПК/тестов (лента симулируется)
build:
	go build -ldflags "-s -w" -o bin/ledstrip .

# Сборка для Raspberry Pi с реальной лентой (нужен cgo + libws2811)
# Зависимости: sudo apt install golang gcc
build-pi:
	CGO_ENABLED=1 go build -tags ws281x -ldflags "-s -w" -o bin/ledstrip-pi .

run:
	go run .