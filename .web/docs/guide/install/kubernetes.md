# Running Gate in Kubernetes

_Gate ships in packaged [Docker](docker) images that you can deploy to [Kubernetes](https://kubernetes.io/)
<button style="border:none;padding:3px;border-radius:4px;vertical-align:bottom" id="play-vite-audio" onclick="document.getElementById('k8s-audio').play();"><svg style="height:2em;width:2em"><use href="/voice.svg#voice" /></svg></button>
<audio id="k8s-audio"><source src="./k8s.mp3" type="audio/mpeg"></audio>_


::: tip Prerequisites

- Kubernetes cluster - e.g. [kind](https://kind.sigs.k8s.io/), [talos](https://www.talos.dev/), [minikube](https://minikube.sigs.k8s.io/docs/start/), [rke2](https://github.com/rancher/rke2), [k0s](https://github.com/k0sproject/k0s), [k3d](https://k3d.io/), [k3s](https://k3s.io/), [k3os](https://k3os.io/), [rancher](https://github.com/rancher/rancher),  ...
- [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl) - Kubernetes command line tool
- Optionally [kustomize](https://kustomize.io/) - Kubernetes configuration management tool
- Optionally [Lens Desktop](https://github.com/lensapp/lens) - A cool Kubernetes IDE

:::

## All-in-One Minecraft Network Example

This Kubernetes manifest contains a Gate deployment
configuring two non-persistent Minecraft servers.

You can join Gate on NodePort `32556` and switch
between the two Minecraft servers using the `/server` command.

You can also try to delete one server pod and see how Gate
automatically reconnects the player to the other server.

```sh console
kubectl apply -f https://raw.githubusercontent.com/minekube/gate/master/.examples/kubernetes/bundle.yaml
```

::: details bundle.yaml

```yaml bundle.yaml
<!--@include: ../../../../.examples/kubernetes/bundle.yaml -->
```

:::


## Using Kustomize <VPBadge>Recommended</VPBadge>

Use [Kustomize](https://kustomize.io/) for
structuring your Kubernetes manifests in a more manageable way before deploying.
In fact, we used Kustomize to generate the [All-in-One example](#all-in-one-minecraft-network-example) above.

```sh console
git clone https://github.com/minekube/gate.git
cd gate/.examples/kubernetes # Edit to your needs
kustomize build # Preview generated manifest
kubectl apply -k . # Generate and deploy to Kubernetes
kubectl delete -k . # Delete deployment
```

You can also overlay your own Kustomize on top of the example:

```yaml kustomization.yaml
resources:
  - https://raw.githubusercontent.com/minekube/gate/master/.examples/kubernetes
  
# Edit to your needs...
patchesStrategicMerge:
  - patch.yaml
```
