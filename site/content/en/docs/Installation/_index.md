---
title: "Install Gate"
linkTitle: "Installation"
weight: 5
description: >
  Instructions for installing Gate.
---

## Single Binary

Download the latest [Gate Release]({{< param releases >}})
pre-built for your target operating system (macOS/Windows/Linux).

{{< alert title="Which to choose?" color="info">}}
- Most users run on the **amd** platform.
    - Example: ...`windows_amd64.exe` for Windows
- Choose `darwin` if you are on macOS.
{{< /alert >}}


## Docker image

Gate also delivers a Docker image that can be easily run anywhere where Docker runs.
([Get Docker](https://docs.docker.com/get-docker/))

**Sample command:**
```shell script
docker run -it --rm \
  -p 25565:25565 \
  registry.gitlab.com/minekube/gate:latest
```

{{< alert title="Note" color="info">}}
We host the Docker image in our official GitLab group because GitHub would require you to
setup authentication credentials for pulling public images.
{{< /alert >}}