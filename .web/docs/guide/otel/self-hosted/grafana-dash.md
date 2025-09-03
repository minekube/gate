---
title: 'Gate Grafana Dashboard Configuration'
description: 'Configure Grafana dashboards for Gate Minecraft proxy. Visual monitoring of connections, performance metrics, and server health indicators.'
---

We provide a sample Grafana dashboard to help you get started with visualizing Gate's metrics.

::: info <VPBadge>You are expected to make your own dashboard, this is just a starting point.</VPBadge>

![Grafana Dashboard](/images/grafana-gate-dash.png)

:::

**Dashboard Features:**

- Total Player Count (`proxy_player_count`)
- Gate Instance Status (`up`)
- Go Goroutines (`go_goroutines`)
- Go Memory Usage (`go_memstats_alloc_bytes`)

**Get the Dashboard JSON:**

- **Download Raw JSON:** [Download Dashboard JSON](https://raw.githubusercontent.com/minekube/gate/master/.web/docs/guide/otel/self-hosted/grafana-dashboards/gate-overview-dashboard.json)
- **View on GitHub:** [gate-overview-dashboard.json](https://github.com/minekube/gate/blob/master/.web/docs/guide/otel/self-hosted/grafana-dashboards/gate-overview-dashboard.json)

If you have cloned the repository, you can also find the dashboard at `.web/docs/guide/otel/self-hosted/grafana-dashboards/gate-overview-dashboard.json` within your local copy.

**Importing the Dashboard:**

1.  Navigate to your Grafana instance (usually http://localhost:3000).
2.  Log in (default: admin/admin, then change the password).
3.  On the left-hand menu, go to **Dashboards**.
4.  On the Dashboards page, click the **"New"** button in the top right and select **"Import"**.
    ![Grafana Menu Dashboards](/images/grafana-new-dash.png)
5.  Click the **"Upload JSON file"** button and select the `gate-overview-dashboard.json` file you downloaded, or paste the JSON content directly into the text area.
6.  On the next screen, you can change the dashboard name if desired.
7.  **Important:** Select your Prometheus data source from the dropdown (usually named "Prometheus").
    ![Grafana Import Dashboard](/images/grafana-import-dash.png)
8.  Click **"Import"**.

You should now see the "Gate Overview" dashboard with panels visualizing metrics from your Gate instance(s).
