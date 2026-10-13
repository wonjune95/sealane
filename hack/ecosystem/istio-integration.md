---
title: Sealane
description: Routing console for Istio and Gateway API.
weight: 70
keywords: [integration, routing, gateway-api]
owner: istio/wg-environments-maintainers
test: n/a
---

[Sealane](https://wonjune95.github.io/sealane/) is a web console for browsing and editing Istio and
Kubernetes Gateway API routing configuration, such as `VirtualService`, `DestinationRule`, `Gateway`,
`HTTPRoute` and `AuthorizationPolicy`. Resources are edited through forms generated from the CRD schema
or as YAML. Every write is validated with a server-side dry run, shown as a diff, and checked against
the object's `resourceVersion` before it is applied. A traffic flow map and a route checker show which
rule a request would match, using only the routing configuration.

Sealane runs as a single binary with no CRDs, operators or database. It reads the resources already in
the cluster, acts as its pod's ServiceAccount (or as the kubeconfigs it is given for remote clusters), and maps local or OpenID Connect users to `admin`,
`editor` and `viewer` roles. Sealane is a third-party project; it is not maintained by the Istio project.

## Install

{{< text bash >}}
$ helm install sealane oci://ghcr.io/wonjune95/charts/sealane -n sealane --create-namespace
$ kubectl -n sealane port-forward svc/sealane 8080:8080
{{< /text >}}

Open `http://localhost:8080` and log in as `admin` / `admin`; you are asked to change the password on
first login.

## Learn more

- [Sealane documentation](https://wonjune95.github.io/sealane/)
- [Source code](https://github.com/wonjune95/sealane)
