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
| **This broker** | 338 MB | 391% | 100.3% | 166K msg/s | 83ms | 1.1s |
| Mochi MQTT | 854 MB | 409% | 102.9% | 113K msg/s | 287ms | 1.5s |
| Mosquitto | 557 MB | 90% | 100.7% | 74K msg/s | 1.4s | 2.4s |
| EMQX | 7.4 GB | 399% | 12.3% | 18K msg/s | 5.6s | 42s |

*EMQX tested with OOM protection disabled to allow fair comparison; default settings caused connection kills under load.*

**Reproduce:**
```bash
go install github.com/bromq-dev/testmqtt@latest
testmqtt performance stress --publishers 2000 --subscribers 100 --topics 100 --rate 100 -d 60s --qos 2
```

**Key Takeaways:**
- **2.5x less memory** than Mochi MQTT (nearest Go competitor)
- **1.5x higher throughput** than Mochi, **2.2x higher than Mosquitto**
- **3-68x lower latency** than alternatives
- Stable memory usage under sustained load (no leaks or OOM)
