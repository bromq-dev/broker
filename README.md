# MQTT Broker

A batteries-not-included, high-performance MQTT broker designed for easy integration into other systems. Built with Go, this broker provides a clean separation between protocol handling and transport layers, with a powerful hook system for authentication, authorization, and message interception.

## Features

- **Protocol Support**: MQTT 3.1, 3.1.1, and 5
- **Transport Agnostic**: TCP, TLS, WebSocket support out of the box
- **Hook System**: Easy integration with authentication, authorization, and message processing
- **High Performance**: Designed for low latency and high throughput
- **Clean Architecture**: Separation of concerns between protocol, transport, and business logic
- **Minimal Dependencies**: Only essential dependencies included
