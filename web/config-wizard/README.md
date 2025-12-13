# OpenGSLB Configuration Wizard (Web)

A modern, user-friendly web interface for generating OpenGSLB configuration files.

## Features

- **Non-linear navigation** - Click any section in the sidebar to edit
- **Live YAML preview** - See configuration changes in real-time
- **Form validation** - Inline validation with helpful error messages
- **Both modes supported** - Overwatch (DNS authority) and Agent (health reporter)
- **Responsive design** - Works on desktop and tablet
- **Copy/Download** - One-click copy to clipboard or download YAML file

## Getting Started

### Prerequisites

- Node.js 18+
- npm or yarn

### Installation

```bash
cd web/config-wizard
npm install
```

### Development

```bash
npm run dev
```

Opens at http://localhost:5173

### Production Build

```bash
npm run build
```

Outputs to `dist/` directory.

## Technology Stack

- **React 18** - UI framework
- **Vite** - Build tool
- **Tailwind CSS** - Styling
- **js-yaml** - YAML generation
- **Lucide React** - Icons

## Project Structure

```
src/
├── main.jsx        # Entry point
├── App.jsx         # Main application with all components
└── index.css       # Tailwind directives and custom styles
```

## Configuration Sections

### Overwatch Mode
- Operation Mode
- Logging
- Metrics
- Identity
- DNS Server
- Gossip Protocol
- Agent Tokens
- Health Validation
- Stale Handling
- DNSSEC
- Geolocation
- Management API
- Regions
- Domains

### Agent Mode
- Operation Mode
- Logging
- Metrics
- Agent Identity
- Backends
- Gossip Protocol
- Heartbeat
- Predictive Health

## Screenshots

The wizard features:
- Dark theme matching OpenGSLB branding
- Sidebar with section status indicators (● configured, ○ not configured)
- Real-time YAML preview panel
- Contextual help boxes for each section
- Form validation with error messages

## Integration

The generated YAML can be:
1. Copied to clipboard
2. Downloaded as `opengslb-config.yaml`
3. Used directly with OpenGSLB: `opengslb -config /path/to/config.yaml`
