# 6 Using WebSocket as a network transport

If MQTT is transported over a WebSocket [RFC6455] connection, the following conditions apply:

- MQTT Control Packets MUST be sent in WebSocket binary data frames. If any other type of data frame is received the recipient MUST close the Network Connection \[MQTT-6.0.0-1\].

- A single WebSocket data frame can contain multiple or partial MQTT Control Packets. The receiver MUST NOT assume that MQTT Control Packets are aligned on WebSocket frame boundaries \[MQTT-6.0.0-2\].

- The Client MUST include `mqtt` in the list of WebSocket Sub Protocols it offers \[MQTT-6.0.0-3\].

- The WebSocket Subprotocol name selected and returned by the Server MUST be `mqtt` \[MQTT-6.0.0-4\].

- The WebSocket URI used to connect the Client and Server has no impact on the MQTT protocol.
