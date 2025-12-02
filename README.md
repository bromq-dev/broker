# MQTT Broker

A batteries-not-included, high-performance MQTT broker designed for easy integration into other systems. Built with Go, this broker provides a clean separation between protocol handling and transport layers, with a powerful hook system for authentication, authorization, and message interception.

## Features

- **Protocol Support**: MQTT 3.1, 3.1.1, and 5
- **Transport Agnostic**: TCP, TLS, WebSocket support out of the box
- **Hook System**: Easy integration with authentication, authorization, and message processing
- **High Performance**: Designed for low latency and high throughput
- **Clean Architecture**: Separation of concerns between protocol, transport, and business logic
- **Minimal Dependencies**: Only essential dependencies included

## Benchmarks

Stress test comparing against popular MQTT brokers. All tests run with identical conditions:

**Test Configuration:**
- 2,000 publishers, 100 subscribers, 100 topics
- QoS 2 (exactly-once delivery)
- 256 byte payload
- 60 second duration
- Docker containers limited to 4 CPUs, 8GB RAM

| Broker | Memory | CPU | Delivery | Throughput | P50 Latency | P99 Latency |
|--------|--------|-----|----------|------------|-------------|-------------|
| **This broker** | 519 MB | 389% | 99.4% | 158K msg/s | 131ms | 757ms |
| Mochi MQTT | 956 MB | 410% | 103.3% | 103K msg/s | 369ms | 1.9s |
| Mosquitto | 530 MB | 92% | 101.2% | 70K msg/s | 1.6s | 2.5s |

*EMQX omitted - default OOM protection kills connections under this load; disabling it causes 7GB+ memory usage with 12% delivery.*

**Reproduce:**
```bash
go install github.com/bromq-dev/testmqtt@latest
testmqtt performance stress --publishers 2000 --subscribers 100 --topics 100 --rate 100 -d 60s --qos 2
```

**Key Takeaways:**
- **1.8x less memory** than Mochi MQTT (nearest Go competitor)
- **1.5x higher throughput** than Mochi, **2.3x higher than Mosquitto**
- **2.8x lower P50 latency** than Mochi, **12x lower than Mosquitto**
- Stable memory usage under sustained load (no leaks or OOM)
