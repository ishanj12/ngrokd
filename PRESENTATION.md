# ngrokd

*~10 minutes · 6 slides*

---

## Slide 1: The Connectivity Gap

Enterprises are moving away from VPNs toward zero-trust, outbound-only connectivity — a market growing at 25% CAGR toward $15B by 2033. ngrok is already a leader here: customers like Databricks use ngrok to connect into thousands of customer networks without VPNs, firewall rules, or static IPs.

But today, ngrok connectivity is one-directional. The ngrok agent exposes local services *to* the internet. The K8s Operator projects bound endpoints *into* Kubernetes clusters. What's missing is the reverse: **consuming bound endpoints from environments that aren't Kubernetes.**

A SaaS vendor running their data plane on bare-metal Linux needs to reach a control plane behind ngrok. A developer on a Mac wants to `curl` a bound endpoint. A CI pipeline needs to hit staging services. Today, none of these work without a Kubernetes cluster running the Operator.

**ngrokd fills this gap.**

---

## Slide 2: What Is ngrokd?

ngrokd is a standalone daemon that makes ngrok bound endpoints reachable from any Linux or macOS machine — no Kubernetes required.

It uses the same mTLS protocol and binding forwarder logic as the K8s Operator, extracted into a single binary that runs anywhere. The daemon auto-discovers bound endpoints from the ngrok API, assigns each one a stable virtual IP on the local machine, and forwards traffic outbound through ngrok's cloud to the backend.

From the application's perspective, bound endpoints look like local services:

```
curl https://api.example.ngrok.app     →  resolves to 10.107.0.2 (local)
psql -h db.example.ngrok.app -p 5432   →  resolves to 10.107.0.3 (local)
```

No inbound firewall rules. No VPN. No Kubernetes. Just outbound mTLS connections from the daemon to ngrok.

---

## Slide 3: How It Works

```
┌─────────────────────────────────────────────────────────────┐
│                    Linux / macOS Machine                    │
│                                                             │
│   Application ──── https://api.example.ngrok.app ────┐      │
│                    (resolves via /etc/hosts or DNS)   │      │
│                                                      ▼      │
│                                                  ┌────────┐ │
│   ngrokd daemon:                                 │listener│ │
│     • Polls ngrok API for bound endpoints        │10.107. │ │
│     • Allocates virtual IP per endpoint          │ 0.2:443│ │
│     • Manages /etc/hosts (exact endpoints)       └───┬────┘ │
│     • Split-DNS for wildcards (*.example.com)        │      │
│     • mTLS cert auto-provisioning                    │      │
│     • Hot-reload config, health API                  │      │
└──────────────────────────────────────────────────────┼──────┘
                                                       │
                                              outbound mTLS
                                                       │
                                                       ▼
                              kubernetes-binding-ingress.ngrok.io:443
                                                       │
                                                       ▼
                                              Bound Endpoint
                                                       │
                                                       ▼
                                              Backend Service
```

---

## Slide 4: Why This Matters for Customers

**1. BYOC (Bring Your Own Cloud) architectures**
SaaS companies deploying data planes into customer environments need those data planes to reach back to a control plane. ngrokd gives them a stable local address for every bound endpoint — no VPN, no static IPs, no firewall changes. This is the same pattern Databricks uses with ngrok, but now available outside Kubernetes.

**2. Developer experience**
Developers working locally can `curl` bound endpoints the same way they would inside a cluster. No port-forwarding, no kubectl proxy, no tunnel setup. The daemon handles everything.

**3. Multi-environment connectivity**
CI runners, staging VMs, on-prem servers, Docker containers — any environment that can run a Linux or macOS binary can now consume bound endpoints. One daemon, auto-discovers everything, persistent IPs across restarts.

**4. Wildcard endpoint support**
ngrokd auto-detects wildcard endpoints (*.example.com), starts a per-domain split-DNS resolver, and routes connections by TLS SNI or HTTP Host header. This was previously only possible inside Kubernetes.

---

## Slide 5: What This Means for ngrok

**Bound endpoints become a universal connectivity primitive, not a Kubernetes-only feature.**

Today, bound endpoints require the K8s Operator to be useful. ngrokd removes that constraint. Any machine — a developer laptop, a CI runner, a bare-metal production server — can consume bound endpoints. This expands the addressable market for ngrok's private connectivity story beyond Kubernetes-native teams to any organization running hybrid or multi-environment infrastructure.

ngrok's existing site-to-site connectivity story is about reaching *into* customer networks. ngrokd completes the picture: reaching *out from* any environment to services published through ngrok. Together, they make ngrok a bidirectional private connectivity platform — the same value proposition that drives the $15B ZTNA market, but without the complexity of traditional zero-trust networking products.

This also creates a natural upsell path: customers start with the free agent for dev tunnels, adopt bound endpoints for cross-cluster connectivity, and now can extend that connectivity to every environment in their stack with ngrokd.

---

## Slide 6: What's Next

**Shipped:**
- Auto-discovery of bound endpoints via ngrok API
- Virtual IP allocation with per-endpoint listeners
- mTLS forwarding using the K8s Operator's binding protocol
- /etc/hosts management for exact endpoints, split-DNS for wildcards
- Linux and macOS support, Docker-ready
- CLI tool (ngrokctl), health monitoring, hot-reload config, crash recovery

**Next:**
- Windows support
- Dedicated certificate provisioning API — today ngrokd registers as a KubernetesOperator to get mTLS certs, which works but creates an unnecessary resource. A `POST /agent_certificates` endpoint would make this clean.
- Connection pooling for high-throughput production use cases
- First-party integration: `ngrok service bind` as an official agent mode

---

*github.com/ngrok/ngrokd*
