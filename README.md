# k8sctl

<h1>Manage Talos Kubernetes Clusters</h1>

A CLI tool for managing Talos Kubernetes Clusters behind Cloud Load Balancers with OIDC authentication.

## Authentication

k8sctl uses Dex OIDC with SSH key-based JWT token exchange for authentication. Users must be members of authorized groups configured on the server.

### Client Configuration

The client automatically authenticates using your SSH keys. Configuration can be provided via flags or environment variables:

- `DEX_URL` - Dex issuer URL for OIDC authentication
- `K8SCTL_CLIENT_ID` - OAuth2 client ID (has built-in default)
- `K8SCTL_CLIENT_SECRET` - OAuth2 client secret (has built-in default)
- `KUBECTL_SSH_USER` - Username for authentication

### Server Configuration

The server requires the following environment variables:

- `OIDC_ISSUER_URL` - Dex issuer URL (required, e.g., https://dex.example.com)
- `OIDC_AUDIENCE` - The URL of this k8sctl server (required, e.g., https://k8sctl-dev.example.com)
- `OIDC_ALLOWED_GROUPS` - Comma-separated list of allowed groups (optional, defaults to engineering)
- `CLOUDFLARE_API_TOKEN` - Cloudflare API token for DNS management (required)
- `CLOUDFLARE_ZONE_ID` - Cloudflare zone ID (required)

## Usage

### Cluster Operations

```bash
# Describe a cluster
k8sctl -c cluster1 cluster describe

# Reconcile cluster state
k8sctl -c cluster1 cluster reconcile

# Fix missing tags during reconciliation
k8sctl -c cluster1 cluster reconcile --fix-tags
```

### Node Operations

```bash
# Create a new node
k8sctl -c cluster1 node create --name cluster1-cp-4 --role controlplane

# Delete a node
k8sctl -c cluster1 node delete --name cluster1-worker-3

# Glass a node (destroy and recreate)
k8sctl -c cluster1 node glass --name cluster1-worker-1

# Describe a node
k8sctl -c cluster1 node describe --name cluster1-cp-1
```

### Monitoring

```bash
# Monitor cluster health (runs continuously)
k8sctl -c cluster1 monitor

# Custom monitoring interval
k8sctl -c cluster1 monitor --interval 30
```

### Authentication Check

```bash
# Verify authentication is working
k8sctl -c cluster1 auth-check
```

## Cluster Configuration

k8sctl uses configuration files to map cluster names to environments and server URLs. This keeps deployment-specific information out of the codebase.

### Configuration File Locations

k8sctl searches for configuration in the following order:

1. `K8SCTL_CONFIG` environment variable (custom path)
2. `./k8sctl.yaml` (current directory)
3. `~/.config/k8sctl/config.yaml` (user config)
4. `/etc/k8sctl/config.yaml` (system config)

### Configuration Format

Create a `k8sctl.yaml` file:

```yaml
# Default environment when cluster not found in mappings
default_environment: dev

# Cluster-specific configurations
clusters:
  cluster1:
    environment: dev

  staging-us:
    environment: staging
    server_url: https://k8sctl-staging.example.com

  prod-us:
    environment: prod
    server_url: https://k8sctl-prod.example.com
```

### Environment Variable Overrides

Override configuration at runtime:

- `K8SCTL_CONFIG` - Path to configuration file
- `K8SCTL_CLUSTER_SUFFIX` - Override environment suffix for all clusters
- `K8SCTL_SERVER_URL` - Override server URL for all clusters

### Priority Order

1. Environment variables (highest priority)
2. Configuration file cluster-specific settings
3. Configuration file defaults
4. Built-in defaults (lowest priority)

### Examples

See `k8sctl.yaml.example` and `k8sctl.yaml.minimal` for configuration examples.

## Server Mode

Run the k8sctl server:

```bash
export OIDC_ISSUER_URL="https://dex.example.com"
export OIDC_AUDIENCE="https://k8sctl-dev.example.com"
export CLOUDFLARE_API_TOKEN="your-token"
export CLOUDFLARE_ZONE_ID="your-zone-id"

k8sctl server
```

### Client Configuration

The client can use environment variables to override default server URLs:

- `K8SCTL_SERVER_URL` - Base URL of k8sctl server (e.g., https://k8sctl-dev.example.com)
- `K8SCTL_AUDIENCE` - OIDC audience for token generation (defaults to server URL)
- `DEX_URL` - Dex issuer URL for OIDC authentication
- `K8SCTL_CLIENT_ID` - OAuth2 client ID
- `K8SCTL_CLIENT_SECRET` - OAuth2 client secret
- `KUBECTL_SSH_USER` - Username for SSH-based authentication

## Development

### Building

```bash
make test   # Run tests
make lint   # Run linters
go build    # Build binary
```

### Testing

```bash
go test ./test/...
```

