---
title: "Self-Hosted Jaeger Tracing with Gate"
description: "Set up self-hosted Jaeger for Gate Minecraft proxy tracing. Configure distributed tracing to monitor request flows and performance bottlenecks."
---

# Self-Hosted Jaeger with Docker Compose

This guide details setting up Jaeger (all-in-one) via Docker Compose for tracing with Gate.

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

Create a `docker-compose.yml` file (e.g., within the `jaeger-config` directory mentioned in the Quick Start). You can place the following content into this file and, after navigating into that directory, run `docker compose up -d`:

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
# OTEL_SERVICE_NAME is highly recommended, e.g., "gate-dev"
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

1.  Save the `docker-compose.yml` file (e.g. as `docker-compose.yml` inside your `jaeger-config` directory).
2.  Open a terminal in that directory (e.g., `jaeger-config`) and run:
    ```bash
    docker compose up -d
    ```
3.  **Access Jaeger UI:**
    - Open your browser and navigate to http://localhost:16686

### 4. Viewing Traces in Jaeger

- Once Gate is running and configured to send traces to Jaeger, you should be able to see your `OTEL_SERVICE_NAME` (e.g., "gate-jaeger-example") in the "Service" dropdown in the Jaeger UI.
- Select your service and click "Find Traces" to see the collected trace data.

---
