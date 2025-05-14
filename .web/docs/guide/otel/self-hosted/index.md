# Self-Hosted OpenTelemetry Solutions

This guide provides instructions on how to set up self-hosted OpenTelemetry backends using Docker Compose and configure Gate to send telemetry data (traces and metrics) to them.

We will cover two common scenarios:

1.  **Grafana Stack**: Grafana for visualization, Prometheus for metrics, and Tempo for traces.
2.  **Jaeger**: All-in-one Jaeger for tracing.

## Scenario 1: Grafana, Prometheus & Tempo

This setup uses Grafana Tempo for traces, Prometheus for metrics, and Grafana for visualizing both. Gate will send traces directly to Tempo via OTLP and expose a Prometheus-compatible scrape endpoint for metrics.

### 1. Docker Compose Configuration

Create a `docker-compose.yml` file with the following content. You can place this in `otel-stack-configs/docker-compose.yml` relative to this document, and then the other config files (`prometheus.yml`, etc.) should be in the same `otel-stack-configs` directory when you run `docker compose -f otel-stack-configs/docker-compose.yml up -d` from the directory containing `otel-stack-configs`.

```yaml
<!--@include: ./otel-stack-configs/docker-compose.yml -->
```

### 2. Configuration Files

You'll need the following configuration files in the same directory as your `docker-compose.yml`:

**`prometheus.yml`:**

```yaml
<!--@include: ./otel-stack-configs/prometheus.yml -->
```

**`tempo.yaml`:**

```yaml
<!--@include: ./otel-stack-configs/tempo.yaml -->
```

**`grafana-datasources.yml`:**

```yaml
<!--@include: ./otel-stack-configs/grafana-datasources.yml -->
```

### 3. Configure Gate Environment Variables

To send telemetry data from Gate to this self-hosted stack:

**For Traces (to Tempo OTLP):**

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317" # Or IP of Docker host if Gate is external
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc" # Default, can be omitted
export OTEL_TRACES_ENABLED="true"
# OTEL_SERVICE_NAME is recommended, e.g., "gate-proxy"
```

If Gate is running as a Docker container itself _on the same Docker network_ (`otel-stack`), you can use the service name:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://tempo:4317"
```

**For Metrics (Prometheus scraping Gate):**
Ensure these Gate environment variables are set (they are enabled by default if not specified):

```bash
export OTEL_METRICS_ENABLED="true"
export OTEL_METRICS_SERVER_ENABLED="true"
export OTEL_METRICS_SERVER_ADDR=":9464" # Default, ensure Prometheus can reach this
export OTEL_METRICS_SERVER_PATH="/metrics" # Default
```

