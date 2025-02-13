# OpenTelemetry

Gate uses OpenTelemetry for observability, leveraging the [otel-config-go](https://github.com/honeycombio/otel-config-go) library for configuration. This provides a simple way to configure tracing and metrics collection through environment variables.

## Configuration

Gate's OpenTelemetry implementation can be configured using the following environment variables:

| Environment Variable        | Required | Default                | Description               |
| --------------------------- | -------- | ---------------------- | ------------------------- |
| OTEL_SERVICE_NAME           | No       | `gate`                 | Name of your service      |
| OTEL_SERVICE_VERSION        | No       | -                      | Version of your service   |
| OTEL_EXPORTER_OTLP_ENDPOINT | No       | `localhost:4317`       | Endpoint for OTLP export  |
| OTEL_LOG_LEVEL              | No       | `info`                 | Logging level             |
| OTEL_PROPAGATORS            | No       | `tracecontext,baggage` | Configured propagators    |
| OTEL_METRICS_ENABLED        | No       | `true`                 | Enable metrics collection |
| OTEL_TRACES_ENABLED         | No       | `true`                 | Enable trace collection   |

Additional environment variables for exporters:

| Environment Variable                | Required | Default          | Description                          |
| ----------------------------------- | -------- | ---------------- | ------------------------------------ |
| OTEL_EXPORTER_OTLP_HEADERS          | No       | `{}`             | Global headers for OTLP exporter     |
| OTEL_EXPORTER_OTLP_TRACES_HEADERS   | No       | `{}`             | Headers specific to trace exporter   |
| OTEL_EXPORTER_OTLP_METRICS_HEADERS  | No       | `{}`             | Headers specific to metrics exporter |
| OTEL_EXPORTER_OTLP_PROTOCOL         | No       | `grpc`           | Protocol for OTLP export (grpc/http) |
| OTEL_EXPORTER_OTLP_TRACES_ENDPOINT  | No       | `localhost:4317` | Endpoint for trace export            |
| OTEL_EXPORTER_OTLP_TRACES_INSECURE  | No       | `false`          | Allow insecure trace connections     |
| OTEL_EXPORTER_OTLP_METRICS_ENDPOINT | No       | `localhost:4317` | Endpoint for metrics export          |
| OTEL_EXPORTER_OTLP_METRICS_INSECURE | No       | `false`          | Allow insecure metrics connections   |
| OTEL_EXPORTER_OTLP_METRICS_PERIOD   | No       | `30s`            | Metrics reporting interval           |
| OTEL_RESOURCE_ATTRIBUTES            | No       | -                | Additional resource attributes       |

## Example Configuration

Here's an example configuration for sending telemetry to a local OpenTelemetry collector:

```env
OTEL_SERVICE_NAME="my-gate-service"
OTEL_EXPORTER_OTLP_ENDPOINT="localhost:4317"
OTEL_EXPORTER_OTLP_PROTOCOL="grpc"
OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"
```

## Observability Solutions

You can use various solutions to collect and visualize OpenTelemetry data. Here are some popular options:

### Cloud Solutions

::: info <VPBadge>Our recommendations</VPBadge>

- [Grafana Cloud](/guide/otel/grafana-cloud/) - Fully managed observability platform with support for metrics, logs, and traces
- [Honeycomb](/guide/otel/honeycomb/) - Observability platform designed for debugging complex systems

:::

- [New Relic](https://newrelic.com/) - Full-stack observability platform with APM capabilities
- [Datadog](https://www.datadog.com/) - Cloud monitoring and analytics platform
- [Azure Monitor](https://azure.microsoft.com/services/monitor/) - Microsoft's cloud-native monitoring solution
- [AWS X-Ray](https://aws.amazon.com/xray/) - Distributed tracing system for AWS applications
- [Google Cloud Operations Suite](https://cloud.google.com/operations) - Formerly Stackdriver, for monitoring, logging, and diagnostics

### Self-Hosted Solutions

#### Tracing

- [Tempo](https://grafana.com/oss/tempo/) - Grafana Tempo is a high-scale distributed tracing backend
- [Jaeger](https://www.jaegertracing.io/) - Open source, end-to-end distributed tracing

#### Metrics

- [Mimir](https://grafana.com/oss/mimir/) - Grafana Mimir is a highly scalable Prometheus solution

#### Visualization

- [Grafana](https://grafana.com/oss/grafana/) - The open and composable observability and data visualization platform

## Best Practices

1. **Service Name**: Always set a meaningful `OTEL_SERVICE_NAME` that clearly identifies your service.

   ```env
   # Good examples:
   OTEL_SERVICE_NAME="gate-proxy-eu"
   OTEL_SERVICE_NAME="gate-proxy-lobby"

   # Bad examples:
   OTEL_SERVICE_NAME="proxy"  # too generic
   OTEL_SERVICE_NAME="gate"   # not specific enough
   ```

2. **Service Version**: Set `OTEL_SERVICE_VERSION` to track your application version:

   ```env
   # Semantic versioning
   OTEL_SERVICE_VERSION="v1.2.3"

   # Git commit hash
   OTEL_SERVICE_VERSION="git-8f45d91"

   # Build number
   OTEL_SERVICE_VERSION="build-1234"
   ```

3. **Resource Attributes**: Use `OTEL_RESOURCE_ATTRIBUTES` to add important context like environment, region, or deployment info.

   ```env
   # Single attribute
   OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production"

   # Multiple attributes
   OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,cloud.region=eu-west-1,kubernetes.namespace=game-servers"

   # With detailed context
   OTEL_RESOURCE_ATTRIBUTES="deployment.environment=production,service.instance.id=gate-1,cloud.provider=aws,cloud.region=us-east-1"
   ```

4. **Security**: In production environments:

   ```env
   # Secure endpoint configuration
   OTEL_EXPORTER_OTLP_ENDPOINT="https://otel-collector.example.com:4317"
   OTEL_EXPORTER_OTLP_HEADERS="api-key=secret123,tenant=team-a"

   # Ensure TLS is enabled
   OTEL_EXPORTER_OTLP_INSECURE=false
   ```

## Further Reading

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [otel-config-go Repository](https://github.com/honeycombio/otel-config-go)
- [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
