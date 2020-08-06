---
title: "Upgrade to multi controlplane"
---

### Upgrading the cluster

After the initial setup, and getting some hands on experience with Talos it's time to upgrade to a more production worth system.
To upgrade from a single controlplane cluster to a multi controlplane cluster we have some requirements listed below.

In this guide we will upgrade a 3 node cluster which has a single controlplane node to a 3 controlplane node cluster. For this setup we assume a existing load balancer, Talos deployment strategy, and working network setup.
Since this is a advanced topic, we expect some familiarity with talosctl and kubectl.

#### Our environment

This guide uses a bare-metal talos cluster, this bare-metal cluster is booting from a matchbox PXE server and serves the configuration files from the matchbox build-in webserver.

The steps taken to upgrade to a multi controlplane can also be used for other types of installation.

#### Requirements

- Minimal 3 controlplane nodes available (including init node)
- Loadbalancer OR DNS access
- Access to Talosctl and Kubectl

### Verify settings

To make sure Talos pulls our adjusted configuration files on boot, we need to set some parameters.
The files we're going to check:

- init.yaml
- controlplane.yaml

We need to make sure these configs point to the correct endpoint, and have the correct certSANs.
If you generate the configs using the `talosctl gen config` command with the correct parameters this should already be set correctly.
We also need to make sure `persist: false` since we want to pull the config again.

> controlplane.yaml and init.yaml

``` yaml
version: v1alpha1
persist: false
.....
machine:
  certSANs:
    - control.example.local
.....
cluster:
  controlPlane:
    endpoint: https://control.example.local:6443
  apiServer:
    certSANs:
    - control.example.local
.....
```

### Upgrade nodes