Update the `prometheus.yml` file (specifically the `gate` job's `targets`) to point to the address where Gate's metrics endpoint is exposed. If Docker is running on Linux, `host.docker.internal` might not resolve, and you may need to use the host's actual IP address on the Docker bridge network (e.g., `172.17.0.1`) or the machine's network IP if Gate is not containerized.

### 4. Running the Stack

1.  Save the `docker-compose.yml` and the configuration files (`prometheus.yml`, `tempo.yaml`, `grafana-datasources.yml`) in the same directory.
2.  Open a terminal in that directory and run:
    ```bash
    docker compose -f otel-stack-configs/docker-compose.yml up -d
    ```
3.  **Access Services:**
    - Grafana: `http://localhost:3000` (admin/admin, then change password)
    - Prometheus: `http://localhost:9090`
    - Tempo: `http://localhost:3200` (for Tempo's own UI, though Grafana is primary)

### 5. Viewing Data in Grafana

- **Prometheus**:
  - The Prometheus data source should be automatically provisioned.
  - Go to "Explore", select "Prometheus", and you can query metrics like `gate_info` (if Gate is running and scraped) or `prometheus_http_requests_total`.
- **Tempo**:
  - The Tempo data source should also be automatically provisioned.
  - Go to "Explore", select "Tempo". You can search for traces by Service Name (e.g., your `OTEL_SERVICE_NAME` for Gate), or look at the Service Graph (if `metrics_generator` in Tempo is working correctly and sending data to Prometheus).
  - If you have metrics that can be correlated with traces (like exemplars), you might be able to jump from metrics in Prometheus to traces in Tempo.

---

## Scenario 2: All-in-one Jaeger

Jaeger is a popular open-source, end-to-end distributed tracing system. The `all-in-one` image is a quick way to get started with Jaeger for development and testing. It includes the Jaeger Collector (which can receive OTLP), Agent, Query service, and UI in a single container.

Gate can send traces directly to Jaeger using the OTLP exporter.

### 1. Docker Compose Configuration

Create a `docker-compose-jaeger.yml` file (or add to an existing one). You can place the following content into a file, for example, at `otel-jaeger-config/docker-compose.yml` relative to this document, and run `docker compose -f otel-jaeger-config/docker-compose.yml up -d`:

```yaml
<!--@include: ./jaeger-config/docker-compose.yml -->
```

### 2. Configure Gate Environment Variables

To send traces from Gate to Jaeger:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4317" # Or IP of Docker host if Gate is external
export OTEL_EXPORTER_OTLP_PROTOCOL="grpc" # or "http/protobuf" for HTTP OTLP
export OTEL_TRACES_ENABLED="true"
# OTEL_SERVICE_NAME is highly recommended, e.g., "gate-proxy-dev"
export OTEL_SERVICE_NAME="gate-jaeger-example"
```

If Gate is running as a Docker container on the same Docker network (`otel-jaeger-net`), you can use the service name:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT="http://jaeger:4317"
```

Note: For this Jaeger setup, metrics collection with Prometheus is not included. Jaeger primarily focuses on tracing. If you need both metrics and traces, the Grafana stack (Scenario 1) or a more complex setup involving an OpenTelemetry Collector to route traces to Jaeger and metrics to Prometheus would be necessary.

### 3. Running Jaeger

1.  Save the `docker-compose-jaeger.yml` file.
2.  Open a terminal in that directory and run:
    ```bash
    docker compose -f docker-compose-jaeger.yml up -d
    ```
3.  **Access Jaeger UI:**
    - Open your browser and navigate to `http://localhost:16686`

### 4. Viewing Traces in Jaeger

- Once Gate is running and configured to send traces to Jaeger, you should be able to see your `OTEL_SERVICE_NAME` (e.g., "gate-jaeger-example") in the "Service" dropdown in the Jaeger UI.
- Select your service and click "Find Traces" to see the collected trace data.

---

## Scenario 3: Kubernetes with kube-prometheus-stack

The `kube-prometheus-stack` Helm chart by the Prometheus Community is a powerful and widely adopted solution for monitoring Kubernetes clusters. It bundles Prometheus, Grafana, Alertmanager, and various exporters to provide a full-fledged monitoring experience out of the box. You can find more about it on [Artifact Hub](https://artifacthub.io/packages/helm/prometheus-community/kube-prometheus-stack) and the [Prometheus Community GitHub](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack).

When Gate is deployed in such a Kubernetes cluster, you'll want Prometheus to scrape Gate's metrics and potentially send Gate's traces to a tracing backend that might be part of or integrated with this stack (like Jaeger or Grafana Tempo, which can also be deployed alongside or as part of `kube-prometheus-stack`'s capabilities).

### 1. Prerequisites

- A running Kubernetes cluster.
- `helm` CLI installed.
- `kube-prometheus-stack` Helm chart installed in your cluster. If not, you can typically install it via:
  ```bash
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo update
  helm install prometheus prometheus-community/kube-prometheus-stack --namespace monitoring --create-namespace
  ```
  (Refer to the official chart documentation for the most up-to-date installation instructions and configuration options.)

### 2. Configuring Gate for Metrics Scraping by Prometheus

Prometheus, as deployed by `kube-prometheus-stack`, typically uses Custom Resources like `ServiceMonitor` or `PodMonitor` to discover and scrape metrics endpoints.

**a. Ensure Gate's Metrics Endpoint is Exposed:**

First, make sure your Gate Kubernetes Deployment and Service are configured to expose the metrics port (default `:9464`).

Your Gate `Service` definition might look something like this:

```yaml
<!--@include: ./k8s-configs/gate-service.yml -->
```

**b. Create a `ServiceMonitor` for Gate:**

This is the recommended way for `kube-prometheus-stack` to discover your Gate metrics. A `ServiceMonitor` CRD (Custom Resource Definition) tells the Prometheus instances managed by the stack how to find and scrape your Gate pods.

It's important to understand that the `ServiceMonitor` doesn't mean Prometheus scrapes the _Service's virtual IP_. Instead, it uses the Service definition to find the underlying Pods:

1. The `ServiceMonitor` selects your Gate `Service` using labels.
2. Prometheus Operator (part of `kube-prometheus-stack`) then looks at the `Endpoints` object associated with that `Service`.
3. The `Endpoints` object contains the actual IP addresses of all individual Gate Pods selected by the `Service`.
4. Prometheus uses these individual Pod IPs as scrape targets. Thus, **each Gate pod is scraped directly.**

Create a `ServiceMonitor` custom resource like this:

```yaml
<!--@include: ./k8s-configs/gate-servicemonitor.yml -->
```

Apply this YAML to your cluster (`kubectl apply -f gate-servicemonitor.yaml`). Prometheus should then automatically discover and start scraping metrics from your individual Gate pods.

**Alternative: `PodMonitor` (More Direct Pod Discovery)**

If you prefer to discover pods directly by their labels without an intermediary `Service` definition, or if your Gate metrics endpoint isn't part of a regular Kubernetes `Service`, you can use a `PodMonitor`.

```yaml
<!--@include: ./k8s-configs/gate-podmonitor.yml -->
```

While `ServiceMonitor` is very common, `PodMonitor` offers a more direct path if needed.

**Alternative: Pod Annotations (Generally not recommended with `kube-prometheus-stack`)**

If `ServiceMonitor` or `PodMonitor` are not an option, or for simpler setups, Prometheus can also be configured to scrape pods based on annotations. You would add these annotations to your Gate Pod specification (e.g., in your Deployment template):

```yaml
<!--@include: ./k8s-configs/gate-pod-annotations-example.yml -->
```

However, `kube-prometheus-stack` is often configured _not_ to discover based on annotations by default to favor `ServiceMonitor`/`PodMonitor`. You might need to adjust the Prometheus configuration within the Helm chart's values if you want to rely solely on annotations.

**c. Gate Environment Variables for Metrics Server:**

Ensure Gate is configured to run its Prometheus metrics server (these are often default values):

```bash
export OTEL_METRICS_ENABLED="true"
export OTEL_METRICS_SERVER_ENABLED="true"
export OTEL_METRICS_SERVER_ADDR=":9464" # Ensure this matches targetPort in Service and port in ServiceMonitor
export OTEL_METRICS_SERVER_PATH="/metrics" # Ensure this matches path in ServiceMonitor if specified
```

### 3. Configuring Gate for Trace Export (OTLP)

If your `kube-prometheus-stack` deployment includes a tracing backend like Grafana Tempo (which can be enabled in the chart) or if you have a separate Jaeger/Tempo instance in your cluster that accepts OTLP:

- **Identify the OTLP Endpoint:** Determine the Kubernetes service name and port for your OTLP collector (e.g., `tempo-distributor.monitoring.svc.cluster.local:4317` or `jaeger-collector.tracing.svc.cluster.local:4317`).
- **Configure Gate Environment Variables for Traces:**

  ```bash
  export OTEL_TRACES_ENABLED="true"
  export OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="http://<your-otlp-collector-service-address>:4317" # e.g., http://tempo-distributor.monitoring:4317
  export OTEL_EXPORTER_OTLP_PROTOCOL="grpc" # or "http/protobuf"
  export OTEL_SERVICE_NAME="gate-k8s-my-cluster" # Choose a meaningful service name
  ```

  If your OTLP collector is in a different namespace, make sure to use the fully qualified domain name (FQDN) of the service (e.g., `my-collector.namespace.svc.cluster.local`).

### 4. Accessing Grafana and Viewing Data

The `kube-prometheus-stack` chart deploys Grafana, which comes pre-configured with Prometheus as a data source.

- **Access Grafana:** You can usually access Grafana by port-forwarding to the Grafana service:
  ```bash
  kubectl port-forward svc/prometheus-grafana 3000:80 -n monitoring
  # The service name might vary based on your Helm release name, e.g., prometheus-kube-prometheus-stack-grafana
  ```
  Then open `http://localhost:3000`. The default login is often `admin` with the password `prom-operator` (as noted in the [Atmosly guide](https://www.atmosly.com/blog/kube-prometheus-stack-a-comprehensive-guide-for-kubernetes-monitoring), but check your chart's documentation or secrets).
- **Viewing Metrics:** Your Gate metrics should appear in Prometheus. You can query them in Grafana using PromQL. The `kube-prometheus-stack` also comes with many pre-built dashboards for Kubernetes itself. You might want to create a new dashboard or customize an existing one to display Gate-specific metrics.
- **Viewing Traces:** If you've configured Gate to send traces to Tempo/Jaeger and set up that data source in Grafana (Tempo is often auto-configured if deployed by the same chart), you can explore traces via Grafana's "Explore" view.

This scenario leverages the power of `kube-prometheus-stack` for robust Kubernetes monitoring and integrates Gate into that ecosystem for both metrics and traces. The key is correctly defining how Prometheus discovers Gate (`ServiceMonitor` being the preferred method) and how Gate sends traces to your OTLP collector within the cluster.

---
