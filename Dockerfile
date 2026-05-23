FROM golang:1.23-bookworm AS builder
WORKDIR /opt/synapse-housekeeper/
COPY . ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o synapse-housekeeper main.go

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/cache/apt/archives/*
RUN addgroup --system matrix && adduser --system --group matrix
USER matrix
WORKDIR /opt/synapse-housekeeper/
COPY --from=builder /opt/synapse-housekeeper/synapse-housekeeper ./
ENTRYPOINT ["/opt/synapse-housekeeper/synapse-housekeeper"]
