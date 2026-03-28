APP     := dbf-sync
VERSION := 1.0.3
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.buildDate=$(DATE) -X main.commit=$(COMMIT)

# Install destinations
PREFIX      ?= /usr/local
INSTALL_BIN  = $(PREFIX)/bin

.PHONY: build install uninstall

build:
	go build -ldflags "$(LDFLAGS)" -o $(APP) .

install: build
	@echo "Instalando $(APP) en $(INSTALL_BIN)..."
	@mkdir -p $(INSTALL_BIN)
	@cp $(APP) $(INSTALL_BIN)/$(APP)
	@chmod +x $(INSTALL_BIN)/$(APP)
	@echo "Listo. Ejecuta: $(APP) interactive"

uninstall:
	@rm -f $(INSTALL_BIN)/$(APP)
	@echo "$(APP) desinstalado."
