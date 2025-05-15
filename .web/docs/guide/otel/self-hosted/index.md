# Self-Hosted OpenTelemetry Solutions

This guide provides instructions on how to set up self-hosted OpenTelemetry backends using Docker Compose and configure Gate to send telemetry data (traces and metrics) to them.

We will cover two common scenarios:

1.  **Grafana Stack**: Grafana for visualization, Prometheus for metrics, and Tempo for traces.
2.  **Jaeger**: All-in-one Jaeger for tracing.

## Scenario 1: Grafana, Prometheus & Tempo

> **Quick Start: Get the Configs**
>
> To get the configuration files for this scenario:
>
> 1. Clone the repository (if you haven't already):
>    ```bash
>    git clone https://github.com/minekube/gate.git
>    ```
> 2. Navigate to the directory for this scenario:
>    ```bash
>    cd gate/.web/docs/guide/otel/self-hosted/otel-stack-configs
>    ```
>    _(Adjust the `cd` path if you cloned into a different parent directory or are already inside the `gate` repo directory, e.g., `cd .web/docs/guide/otel/self-hosted/otel-stack-configs`)_
>
> Or, [view the files directly on GitHub](https://github.com/minekube/gate/tree/master/.web/docs/guide/otel/self-hosted/otel-stack-configs/).

This setup uses Grafana Tempo for traces, Prometheus for metrics, and Grafana for visualizing both. Gate will send traces directly to Tempo via OTLP. Metrics are also sent via OTLP using `OTEL_METRICS_ENABLED="true"`.

### 1. Configuration Files

You'll need the following configuration files. The `docker-compose.yml` below assumes `prometheus.yml`, `tempo.yaml`, and `grafana-datasources.yml` are in the same directory (e.g., `otel-stack-configs/`) when you run `docker compose -f otel-stack-configs/docker-compose.yml up -d` from the directory containing `otel-stack-configs`.

::: code-group

```yaml [docker-compose.yml]
<!--@include: ./otel-stack-configs/docker-compose.yml -->
```

```yaml [prometheus.yml]
<!--@include: ./otel-stack-configs/prometheus.yml -->
```

```yaml [tempo.yaml]
<!--@include: ./otel-stack-configs/tempo.yaml -->
```

```yaml [grafana-datasources.yml]
<!--@include: ./otel-stack-configs/grafana-datasources.yml -->
```

:::

### 2. Configure Gate Environment Variables

To send telemetry data from Gate to this self-hosted stack:

**For Traces and Metrics (to OpenTelemetry Collector via OTLP/HTTP):**

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318" # Or IP of Docker host if Gate is external
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_TRACES_ENABLED="true"
export OTEL_METRICS_ENABLED="true" # Ensure metrics are enabled to be sent via OTLP
# OTEL_SERVICE_NAME is recommended, e.g., "gate-proxy"
```

> **Note on Insecure Connection:** Since the `OTEL_EXPORTER_OTLP_ENDPOINT` is set to an `http://` address (e.g., `http://localhost:4318`) and `OTEL_EXPORTER_OTLP_PROTOCOL` is `http/protobuf`, the connection to the OTLP receiver (OpenTelemetry Collector in this case) will be insecure (not using TLS). This is typically handled automatically by the OTel SDK when an `http://` scheme is used with an HTTP-based protocol.
> For self-hosted setups like this one, especially in local development, using an insecure connection is common. In production environments, always prefer secure `https://` endpoints and appropriate authentication mechanisms.

If Gate is running as a Docker container itself _on the same Docker network_ (`otel-stack`), you can use the service name:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://otel-collector:4318"
```

### 3. Running the Stack

1.  Save all the configuration files (`docker-compose.yml`, `prometheus.yml`, `tempo.yaml`, `grafana-datasources.yml`, and `otel-collector-config.yaml`) in the same directory.
2.  Open a terminal in that directory and run:
    ```bash
    docker compose -f otel-stack-configs/docker-compose.yml up -d
    ```
3.  **Access Services:**
    - Grafana: http://localhost:3000 (admin/admin, then change password)
    - Prometheus: http://localhost:9090
    - Tempo: http://localhost:3200 (for Tempo's own UI, though Grafana is primary)

### 4. Understanding the Stack Architecture

This setup uses the following components:

- **OpenTelemetry Collector**: Receives traces and metrics from Gate via OTLP, processes them, and forwards traces to Tempo and exposes metrics for Prometheus to scrape.
- **Tempo**: Stores and indexes traces for efficient querying.
- **Prometheus**: Scrapes and stores metrics from the OpenTelemetry Collector.
- **Grafana**: Provides visualization for both traces and metrics, with correlation between them.

The data flow is as follows:

1. Gate sends both traces and metrics to the OpenTelemetry Collector via OTLP.
2. The collector forwards traces to Tempo for storage.
3. The collector exposes metrics on port 8889, which Prometheus scrapes.
4. Grafana queries both Tempo and Prometheus to provide a unified view of your telemetry data.

This architecture allows for:

- Efficient collection and processing of telemetry data
- Correlation between traces and metrics
- Service graphs and span metrics for better visualization
- Scalability as your observability needs grow

### 5. Viewing Data in Grafana

- **Prometheus**:
  - The Prometheus data source should be automatically provisioned.
  - Go to "Explore", select "Prometheus", and you can query metrics like `prometheus_http_requests_total`. To view Gate metrics, you'd first need to set up an OpenTelemetry Collector to receive OTLP metrics from Gate and expose them to Prometheus.
- **Tempo**:
  - The Tempo data source should also be automatically provisioned.
  - Go to "Explore", select "Tempo". You can search for traces by Service Name (e.g., your `OTEL_SERVICE_NAME` for Gate), or look at the Service Graph (if `metrics_generator` in Tempo is working correctly and sending data to Prometheus).
  - If you have metrics that can be correlated with traces (like exemplars), you might be able to jump from metrics in Prometheus to traces in Tempo.

### 6. Sample Gate Dashboard

<!--@include: ./grafana-dash.md -->

---

## Scenario 2: All-in-one Jaeger

> **Quick Start: Get the Configs**
>
> To get the configuration file for this scenario:
>
> 1. Clone the repository (if you haven't already):
>    ```bash
>    git clone https://github.com/minekube/gate.git
>    ```
> 2. Navigate to the directory for this scenario:
>    ```bash
>    cd gate/.web/docs/guide/otel/self-hosted/jaeger-config
>    ```
>    _(Adjust the `cd` path if you cloned into a different parent directory or are already inside the `gate` repo directory, e.g., `cd .web/docs/guide/otel/self-hosted/jaeger-config`)_
>
> Or, [view the file directly on GitHub](https://github.com/minekube/gate/tree/master/.web/docs/guide/otel/self-hosted/jaeger-config/).

Jaeger is a popular open-source, end-to-end distributed tracing system. The `all-in-one` image is a quick way to get started with Jaeger for development and testing. It includes the Jaeger Collector (which can receive OTLP), Agent, Query service, and UI in a single container.

Gate can send traces directly to Jaeger using the OTLP exporter.

### 1. Docker Compose Configuration

Create a `docker-compose-jaeger.yml` file (or add to an existing one). You can place the following content into a file, for example, at `otel-jaeger-config/docker-compose.yml` relative to this document, and run `docker compose -f otel-jaeger-config/docker-compose.yml up -d`:

::: code-group

```yaml [docker-compose.yml]
<!--@include: ./jaeger-config/docker-compose.yml -->
```

:::

### 2. Configure Gate Environment Variables

To send traces from Gate to Jaeger:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318" # Or IP of Docker host if Gate is external
export OTEL_EXPORTER_OTLP_PROTOCOL="http/protobuf"
export OTEL_TRACES_ENABLED="true"
export OTEL_METRICS_ENABLED="false" # Jaeger does not support metrics
# The following INSECURE flag is necessary when using an http:// endpoint for traces:
export OTEL_EXPORTER_OTLP_TRACES_INSECURE="true"
# OTEL_SERVICE_NAME is highly recommended, e.g., "gate-proxy-dev"
export OTEL_SERVICE_NAME="gate-jaeger-example"
```

> **Note on Insecure Connection:** As with the Tempo setup, if your `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g., `http://localhost:4317`) uses an insecure `http://` connection, you **must** explicitly enable insecure connections for traces by setting `OTEL_EXPORTER_OTLP_TRACES_INSECURE="true"` as shown in the configuration example above. If also sending OTLP metrics via HTTP to Jaeger or a collector, `OTEL_EXPORTER_OTLP_METRICS_INSECURE="true"` would be needed too.
> Remember to use secure `https://` endpoints and authentication in production.

If Gate is running as a Docker container on the same Docker network (`otel-jaeger-net`), you can use the service name:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://jaeger:4317"
```

Note: For this Jaeger setup, metrics collection with Prometheus is not included. Jaeger primarily focuses on tracing. If you need metrics alongside traces sent via OTLP from Gate, you would typically use an OpenTelemetry Collector that can route traces to Jaeger and simultaneously expose metrics for Prometheus (or send them to another metrics backend).

### 3. Running Jaeger

1.  Save the `docker-compose-jaeger.yml` file.
2.  Open a terminal in that directory and run:
    ```bash
    docker compose -f docker-compose-jaeger.yml up -d
    ```
3.  **Access Jaeger UI:**
    - Open your browser and navigate to http://localhost:16686

### 4. Viewing Traces in Jaeger

- Once Gate is running and configured to send traces to Jaeger, you should be able to see your `OTEL_SERVICE_NAME` (e.g., "gate-jaeger-example") in the "Service" dropdown in the Jaeger UI.
- Select your service and click "Find Traces" to see the collected trace data.

---
