# Event Bus Configuration Guide

## Overview

The event bus can be configured to use either in-memory (single process) or NATS (distributed) communication. This guide shows you what parameters to add to your configuration files.

## Configuration Parameters

### 1. Event Bus Type Configuration

Add this section to your `config.yaml`:

```yaml
extensions:
  trazr:
    # Event Bus Configuration
    event_bus:
      # Type: "in-memory" or "nats"
      type: "nats"
      
      # NATS Configuration (only needed if type is "nats")
      nats:
        # NATS server URL
        url: "nats://localhost:4222"
        
        # Connection options
        connection:
          # Connection timeout
          timeout: "5s"
          
          # Reconnect settings
          reconnect:
            enabled: true
            max_attempts: 10
            delay: "1s"
          
          # Buffer size for messages
          buffer_size: 65536
        
        # JetStream configuration (optional, for persistence)
        jetstream:
          enabled: false
          # domain: "default"
          # stream_name: "telemetry_events"
        
        # Health check settings
        health_check:
          enabled: true
          interval: "30s"
          timeout: "5s"
      
      # In-memory configuration (only needed if type is "in-memory")
      in_memory:
        # Buffer size for event queue
        buffer_size: 1000
        
        # Number of worker goroutines
        max_workers: 10
        
        # Event processing timeout
        timeout: "5s"
      
      # Common event bus settings
      common:
        # Enable event bus metrics
        metrics_enabled: true
        
        # Log level for event bus operations
        log_level: "info"
        
        # Event types to handle
        event_types:
          - "log.inserted"
          - "trace.inserted"
          - "metric.inserted"
          - "profile.inserted"
```

### 2. Environment Variables

You can also configure via environment variables:

```bash
# Event Bus Type
export EVENT_BUS_TYPE="nats"  # or "in-memory"

# NATS Configuration
export NATS_URL="nats://localhost:4222"
export NATS_TIMEOUT="5s"
export NATS_RECONNECT_ENABLED="true"
export NATS_RECONNECT_MAX_ATTEMPTS="10"
export NATS_RECONNECT_DELAY="1s"
export NATS_BUFFER_SIZE="65536"

# JetStream Configuration
export NATS_JETSTREAM_ENABLED="false"
export NATS_JETSTREAM_DOMAIN="default"
export NATS_JETSTREAM_STREAM_NAME="telemetry_events"

# Health Check Configuration
export NATS_HEALTH_CHECK_ENABLED="true"
export NATS_HEALTH_CHECK_INTERVAL="30s"
export NATS_HEALTH_CHECK_TIMEOUT="5s"

# In-memory Configuration
export EVENT_BUS_BUFFER_SIZE="1000"
export EVENT_BUS_MAX_WORKERS="10"
export EVENT_BUS_TIMEOUT="5s"

# Common Configuration
export EVENT_BUS_METRICS_ENABLED="true"
export EVENT_BUS_LOG_LEVEL="info"
```

### 3. Complete Configuration Example

Here's a complete `config.yaml` with event bus configuration:

```yaml
extensions:
  trazr:
    # Event Bus Configuration
    event_bus:
      type: "nats"  # or "in-memory"
      
      nats:
        url: "nats://localhost:4222"
        connection:
          timeout: "5s"
          reconnect:
            enabled: true
            max_attempts: 10
            delay: "1s"
          buffer_size: 65536
        jetstream:
          enabled: false
        health_check:
          enabled: true
          interval: "30s"
          timeout: "5s"
      
      in_memory:
        buffer_size: 1000
        max_workers: 10
        timeout: "5s"
      
      common:
        metrics_enabled: true
        log_level: "info"
        event_types:
          - "log.inserted"
          - "trace.inserted"
          - "metric.inserted"
          - "profile.inserted"

receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  batch:
    send_batch_size: 1024
    timeout: 5s
    send_batch_max_size: 2048
  trazr:

exporters:
  trazr:
    sending_queue:
      enabled: true
      queue_size: 10000
      num_consumers: 1
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
      max_elapsed_time: 300s
  file:
    path: ./logs/trazr2.log
  debug:
    verbosity: detailed

service:
  extensions: [trazr]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [trazr, debug]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [trazr, debug]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [trazr, debug]
```

## Configuration Options Explained

### Event Bus Types

1. **`in-memory`**: 
   - Single process communication
   - Fastest performance
   - No external dependencies
   - Use for single-service deployments

2. **`nats`**: 
   - Distributed communication
   - Multi-service support
   - High throughput (10M+ messages/second)
   - Use for microservices architecture

### NATS Configuration

- **`url`**: NATS server connection URL
- **`timeout`**: Connection timeout
- **`reconnect`**: Automatic reconnection settings
- **`buffer_size`**: Message buffer size
- **`jetstream`**: Persistence configuration (optional)
- **`health_check`**: Health monitoring settings

### In-Memory Configuration

- **`buffer_size`**: Event queue buffer size
- **`max_workers`**: Number of worker goroutines
- **`timeout`**: Event processing timeout

### Common Configuration

- **`metrics_enabled`**: Enable Prometheus metrics
- **`log_level`**: Logging verbosity
- **`event_types`**: List of event types to handle

## Usage Examples

### Development (In-Memory)
```yaml
event_bus:
  type: "in-memory"
  in_memory:
    buffer_size: 100
    max_workers: 5
```

### Production (NATS)
```yaml
event_bus:
  type: "nats"
  nats:
    url: "nats://nats-server:4222"
    connection:
      timeout: "10s"
      reconnect:
        enabled: true
        max_attempts: 20
        delay: "2s"
    health_check:
      enabled: true
      interval: "60s"
```

### High-Performance (NATS with JetStream)
```yaml
event_bus:
  type: "nats"
  nats:
    url: "nats://nats-server:4222"
    jetstream:
      enabled: true
      stream_name: "telemetry_events"
    connection:
      buffer_size: 131072
```

## Validation

The configuration is validated at startup. Common validation errors:

- Invalid NATS URL format
- Connection timeout to NATS server
- Invalid buffer sizes
- Unsupported event types

## Monitoring

When `metrics_enabled: true`, the following metrics are available:

- `event_bus_events_published_total`
- `event_bus_events_delivered_total`
- `event_bus_events_failed_total`
- `event_bus_processing_latency_seconds`
- `event_bus_queue_size`
- `nats_connection_status`
- `nats_messages_sent_total`
- `nats_messages_received_total` 