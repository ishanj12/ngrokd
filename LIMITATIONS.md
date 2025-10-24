# Known Limitations

## Kubernetes Operator Resource Creation

### Issue

When using automatic certificate provisioning (`--api-key` flag), the agent registers as a **Kubernetes Operator** with ngrok to obtain mTLS certificates. This creates a `KubernetesOperator` resource in your ngrok account.

### Why?

The agent connects to `kubernetes-binding-ingress.ngrok.io:443` which requires mTLS client certificates signed by ngrok's Certificate Authority. Currently, the **only API** to obtain these signed certificates is:

```
POST /kubernetes_operators
```

This endpoint:
1. Accepts a Certificate Signing Request (CSR)
2. Signs it with ngrok's CA
3. Returns a valid certificate for bindings authentication
4. Creates a KubernetesOperator resource as a side effect

### Impact

**What gets created:**
- A KubernetesOperator resource in your ngrok account
- Visible in ngrok dashboard under operators
- Named: "ngrok forward proxy agent" (or custom `--description`)
- ⚠️ **Must remain**: Deleting the operator invalidates the certificate (mTLS validation requires active registration)

**What does NOT happen:**
- ❌ No Kubernetes cluster is created
- ❌ No Kubernetes resources are deployed
- ❌ No pods or services are created
- ❌ The agent runs completely standalone

### Workarounds

**Option 1: Accept the Limitation**
- Use auto-provisioning as-is
- Operator resource is harmless (just metadata in ngrok account)
- ⚠️ **Must keep operator** - certificate authentication requires it to remain active
- One operator per agent instance (certificates are bound to operator ID)

**Option 2: Manual Certificate Provisioning**
- Extract certificates from an existing Kubernetes operator:
  ```bash
  kubectl get secret ngrok-operator-default-tls -n ngrok-op \
    -o jsonpath='{.data.tls\.crt}' | base64 -d > tls.crt
  kubectl get secret ngrok-operator-default-tls -n ngrok-op \
    -o jsonpath='{.data.tls\.key}' | base64 -d > tls.key
  ```
- Use `--cert` and `--key` flags instead of `--api-key`

**Option 3: Reuse Certificates**
1. Run agent with `--api-key` once to get certificates
2. Certificates saved to `~/.ngrok-forward-proxy/certs/`
3. Subsequent runs use cached certificates (no new operator resources)
4. ⚠️ **Cannot delete operator** - certificate validation requires the operator registration to remain active

### Future Solution

We are tracking this issue and hope ngrok will provide a dedicated API endpoint for standalone agent certificate provisioning:

```
POST /agent_certificates  (proposed)
{
  "csr": "...",
  "description": "Standalone forward proxy agent",
  "type": "bindings-forwarder"
}
```

This would eliminate the need to create operator resources.

### Technical Details

**Why mTLS is Required:**
- `kubernetes-binding-ingress.ngrok.io:443` uses mutual TLS for authentication
- Each certificate is tied to a specific registration
- Provides isolation between different deployments
- No token-based auth alternative exists for this endpoint

**Certificate Lifecycle:**
- Certificates are ECDSA P-384 key pairs
- Signed by ngrok's internal CA
- Stored locally in `~/.ngrok-forward-proxy/certs/`
- Reused across agent restarts (provisioned once)

### Why Can't We Delete the Operator?

The certificate validation is **tied to the operator registration**:

1. **Certificate Binding**: Certificates are issued for a specific operator ID (e.g., `k8sop_2i...`)
2. **Active Validation**: ngrok validates certificates against the registered operator on each connection
3. **Endpoint Registry**: Bound endpoints are associated with the operator ID
4. **No Grace Period**: Deleting the operator immediately invalidates all associated certificates

**Technical:** The mTLS handshake verifies the client certificate against ngrok's operator registry. Without an active operator registration, authentication fails.

### Questions?

If you have concerns about operator resource creation or need a cleaner solution, please:
1. Contact ngrok support to request standalone agent certificate API
2. Open an issue in this repository
3. Use manual certificate provisioning as an alternative

**Note:** Each agent instance can reuse the same operator/certificate. Multiple agents can share certificates if desired (copy from `~/.ngrok-forward-proxy/certs/`).
