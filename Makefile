.PHONY: help all build clean run stop test conformance-v3 conformance-v5 conformance

# Variables
MAIN_PATH=cmd/broker
BIN_DIR=bin

.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the project
	@go build -o $(BIN_DIR)/broker ./$(MAIN_PATH)

run: build ## Run the application
	@$(BIN_DIR)/broker

stop: ## Stop the application
	@pkill -f $(BIN_DIR)/broker || echo "No running instance found."

install-testmqtt:
	@go install github.com/bromq-dev/testmqtt@latest

conformance-v3: install-testmqtt ## Run MQTT 3.1.1 Conformance Tests
	@echo "Running MQTT 3.1.1 Conformance Tests..."
	@testmqtt conformance --version 3 --verbose

conformance-v5: install-testmqtt ## Run MQTT 5.0 Conformance Tests
	@echo "Running MQTT 5.0 Conformance Tests..."
	@testmqtt conformance --version 5 --verbose

conformance: conformance-v3 conformance-v5 ## Run all Conformance Tests
