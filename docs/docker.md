# Docker Guide

## Pulling the Image

Once published, pull the image from GitHub Container Registry:
```bash
docker pull ghcr.io/<your-username>/opengslb:latest
```

## Running the Container
```bash
docker run -d \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 9090:9090 \
  ghcr.io/<your-username>/opengslb:latest
```

## Available Tags

- `latest` - Most recent build from main branch
- `main` - Same as latest
- `<commit-sha>` - Specific commit builds

## Making the Package Public

After the first successful build:

1. Go to repository → Packages → opengslb
2. Click Package settings
3. Change visibility to Public