# Use Ubuntu as base image for proper Linux environment
FROM ubuntu:22.04

# Set environment variables to avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# Install system dependencies
RUN apt-get update && apt-get install -y \
    wget \
    curl \
    git \
    build-essential \
    sqlite3 \
    ca-certificates \
    sudo \
    adduser \
    passwd \
    && rm -rf /var/lib/apt/lists/*

# Install Go 1.23 - detect architecture automatically
RUN ARCH=$(dpkg --print-architecture) && \
    if [ "$ARCH" = "amd64" ]; then GOARCH="amd64"; elif [ "$ARCH" = "arm64" ]; then GOARCH="arm64"; else GOARCH="amd64"; fi && \
    wget https://go.dev/dl/go1.23.3.linux-${GOARCH}.tar.gz && \
    tar -C /usr/local -xzf go1.23.3.linux-${GOARCH}.tar.gz && \
    rm go1.23.3.linux-${GOARCH}.tar.gz

# Set Go environment variables
ENV PATH="/usr/local/go/bin:${PATH}"
ENV GOPATH="/go"
ENV GOBIN="/go/bin"
ENV CGO_ENABLED=1

# Create Go workspace
RUN mkdir -p /go/bin /go/src /go/pkg

# Create application user
RUN useradd -m -s /bin/bash -u 1000 deployer && \
    usermod -aG sudo deployer

# Create necessary directories for the application
RUN mkdir -p /var/lib/deployer/users \
    /var/lib/deployer/chroot \
    /app/data \
    /app/deployments/logs \
    /app/web/static && \
    chown -R deployer:deployer /var/lib/deployer /app

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Install templ CLI
RUN go install github.com/a-h/templ/cmd/templ@latest

# Copy the entire projectc
COPY . .

# Change ownership of copied files
RUN chown -R deployer:deployer /app

# Switch to deployer user
USER deployer

# Set PATH for deployer user
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}"

# Generate templ files and build the application
RUN templ generate && \
    go build -o deployer cmd/server/main.go

# Create a startup script that ensures proper permissions
USER root
RUN echo '#!/bin/bash' > /start.sh && \
    echo '' >> /start.sh && \
    echo '# Ensure directories exist and have proper permissions' >> /start.sh && \
    echo 'mkdir -p /var/lib/deployer/users /var/lib/deployer/chroot /app/data /app/deployments/logs' >> /start.sh && \
    echo 'chown -R deployer:deployer /app/data /app/deployments' >> /start.sh && \
    echo 'chmod 755 /var/lib/deployer /var/lib/deployer/users /var/lib/deployer/chroot' >> /start.sh && \
    echo '' >> /start.sh && \
    echo '# Start the application as root (needed for user creation)' >> /start.sh && \
    echo 'cd /app && exec ./deployer' >> /start.sh && \
    chmod +x /start.sh

# Expose port 8080
EXPOSE 8080

# Use the startup script
CMD ["/start.sh"] 