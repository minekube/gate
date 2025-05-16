# Self-Hosted Grafana, Prometheus & Tempo Stack

This guide provides instructions on how to set up a self-hosted Grafana, Prometheus, and Tempo stack using Docker Compose and configure Gate to send telemetry data (traces and metrics) to this stack.

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

You'll need the following configuration files. The `docker-compose.yml` below assumes these configuration files are in the same directory when you run `docker compose up -d` from within the `otel-stack-configs` directory (after navigating into it as shown in the Quick Start). The default setup uses a "push" model where the OpenTelemetry Collector sends metrics to Prometheus.

::: code-group

```yaml [docker-compose.yml]
<!--@include: ./otel-stack-configs/docker-compose.yml -->
```

```yaml [otel-collector-config-push.yaml]
<!--@include: ./otel-stack-configs/otel-collector-config-push.yaml -->
```

```yaml [prometheus-config-push.yml]
<!--@include: ./otel-stack-configs/prometheus-config-push.yml -->
```

```yaml [tempo.yaml]
<!--@include: ./otel-stack-configs/tempo.yaml -->
```

```yaml [grafana-datasources.yml]
<!--@include: ./otel-stack-configs/grafana-datasources.yml -->
```

:::

The OpenTelemetry Collector first receives telemetry data (traces and metrics) from Gate via OTLP. Its configuration for handling and forwarding this data, particularly metrics to Prometheus, is detailed below. The default "push" vs. alternative "pull" options refer to how metrics are sent from the Collector to Prometheus.
Refer to the comments within the main `docker-compose.yml` (included above) for instructions on how to set up the `otel-collector` and `prometheus` services for each approach.

::: code-group

```yaml [1. Push to Prometheus (otel-collector-config-push.yaml)]
# Collector receives from Gate (via OTLP receiver, see config) and then pushes metrics to Prometheus's remote_write endpoint.
# (This is the default setup shown in the main docker-compose.yml)
# otel-collector service in docker-compose.yml should use:
# command: ['--config=/etc/otel-collector-config-push.yaml']
# Prometheus service in docker-compose.yml should mount:
# - ./otel-stack-configs/prometheus-config-push.yml:/etc/prometheus/prometheus.yml
# And Prometheus service command in docker-compose.yml should include:
# - '--web.enable-remote-write-receiver'
<!--@include: ./otel-stack-configs/otel-collector-config-push.yaml -->
```

```yaml [prometheus-config-push.yml]
# Corresponding Prometheus config (enables remote_write receiver)
# Ensure Prometheus service in docker-compose.yml has the flag:
# command:
#   - '--config.file=/etc/prometheus/prometheus-config-push.yml'
#   - '--web.enable-lifecycle'
#   - '--web.enable-remote-write-receiver' # <--- This flag is active by default
<!--@include: ./otel-stack-configs/prometheus-config-push.yml -->
```

```yaml [2. Pull by Prometheus (otel-collector-config-pull.yaml)]
# ALTERNATIVE: Collector receives from Gate (via OTLP receiver, see config) and exposes metrics on :8889. Prometheus then scrapes (pulls) from the collector.
# To use this, modify docker-compose.yml:
# otel-collector service command: ['--config=/etc/otel-collector-config-pull.yaml']
# Prometheus service command: use '--config.file=/etc/prometheus/prometheus-config-pull.yml' (and consider removing --web.enable-remote-write-receiver if not needed for other purposes)
<!--@include: ./otel-stack-configs/otel-collector-config-pull.yaml -->
```

```yaml [prometheus-config-pull.yml]
# Corresponding Prometheus config for pull (scrapes otel-collector:8889)
<!--@include: ./otel-stack-configs/prometheus-config-pull.yml -->
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

1.  Save all the configuration files (`docker-compose.yml`, `prometheus-config-push.yml`, `tempo.yaml`, `grafana-datasources.yml`, and `otel-collector-config-push.yaml`) in the `otel-stack-configs` directory (if you haven't already from the Quick Start steps).
2.  Open a terminal, navigate into the `otel-stack-configs` directory, and run:
    ```bash
    docker compose up -d
    ```
3.  **Access Services:**
    Once the stack is running, you can access the UIs for the different services:

    | Service    | URL                   | Default Credentials             |
    | ---------- | --------------------- | ------------------------------- |
    | Grafana    | http://localhost:3000 | `admin` / `admin`               |
    | Prometheus | http://localhost:9090 | N/A                             |
    | Tempo      | http://localhost:3200 | N/A (UI via Grafana is primary) |

    For Grafana, you will be prompted to change the password after the first login.

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
