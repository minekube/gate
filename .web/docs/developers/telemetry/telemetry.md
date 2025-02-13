# Gate Telemetry

Gate supports OpenTelemetry for metrics and distributed tracing, allowing operators to monitor their deployment's health, performance, and behavior.

## Configuration

Enable telemetry in your `config.yml`:

```yaml
telemetry:
  # Metrics configuration using OpenTelemetry
  metrics:
    enabled: true
    endpoint: "0.0.0.0:8888"  # Endpoint for /metrics
    anonymousMetrics: true    # Send anonymous usage metrics
    exporter: prometheus      # Supported: prometheus, otlp
    prometheus:
      path: "/metrics"        # Path for Prometheus scraping
  
  # Distributed tracing configuration
  tracing:
    enabled: false           # Disabled by default
    endpoint: "localhost:4317"  # OTLP collector endpoint
    sampler: "parentbased_always_on"
    exporter: otlp           # Supported: otlp, jaeger, stdout
```

## Setup Options

### Self-Hosted Stack

1. **Prometheus + Grafana + Loki + Tempo**

```yaml
version: '3'
services:
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_AUTH_ANONYMOUS_ENABLED=true
      - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin

  loki:
    image: grafana/loki
    ports:
      - "3100:3100"

  tempo:
    image: grafana/tempo
    ports:
      - "14250:14250"
```

prometheus.yml:
```yaml
scrape_configs:
  - job_name: 'gate'
    static_configs:
      - targets: ['localhost:8888']
```

2. **Jaeger All-in-One**

```bash
docker run -d --name jaeger \
  -e COLLECTOR_OTLP_ENABLED=true \
  -p 16686:16686 \
  -p 4317:4317 \
  jaegertracing/all-in-one:latest
```

### Managed Services

1. **Grafana Cloud**
   - Create account at https://grafana.com/
   - Get endpoints and credentials
   - Configure Gate:
   ```yaml
   telemetry:
     metrics:
       enabled: true
       endpoint: "prometheus-us-central1.grafana.net:9090"
       exporter: otlp
     tracing:
       enabled: true
       endpoint: "tempo-us-central1.grafana.net:4317"
       exporter: otlp
   ```

2. **Honeycomb**
   - Sign up at https://www.honeycomb.io/
   - Get API key
   - Configure Gate:
   ```yaml
   telemetry:
     tracing:
       enabled: true
       endpoint: "api.honeycomb.io:443"
       exporter: otlp
   ```

3. **New Relic**
   - Create account at https://newrelic.com/
   - Get license key
   - Configure Gate:
   ```yaml
   telemetry:
     metrics:
       enabled: true
       endpoint: "otlp.nr-data.net:4317"
       exporter: otlp
     tracing:
       enabled: true
       endpoint: "otlp.nr-data.net:4317"
       exporter: otlp
   ```

## Sample Grafana Dashboard

```json
{
  "annotations": {
    "list": []
  },
  "editable": true,
  "panels": [
    {
      "title": "Connected Players",
      "type": "graph",
      "datasource": "Prometheus",
      "targets": [
        {
          "expr": "gate_players_current",
          "legendFormat": "Players"
        }
      ]
    },
    {
      "title": "Server Performance",
      "type": "gauge",
      "datasource": "Prometheus",
      "targets": [
        {
          "expr": "gate_performance_tps",
          "legendFormat": "TPS"
        }
      ],
      "options": {
        "maxValue": 20,
        "minValue": 0,
        "thresholds": [
          { "value": 15, "color": "red" },
          { "value": 18, "color": "yellow" },
          { "value": 19.5, "color": "green" }
        ]
      }
    },
    {
      "title": "Player Sessions",
      "type": "heatmap",
      "datasource": "Prometheus",
      "targets": [
        {
          "expr": "rate(gate_connection_duration_bucket[5m])",
          "legendFormat": "{{le}}"
        }
      ]
    },
    {
      "title": "Command Executions",
      "type": "timeseries",
      "datasource": "Prometheus",
      "targets": [
        {
          "expr": "sum(rate(gate_command_executions_total[5m])) by (command)",
          "legendFormat": "{{command}}"
        }
      ]
    },
    {
      "title": "Server Load",
      "type": "timeseries",
      "datasource": "Prometheus",
      "targets": [
        {
          "expr": "sum(rate(gate_server_connections_total[5m])) by (server)",
          "legendFormat": "{{server}}"
        }
      ]
    }
  ],
  "rows": [
    {
      "panels": [
        {
          "title": "Player Logs",
          "type": "logs",
          "datasource": "Loki",
          "targets": [
            {
              "expr": "{job=\"gate\"} |= \"player\""
            }
          ]
        }
      ]
    },
    {
      "panels": [
        {
          "title": "Traces",
          "type": "traces",
          "datasource": "Tempo",
          "targets": [
            {
              "query": "service.name=\"gate\""
            }
          ]
        }
      ]
    }
  ]
}
```

## Anonymous Metrics

When enabled, Gate sends the following anonymous data:
- Random installation ID
- Gate version
- Operating system and architecture
- Number of connected players (aggregate only)
- Performance metrics (TPS, latency histograms)
- Error rates and types
- Feature usage statistics

This data helps:
- Identify performance bottlenecks
- Prioritize features and fixes
- Understand deployment patterns
- Improve stability

No personal data or player information is collected.