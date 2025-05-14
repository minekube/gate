# OpenTelemetry FAQ

This FAQ addresses common questions about using OpenTelemetry with Gate, particularly in conjunction with Grafana Mimir, Grafana Tempo, and the OpenTelemetry Collector.

## What does a recommended scalable OpenTelemetry setup for Gate look like?

A recommended scalable setup involves:

1.  **Gate**: Your application, instrumented with OpenTelemetry, emitting metrics and traces via OTLP.
2.  **OpenTelemetry Collector**: Receives OTLP data from Gate, processes it (batching, filtering, enrichment), and exports it.
3.  **Grafana Mimir**: A scalable, long-term storage backend for metrics, receiving data from the OTel Collector (e.g., via Prometheus remote write or OTLP). It's queried using PromQL.
4.  **Grafana Tempo**: A scalable backend for traces, receiving OTLP traces from the OTel Collector.
5.  **Grafana OSS**: The visualization platform, connecting to Mimir (for metrics) and Tempo (for traces).

The data flow generally looks like this:
`Gate (OTLP) -> OTel Collector -> Grafana Mimir (Metrics) & Grafana Tempo (Traces) -> Grafana OSS (Visualization)`

## In a setup with Grafana Mimir, is a separate Prometheus server still deployed?

Generally, no. Grafana Mimir takes on the role of the scalable metrics backend, handling storage and PromQL querying. You wouldn't typically deploy and manage a separate, standalone Prometheus server for its own data storage in this scenario. However, Prometheus concepts and technologies are still integral:

- **PromQL**: Used to query metrics from Mimir.
- **Exposition Format**: Applications might still expose metrics in the Prometheus format.
- **Collection Mechanisms**: The OTel Collector might use its Prometheus receiver, or Mimir ingests data via Prometheus remote write.

Mimir effectively becomes your Prometheus-compatible, scalable metrics datastore and query engine.

## What is the role of the OpenTelemetry Collector? Can applications like Gate push data directly to backends like Prometheus or Mimir?

The OpenTelemetry Collector is a crucial component that acts as a telemetry processing and routing pipeline. While some backends (including newer versions of Prometheus and potentially Mimir) can accept OTLP data directly, the Collector offers significant advantages:

- **Decoupling**: Applications only need to send data to the Collector, which then handles routing to various backends. This simplifies application configuration and makes it easier to change or add backends.
- **Processing**: The Collector can batch data for efficiency, filter unwanted telemetry, enrich data with additional attributes (e.g., Kubernetes metadata), and handle export retries.
- **Protocol Translation**: It can convert telemetry data between different protocols if needed.
- **Standardization**: It promotes a standardized way of handling telemetry data before it reaches the backends.

For these reasons, even if direct sending is possible, using the OTel Collector is often the recommended approach for flexibility and robustness.

## How do Grafana Mimir and Grafana Tempo fit into this?

- **Grafana Mimir**: Serves as a highly scalable, long-term storage solution for Prometheus metrics. It addresses potential scaling limitations of a single Prometheus instance for large data volumes and long retention periods. It remains compatible with PromQL for querying.
- **Grafana Tempo**: Is a highly scalable, easy-to-operate distributed tracing backend. It's optimized for ingesting and retrieving traces by ID and integrates well with Grafana for visualization and correlation with metrics and logs.
