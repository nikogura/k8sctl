FROM golang:latest AS builder

ENV SRC_DIR="/go/src/github.com/nikogura"

RUN mkdir -p ${SRC_DIR}/k8sctl

COPY . ${SRC_DIR}/k8sctl

WORKDIR ${SRC_DIR}/k8sctl

RUN go build -o /go/bin/k8sctl .

FROM golang:latest

COPY --from=builder /go/bin/k8sctl /usr/local/bin/k8sctl

RUN apt update && apt install -y bash less awscli

RUN mkdir /app

WORKDIR /app

# Run the k8sctl server with OIDC authentication
# Configuration is provided via environment variables:
# - OIDC_ISSUER_URL (required, e.g., https://dex.example.com)
# - OIDC_AUDIENCE (required, e.g., https://k8sctl-dev.example.com)
# - OIDC_ALLOWED_GROUPS (optional, defaults to "engineering")
# - CLOUDFLARE_API_TOKEN (required)
# - CLOUDFLARE_ZONE_ID (required)
CMD ["k8sctl", "server"]
