# Contributing to OpenGSLB

## Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- golangci-lint
- make
- git

### Quick Setup (Ubuntu/Pop!_OS)

```bash
./scripts/setup-dev-env.sh
```

Or install manually:

```bash
# Go
wget https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Docker
sudo apt install docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo usermod -aG docker $USER

# golangci-lint
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

## Development Workflow

### 1. Clone and Setup

```bash
git clone https://github.com/loganrossus/OpenGSLB.git
cd OpenGSLB
go mod download
```

### 2. Create Feature Branch

```bash
git checkout develop
git pull origin develop
git checkout -b feature/your-feature-name
```

### 3. Make Changes

```bash
# Run tests
make test

# Run linter
make lint

# Build
make build
```

### 4. Run Integration Tests

```bash
make test-integration
```

### 5. Commit and Push

Use conventional commits:

```bash
git add .
git commit -m "feat: add new feature"
git push origin feature/your-feature-name
```

Commit types: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`

### 6. Create Pull Request

- PR to `develop` for features
- PR to `main` for releases/hotfixes
- All CI checks must pass
- Self-review required (CODEOWNERS)

## Make Targets

```
make help              # Show all targets
make build             # Build binary
make test              # Run unit tests
make test-integration  # Run integration tests
make lint              # Run linter
make clean             # Clean artifacts
```

## Project Structure

```
├── cmd/opengslb/      # Application entrypoint
├── pkg/               # Library code
├── config/            # Configuration files
├── docs/              # Documentation
├── scripts/           # Development scripts
└── test/integration/  # Integration tests
```

## Testing

See [docs/testing.md](docs/testing.md) for details.

## Code Style

See [CODE_CONVENTIONS.md](CODE_CONVENTIONS.md) for standards.
