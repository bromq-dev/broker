# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A high-performance MQTT broker written in Go, supporting MQTT 3.1, 3.1.1, and 5.0 protocols. Designed for easy integration with transport-agnostic architecture (TCP, TLS, WebSocket) and a hook system for authentication, authorization, and message interception.

## Build Commands

```bash
make build          # Build the broker to bin/broker
make run            # Build and run the broker
make stop           # Stop running broker instance
make test           # Run tests (standard go test)
```

## Testing

```bash
go test ./...                           # Run all tests
go test ./pkg/...                       # Run package tests
go test -run TestName ./path/to/pkg     # Run a single test

# MQTT Conformance Tests (requires testmqtt tool)
make conformance-v3   # Run MQTT 3.1.1 conformance tests
make conformance-v5   # Run MQTT 5.0 conformance tests
make conformance      # Run all conformance tests
```

## Architecture

- **cmd/broker/**: Main entry point for the broker executable
- **pkg/**: Library packages (protocol handling, transport, hooks, etc.)

The broker separates protocol handling from transport layers, allowing different transport implementations while sharing the MQTT protocol logic.

## MQTT Specifications

The `spec/` directory contains the MQTT specifications broken into searchable, linked markdown files:

- **spec/v3.1.1/**: MQTT 3.1.1 specification
- **spec/v5.0/**: MQTT 5.0 specification

Files are organized by section (e.g., `3.1_connect.md`, `4.3_qos.md`) and contain cross-references to related sections. Use these for protocol reference when implementing or debugging MQTT features.

## Design Principles

When implementing or modifying broker code, follow these principles:

1. **Zero-copy whenever possible** - Avoid unnecessary allocations and copies. Use byte slices directly, pool buffers, and pass references rather than copying data.

2. **Non-blocking / minimal locks** - Prefer channels over mutexes for synchronization. When locks are necessary, keep critical sections small. Use lock-free data structures where appropriate.

3. **Idiomatic Go** - Follow Go conventions: accept interfaces/return structs, use context for cancellation, handle errors explicitly, prefer composition over inheritance.

4. **Extensible by design** - Use interfaces and hooks to allow users to customize behavior. The broker should be a library that developers build upon.

5. **Batteries-not-included** - Keep the core minimal. Don't bundle ACL, auth, clustering, bridging, persistence, etc. Provide clean extension points (hooks/interfaces) so developers can implement these themselves.
