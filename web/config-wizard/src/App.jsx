import React, { useState, useCallback } from 'react'
import {
  Server, Globe, Shield, Database, Activity, Settings,
  FileText, Check, Circle, ChevronRight, Download, Copy,
  AlertCircle, HelpCircle, X, Plus, Trash2, Eye, EyeOff
} from 'lucide-react'
import yaml from 'js-yaml'

// ============================================================================
// CONFIGURATION SCHEMA
// ============================================================================

const defaultConfig = {
  mode: 'overwatch',
  logging: {
    level: 'info',
    format: 'json',
  },
  metrics: {
    enabled: false,
    address: ':9090',
  },
  dns: {
    listen_address: ':53',
    default_ttl: 60,
    return_last_healthy: false,
  },
  overwatch: {
    identity: {
      node_id: '',
      region: '',
    },
    data_dir: '/var/lib/opengslb',
    agent_tokens: {},
    gossip: {
      bind_address: '0.0.0.0:7946',
      encryption_key: '',
      probe_interval: '1s',
      probe_timeout: '500ms',
      gossip_interval: '200ms',
    },
    validation: {
      enabled: true,
      check_interval: '30s',
      check_timeout: '5s',
    },
    stale: {
      threshold: '30s',
      remove_after: '5m',
    },
    dnssec: {
      enabled: true,
      algorithm: 'ECDSAP256SHA256',
    },
    geolocation: {
      database_path: '',
      default_region: '',
      ecs_enabled: true,
      custom_mappings: [],
    },
  },
  api: {
    enabled: true,
    address: '127.0.0.1:8080',
    allowed_networks: ['127.0.0.1/32', '::1/128'],
    trust_proxy_headers: false,
  },
  regions: [],
  domains: [],
  agent: {
    identity: {
      service_token: '',
      region: '',
      cert_path: '/var/lib/opengslb/agent.crt',
      key_path: '/var/lib/opengslb/agent.key',
    },
    backends: [],
    gossip: {
      encryption_key: '',
      overwatch_nodes: [],
    },
    heartbeat: {
      interval: '10s',
      missed_threshold: 3,
    },
    predictive: {
      enabled: false,
      check_interval: '10s',
      cpu: { threshold: 90.0, bleed_duration: '30s' },
      memory: { threshold: 85.0, bleed_duration: '30s' },
      error_rate: { threshold: 10.0, window: '60s', bleed_duration: '30s' },
    },
  },
}

// ============================================================================
// SECTION DEFINITIONS
// ============================================================================

const overwatchSections = [
  { id: 'mode', name: 'Operation Mode', icon: Settings, description: 'Choose between Overwatch or Agent mode' },
  { id: 'logging', name: 'Logging', icon: FileText, description: 'Configure log level and format' },
  { id: 'metrics', name: 'Metrics', icon: Activity, description: 'Prometheus metrics endpoint' },
  { id: 'identity', name: 'Identity', icon: Server, description: 'Node identification' },
  { id: 'dns', name: 'DNS Server', icon: Globe, description: 'DNS server configuration' },
  { id: 'gossip', name: 'Gossip Protocol', icon: Activity, description: 'Cluster communication' },
  { id: 'tokens', name: 'Agent Tokens', icon: Shield, description: 'Authentication tokens for agents' },
  { id: 'validation', name: 'Health Validation', icon: Check, description: 'Health check validation settings' },
  { id: 'stale', name: 'Stale Handling', icon: AlertCircle, description: 'Stale backend management' },
  { id: 'dnssec', name: 'DNSSEC', icon: Shield, description: 'DNS security extensions' },
  { id: 'geolocation', name: 'Geolocation', icon: Globe, description: 'Geographic routing' },
  { id: 'api', name: 'Management API', icon: Server, description: 'REST API configuration' },
  { id: 'regions', name: 'Regions', icon: Database, description: 'Backend server regions' },
  { id: 'domains', name: 'Domains', icon: Globe, description: 'DNS domains to serve' },
]

const agentSections = [
  { id: 'mode', name: 'Operation Mode', icon: Settings, description: 'Choose between Overwatch or Agent mode' },
  { id: 'logging', name: 'Logging', icon: FileText, description: 'Configure log level and format' },
  { id: 'metrics', name: 'Metrics', icon: Activity, description: 'Prometheus metrics endpoint' },
  { id: 'agent_identity', name: 'Agent Identity', icon: Server, description: 'Agent identification' },
  { id: 'agent_backends', name: 'Backends', icon: Database, description: 'Local services to monitor' },
  { id: 'agent_gossip', name: 'Gossip Protocol', icon: Activity, description: 'Overwatch communication' },
  { id: 'agent_heartbeat', name: 'Heartbeat', icon: Activity, description: 'Heartbeat configuration' },
  { id: 'agent_predictive', name: 'Predictive Health', icon: AlertCircle, description: 'Proactive health monitoring' },
]

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

const generateEncryptionKey = () => {
  const array = new Uint8Array(32)
  crypto.getRandomValues(array)
  return btoa(String.fromCharCode.apply(null, array))
}

const generateToken = () => {
  const array = new Uint8Array(24)
  crypto.getRandomValues(array)
  return btoa(String.fromCharCode.apply(null, array))
}

const validateIP = (ip) => {
  // IPv4
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/
  if (ipv4Regex.test(ip)) {
    return ip.split('.').every(octet => parseInt(octet) >= 0 && parseInt(octet) <= 255)
  }
  // IPv6 (simplified)
  const ipv6Regex = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/
  return ipv6Regex.test(ip) || ip === '::' || ip === '::1'
}

const validateCIDR = (cidr) => {
  const parts = cidr.split('/')
  if (parts.length !== 2) return false
  return validateIP(parts[0]) && parseInt(parts[1]) >= 0 && parseInt(parts[1]) <= 128
}

const validateHostPort = (value) => {
  if (!value) return false
  // :port
  if (/^:\d+$/.test(value)) return true
  // ip:port or hostname:port
  if (/^[\w\d\.\-\[\]:]+:\d+$/.test(value)) return true
  return false
}

const validateDuration = (value) => {
  return /^\d+(ms|s|m|h)$/.test(value)
}

// ============================================================================
// UI COMPONENTS
// ============================================================================

const Toggle = ({ checked, onChange, label, description }) => (
  <div className="flex items-center justify-between py-2">
    <div>
      <label className="text-sm font-medium text-gray-200">{label}</label>
      {description && <p className="text-xs text-gray-400 mt-0.5">{description}</p>}
    </div>
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={`toggle-switch ${checked ? 'toggle-switch-checked' : 'toggle-switch-unchecked'}`}
    >
      <span className={`toggle-switch-dot ${checked ? 'toggle-switch-dot-checked' : 'toggle-switch-dot-unchecked'}`} />
    </button>
  </div>
)

const Input = ({ label, value, onChange, placeholder, type = 'text', error, help, required, disabled }) => (
  <div className="mb-4">
    <label className="block text-sm font-medium text-gray-200 mb-1">
      {label}
      {required && <span className="text-danger-500 ml-1">*</span>}
    </label>
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      className={`w-full px-3 py-2 bg-gray-800 border rounded-lg text-gray-100 placeholder-gray-500
        ${error ? 'border-danger-500' : 'border-gray-600'}
        ${disabled ? 'opacity-50 cursor-not-allowed' : 'hover:border-gray-500'}
        focus:border-primary-500 transition-colors`}
    />
    {help && !error && <p className="text-xs text-gray-400 mt-1">{help}</p>}
    {error && <p className="text-xs text-danger-500 mt-1">{error}</p>}
  </div>
)

const Select = ({ label, value, onChange, options, help, required }) => (
  <div className="mb-4">
    <label className="block text-sm font-medium text-gray-200 mb-1">
      {label}
      {required && <span className="text-danger-500 ml-1">*</span>}
    </label>
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full px-3 py-2 bg-gray-800 border border-gray-600 rounded-lg text-gray-100
        hover:border-gray-500 focus:border-primary-500 transition-colors"
    >
      {options.map(opt => (
        <option key={opt.value} value={opt.value}>{opt.label}</option>
      ))}
    </select>
    {help && <p className="text-xs text-gray-400 mt-1">{help}</p>}
  </div>
)

const HelpBox = ({ title, children }) => (
  <div className="bg-gray-800/50 border border-primary-500/30 rounded-lg p-4 mb-6">
    <div className="flex items-start gap-2">
      <HelpCircle className="w-5 h-5 text-primary-400 flex-shrink-0 mt-0.5" />
      <div>
        <h4 className="text-sm font-medium text-primary-400 mb-1">{title}</h4>
        <div className="text-sm text-gray-300 space-y-1">{children}</div>
      </div>
    </div>
  </div>
)

const ArrayInput = ({ label, items, onChange, placeholder, help, validate, itemLabel = 'item' }) => {
  const [newItem, setNewItem] = useState('')
  const [error, setError] = useState('')

  const addItem = () => {
    if (!newItem.trim()) return
    if (validate && !validate(newItem.trim())) {
      setError(`Invalid ${itemLabel} format`)
      return
    }
    if (items.includes(newItem.trim())) {
      setError(`${itemLabel} already exists`)
      return
    }
    onChange([...items, newItem.trim()])
    setNewItem('')
    setError('')
  }

  const removeItem = (index) => {
    onChange(items.filter((_, i) => i !== index))
  }

  return (
    <div className="mb-4">
      <label className="block text-sm font-medium text-gray-200 mb-1">{label}</label>
      <div className="flex gap-2 mb-2">
        <input
          type="text"
          value={newItem}
          onChange={(e) => { setNewItem(e.target.value); setError('') }}
          onKeyPress={(e) => e.key === 'Enter' && (e.preventDefault(), addItem())}
          placeholder={placeholder}
          className="flex-1 px-3 py-2 bg-gray-800 border border-gray-600 rounded-lg text-gray-100
            placeholder-gray-500 hover:border-gray-500 focus:border-primary-500 transition-colors"
        />
        <button
          type="button"
          onClick={addItem}
          className="px-3 py-2 bg-primary-600 hover:bg-primary-500 text-white rounded-lg transition-colors"
        >
          <Plus className="w-5 h-5" />
        </button>
      </div>
      {error && <p className="text-xs text-danger-500 mb-2">{error}</p>}
      {help && <p className="text-xs text-gray-400 mb-2">{help}</p>}
      {items.length > 0 && (
        <div className="space-y-1">
          {items.map((item, index) => (
            <div key={index} className="flex items-center justify-between px-3 py-2 bg-gray-800 rounded-lg">
              <span className="text-sm text-gray-200 font-mono">{item}</span>
              <button
                type="button"
                onClick={() => removeItem(index)}
                className="text-gray-400 hover:text-danger-500 transition-colors"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ============================================================================
// SECTION COMPONENTS
// ============================================================================

const ModeSection = ({ config, setConfig }) => (
  <div className="animate-fadeIn">
    <h2 className="text-2xl font-bold text-gray-100 mb-2">Operation Mode</h2>
    <p className="text-gray-400 mb-6">Choose how OpenGSLB will operate</p>

    <HelpBox title="Understanding Modes">
      <p><strong>Overwatch</strong> is the DNS authority server that answers queries, performs health checks, and makes routing decisions. It's the "brain" of your GSLB setup.</p>
      <p className="mt-2"><strong>Agent</strong> runs on application servers to report local health status to Overwatch. It's lightweight and performs local health checks.</p>
    </HelpBox>

    <div className="grid gap-4 md:grid-cols-2">
      <button
        type="button"
        onClick={() => setConfig({ ...config, mode: 'overwatch' })}
        className={`p-6 rounded-xl border-2 text-left transition-all ${
          config.mode === 'overwatch'
            ? 'border-primary-500 bg-primary-500/10'
            : 'border-gray-700 hover:border-gray-600 bg-gray-800/50'
        }`}
      >
        <div className="flex items-center gap-3 mb-3">
          <div className={`p-2 rounded-lg ${config.mode === 'overwatch' ? 'bg-primary-500' : 'bg-gray-700'}`}>
            <Server className="w-6 h-6 text-white" />
          </div>
          <h3 className="text-lg font-semibold text-gray-100">Overwatch</h3>
        </div>
        <p className="text-sm text-gray-400">DNS authority server with health checking and intelligent routing</p>
        <ul className="mt-3 space-y-1 text-sm text-gray-300">
          <li className="flex items-center gap-2"><Check className="w-4 h-4 text-success-500" /> Serves DNS queries</li>
          <li className="flex items-center gap-2"><Check className="w-4 h-4 text-success-500" /> Health validation</li>
          <li className="flex items-center gap-2"><Check className="w-4 h-4 text-success-500" /> Routing algorithms</li>
        </ul>
      </button>

      <button
        type="button"
        onClick={() => setConfig({ ...config, mode: 'agent' })}
        className={`p-6 rounded-xl border-2 text-left transition-all ${
          config.mode === 'agent'
            ? 'border-primary-500 bg-primary-500/10'
            : 'border-gray-700 hover:border-gray-600 bg-gray-800/50'
        }`}
      >
        <div className="flex items-center gap-3 mb-3">
          <div className={`p-2 rounded-lg ${config.mode === 'agent' ? 'bg-primary-500' : 'bg-gray-700'}`}>
            <Activity className="w-6 h-6 text-white" />
          </div>
          <h3 className="text-lg font-semibold text-gray-100">Agent</h3>
        </div>
        <p className="text-sm text-gray-400">Health reporter running on application servers</p>
        <ul className="mt-3 space-y-1 text-sm text-gray-300">
          <li className="flex items-center gap-2"><Check className="w-4 h-4 text-success-500" /> Local health checks</li>
          <li className="flex items-center gap-2"><Check className="w-4 h-4 text-success-500" /> Reports to Overwatch</li>
          <li className="flex items-center gap-2"><Check className="w-4 h-4 text-success-500" /> Predictive health</li>
        </ul>
      </button>
    </div>
  </div>
)

const LoggingSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      logging: { ...config.logging, [key]: value }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Logging Configuration</h2>
      <p className="text-gray-400 mb-6">Configure how OpenGSLB logs its activity</p>

      <HelpBox title="Logging Options">
        <p><strong>Level:</strong> debug (verbose), info (normal), warn (warnings only), error (errors only)</p>
        <p className="mt-1"><strong>Format:</strong> json for log aggregation (ELK, Splunk), text for human readability</p>
      </HelpBox>

      <Select
        label="Log Level"
        value={config.logging.level}
        onChange={(v) => update('level', v)}
        options={[
          { value: 'debug', label: 'Debug - Very verbose, includes internal state' },
          { value: 'info', label: 'Info - Normal operation messages (recommended)' },
          { value: 'warn', label: 'Warn - Warnings and potential issues' },
          { value: 'error', label: 'Error - Only errors and critical issues' },
        ]}
      />

      <Select
        label="Log Format"
        value={config.logging.format}
        onChange={(v) => update('format', v)}
        options={[
          { value: 'json', label: 'JSON - Structured logging for log aggregation' },
          { value: 'text', label: 'Text - Human-readable for development' },
        ]}
      />
    </div>
  )
}

const MetricsSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      metrics: { ...config.metrics, [key]: value }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Prometheus Metrics</h2>
      <p className="text-gray-400 mb-6">Expose metrics for monitoring</p>

      <HelpBox title="Metrics Endpoint">
        <p>When enabled, metrics are available at <code className="text-primary-400">/metrics</code> and health at <code className="text-primary-400">/health</code></p>
        <p className="mt-1">Useful for monitoring DNS query rates, health check results, and backend status.</p>
      </HelpBox>

      <Toggle
        label="Enable Metrics"
        description="Expose Prometheus-compatible metrics endpoint"
        checked={config.metrics.enabled}
        onChange={(v) => update('enabled', v)}
      />

      {config.metrics.enabled && (
        <Input
          label="Listen Address"
          value={config.metrics.address}
          onChange={(v) => update('address', v)}
          placeholder=":9090"
          help="Format: :port or ip:port"
          error={config.metrics.address && !validateHostPort(config.metrics.address) ? 'Invalid address format' : ''}
        />
      )}
    </div>
  )
}

const DNSSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      dns: { ...config.dns, [key]: value }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">DNS Server Configuration</h2>
      <p className="text-gray-400 mb-6">Configure how OpenGSLB serves DNS queries</p>

      <HelpBox title="DNS Settings">
        <p><strong>Listen Address:</strong> Use :53 for standard DNS (requires root), or higher ports like :5353 for testing</p>
        <p className="mt-1"><strong>TTL:</strong> Lower = faster failover but more queries. 60 seconds is a good default.</p>
        <p className="mt-1"><strong>Limp Mode:</strong> Return last healthy IP when all backends are down (instead of SERVFAIL)</p>
      </HelpBox>

      <Input
        label="Listen Address"
        value={config.dns.listen_address}
        onChange={(v) => update('listen_address', v)}
        placeholder=":53"
        help="Examples: :53, 0.0.0.0:53, [::]:53, 10.0.0.1:53"
        required
      />

      <Input
        label="Default TTL (seconds)"
        type="number"
        value={config.dns.default_ttl}
        onChange={(v) => update('default_ttl', parseInt(v) || 60)}
        placeholder="60"
        help="Time-to-live for DNS responses (1-86400)"
      />

      <Toggle
        label="Return Last Healthy (Limp Mode)"
        description="Return last known healthy IP instead of SERVFAIL when all backends are down"
        checked={config.dns.return_last_healthy}
        onChange={(v) => update('return_last_healthy', v)}
      />
    </div>
  )
}

const IdentitySection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        identity: { ...config.overwatch.identity, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Overwatch Identity</h2>
      <p className="text-gray-400 mb-6">Configure how this node identifies itself</p>

      <HelpBox title="Node Identity">
        <p><strong>Node ID:</strong> Unique identifier for this Overwatch in the cluster. Defaults to hostname.</p>
        <p className="mt-1"><strong>Region:</strong> Geographic region for multi-region deployments (optional).</p>
      </HelpBox>

      <Input
        label="Node ID"
        value={config.overwatch.identity.node_id}
        onChange={(v) => update('node_id', v)}
        placeholder="overwatch-1"
        help="Unique identifier for this node"
      />

      <Input
        label="Region"
        value={config.overwatch.identity.region}
        onChange={(v) => update('region', v)}
        placeholder="us-east-1"
        help="Geographic region (optional)"
      />

      <Input
        label="Data Directory"
        value={config.overwatch.data_dir}
        onChange={(v) => setConfig({ ...config, overwatch: { ...config.overwatch, data_dir: v }})}
        placeholder="/var/lib/opengslb"
        help="Directory for persistent data (DNSSEC keys, database)"
      />
    </div>
  )
}

const GossipSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        gossip: { ...config.overwatch.gossip, [key]: value }
      }
    })
  }

  const generateKey = () => {
    update('encryption_key', generateEncryptionKey())
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Gossip Protocol</h2>
      <p className="text-gray-400 mb-6">Configure cluster communication</p>

      <HelpBox title="Gossip Encryption">
        <p>A 32-byte AES-256 encryption key is <strong>required</strong> for gossip traffic.</p>
        <p className="mt-1">All nodes in the cluster must use the same key.</p>
      </HelpBox>

      <Input
        label="Bind Address"
        value={config.overwatch.gossip.bind_address}
        onChange={(v) => update('bind_address', v)}
        placeholder="0.0.0.0:7946"
        help="Address to listen for agent gossip"
      />

      <div className="mb-4">
        <label className="block text-sm font-medium text-gray-200 mb-1">
          Encryption Key <span className="text-danger-500">*</span>
        </label>
        <div className="flex gap-2">
          <input
            type="text"
            value={config.overwatch.gossip.encryption_key}
            onChange={(e) => update('encryption_key', e.target.value)}
            placeholder="Base64-encoded 32-byte key"
            className="flex-1 px-3 py-2 bg-gray-800 border border-gray-600 rounded-lg text-gray-100
              font-mono text-sm placeholder-gray-500 hover:border-gray-500 focus:border-primary-500"
          />
          <button
            type="button"
            onClick={generateKey}
            className="px-4 py-2 bg-primary-600 hover:bg-primary-500 text-white rounded-lg transition-colors whitespace-nowrap"
          >
            Generate
          </button>
        </div>
        <p className="text-xs text-gray-400 mt-1">Required for secure cluster communication</p>
        {config.overwatch.gossip.encryption_key && (
          <p className="text-xs text-success-500 mt-1">Key generated - save this for other nodes!</p>
        )}
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <Input
          label="Probe Interval"
          value={config.overwatch.gossip.probe_interval}
          onChange={(v) => update('probe_interval', v)}
          placeholder="1s"
          help="Time between failure probes"
        />
        <Input
          label="Probe Timeout"
          value={config.overwatch.gossip.probe_timeout}
          onChange={(v) => update('probe_timeout', v)}
          placeholder="500ms"
          help="Timeout for probes"
        />
        <Input
          label="Gossip Interval"
          value={config.overwatch.gossip.gossip_interval}
          onChange={(v) => update('gossip_interval', v)}
          placeholder="200ms"
          help="Time between gossip broadcasts"
        />
      </div>
    </div>
  )
}

const TokensSection = ({ config, setConfig }) => {
  const [serviceName, setServiceName] = useState('')
  const [token, setToken] = useState('')

  const addToken = () => {
    if (!serviceName.trim()) return
    const newToken = token || generateToken()
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        agent_tokens: { ...config.overwatch.agent_tokens, [serviceName]: newToken }
      }
    })
    setServiceName('')
    setToken('')
  }

  const removeToken = (name) => {
    const { [name]: removed, ...rest } = config.overwatch.agent_tokens
    setConfig({
      ...config,
      overwatch: { ...config.overwatch, agent_tokens: rest }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Agent Authentication Tokens</h2>
      <p className="text-gray-400 mb-6">Pre-shared tokens that agents must provide to register</p>

      <HelpBox title="Service Tokens">
        <p>Each service needs a unique token. Agents must know this token to connect.</p>
        <p className="mt-1">Tokens should be at least 16 characters for security.</p>
      </HelpBox>

      <div className="grid gap-4 md:grid-cols-2 mb-4">
        <Input
          label="Service Name"
          value={serviceName}
          onChange={setServiceName}
          placeholder="e.g., webapp, api"
        />
        <div>
          <label className="block text-sm font-medium text-gray-200 mb-1">Token (auto-generated if empty)</label>
          <div className="flex gap-2">
            <input
              type="text"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="Leave empty to auto-generate"
              className="flex-1 px-3 py-2 bg-gray-800 border border-gray-600 rounded-lg text-gray-100
                placeholder-gray-500 hover:border-gray-500 focus:border-primary-500"
            />
            <button
              type="button"
              onClick={addToken}
              disabled={!serviceName.trim()}
              className="px-4 py-2 bg-primary-600 hover:bg-primary-500 disabled:bg-gray-600
                disabled:cursor-not-allowed text-white rounded-lg transition-colors"
            >
              Add
            </button>
          </div>
        </div>
      </div>

      {Object.keys(config.overwatch.agent_tokens).length > 0 && (
        <div className="space-y-2">
          <h4 className="text-sm font-medium text-gray-300">Configured Tokens:</h4>
          {Object.entries(config.overwatch.agent_tokens).map(([name, tkn]) => (
            <div key={name} className="flex items-center justify-between px-4 py-3 bg-gray-800 rounded-lg">
              <div>
                <span className="font-medium text-gray-200">{name}</span>
                <span className="ml-3 text-sm text-gray-400 font-mono">{tkn.substring(0, 20)}...</span>
              </div>
              <button
                type="button"
                onClick={() => removeToken(name)}
                className="text-gray-400 hover:text-danger-500 transition-colors"
              >
                <Trash2 className="w-4 h-4" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const ValidationSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        validation: { ...config.overwatch.validation, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Health Validation</h2>
      <p className="text-gray-400 mb-6">Configure how Overwatch validates health claims</p>

      <HelpBox title="Why Validate?">
        <p>Overwatch can independently verify agent health claims for security.</p>
        <p className="mt-1">When enabled, Overwatch performs its own checks and its decision always wins.</p>
      </HelpBox>

      <Toggle
        label="Enable Validation"
        description="Independently verify agent health claims (recommended)"
        checked={config.overwatch.validation.enabled}
        onChange={(v) => update('enabled', v)}
      />

      {config.overwatch.validation.enabled && (
        <div className="grid gap-4 md:grid-cols-2">
          <Input
            label="Check Interval"
            value={config.overwatch.validation.check_interval}
            onChange={(v) => update('check_interval', v)}
            placeholder="30s"
            help="How often to validate (e.g., 30s, 1m)"
          />
          <Input
            label="Check Timeout"
            value={config.overwatch.validation.check_timeout}
            onChange={(v) => update('check_timeout', v)}
            placeholder="5s"
            help="Timeout for validation checks"
          />
        </div>
      )}
    </div>
  )
}

const StaleSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        stale: { ...config.overwatch.stale, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Stale Backend Handling</h2>
      <p className="text-gray-400 mb-6">Configure how to handle backends that stop reporting</p>

      <HelpBox title="Stale Detection">
        <p><strong>Threshold:</strong> Time without heartbeat before marking as "stale" (deprioritized)</p>
        <p className="mt-1"><strong>Remove After:</strong> Time after which stale backends are completely removed</p>
      </HelpBox>

      <div className="grid gap-4 md:grid-cols-2">
        <Input
          label="Stale Threshold"
          value={config.overwatch.stale.threshold}
          onChange={(v) => update('threshold', v)}
          placeholder="30s"
          help="Time without heartbeat to mark stale"
        />
        <Input
          label="Remove After"
          value={config.overwatch.stale.remove_after}
          onChange={(v) => update('remove_after', v)}
          placeholder="5m"
          help="Time to remove stale backends"
        />
      </div>
    </div>
  )
}

const DNSSECSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        dnssec: { ...config.overwatch.dnssec, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">DNSSEC Configuration</h2>
      <p className="text-gray-400 mb-6">DNS Security Extensions for cryptographic signing</p>

      <HelpBox title="DNSSEC">
        <p>DNSSEC prevents DNS spoofing, cache poisoning, and MITM attacks.</p>
        <p className="mt-1"><strong>Recommended:</strong> Keep enabled for production deployments.</p>
      </HelpBox>

      <Toggle
        label="Enable DNSSEC"
        description="Cryptographically sign DNS responses (highly recommended)"
        checked={config.overwatch.dnssec.enabled}
        onChange={(v) => update('enabled', v)}
      />

      {config.overwatch.dnssec.enabled && (
        <Select
          label="Algorithm"
          value={config.overwatch.dnssec.algorithm}
          onChange={(v) => update('algorithm', v)}
          options={[
            { value: 'ECDSAP256SHA256', label: 'ECDSAP256SHA256 - Modern, fast (recommended)' },
            { value: 'ECDSAP384SHA384', label: 'ECDSAP384SHA384 - Stronger, slightly slower' },
            { value: 'RSASHA256', label: 'RSASHA256 - Legacy compatibility' },
            { value: 'ED25519', label: 'ED25519 - Newest, very fast' },
          ]}
        />
      )}

      {!config.overwatch.dnssec.enabled && (
        <div className="p-4 bg-warning-500/10 border border-warning-500/30 rounded-lg">
          <div className="flex items-start gap-2">
            <AlertCircle className="w-5 h-5 text-warning-500 flex-shrink-0 mt-0.5" />
            <div>
              <p className="text-sm font-medium text-warning-400">Security Warning</p>
              <p className="text-sm text-gray-300 mt-1">
                Disabling DNSSEC reduces security. Only disable for testing.
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

const GeolocationSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      overwatch: {
        ...config.overwatch,
        geolocation: { ...config.overwatch.geolocation, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Geolocation Configuration</h2>
      <p className="text-gray-400 mb-6">Route users to the nearest datacenter</p>

      <HelpBox title="Geographic Routing">
        <p>Requires a MaxMind GeoLite2-Country database (free with registration).</p>
        <p className="mt-1">Download from: <a href="https://dev.maxmind.com/geoip/geolite2-free-geolocation-data" className="text-primary-400 hover:underline" target="_blank" rel="noopener">dev.maxmind.com</a></p>
      </HelpBox>

      <Input
        label="Database Path"
        value={config.overwatch.geolocation.database_path}
        onChange={(v) => update('database_path', v)}
        placeholder="/path/to/GeoLite2-Country.mmdb"
        help="Path to MaxMind GeoLite2-Country database (leave empty to disable)"
      />

      {config.overwatch.geolocation.database_path && (
        <>
          <Input
            label="Default Region"
            value={config.overwatch.geolocation.default_region}
            onChange={(v) => update('default_region', v)}
            placeholder="us-east"
            help="Fallback region for unknown IPs"
            required
          />

          <Toggle
            label="Enable ECS"
            description="EDNS Client Subnet for more accurate geolocation"
            checked={config.overwatch.geolocation.ecs_enabled}
            onChange={(v) => update('ecs_enabled', v)}
          />
        </>
      )}
    </div>
  )
}

const APISection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      api: { ...config.api, [key]: value }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Management API</h2>
      <p className="text-gray-400 mb-6">REST API for runtime management</p>

      <HelpBox title="API Security">
        <p>The API provides health status, backend management, and DNSSEC controls.</p>
        <p className="mt-1">Default: localhost only. Use <code className="text-primary-400">allowed_networks</code> to restrict access.</p>
      </HelpBox>

      <Toggle
        label="Enable API"
        description="Enable the management REST API"
        checked={config.api.enabled}
        onChange={(v) => update('enabled', v)}
      />

      {config.api.enabled && (
        <>
          <Input
            label="Listen Address"
            value={config.api.address}
            onChange={(v) => update('address', v)}
            placeholder="127.0.0.1:8080"
            help="Address and port for the API server"
          />

          <ArrayInput
            label="Allowed Networks"
            items={config.api.allowed_networks}
            onChange={(v) => update('allowed_networks', v)}
            placeholder="10.0.0.0/8"
            help="CIDR networks allowed to access the API"
            validate={validateCIDR}
            itemLabel="CIDR"
          />

          <Toggle
            label="Trust Proxy Headers"
            description="Trust X-Forwarded-For headers (only with trusted proxy)"
            checked={config.api.trust_proxy_headers}
            onChange={(v) => update('trust_proxy_headers', v)}
          />
        </>
      )}
    </div>
  )
}

// ============================================================================
// REGIONS SECTION
// ============================================================================

const RegionsSection = ({ config, setConfig }) => {
  const [editingRegion, setEditingRegion] = useState(null)

  const addRegion = () => {
    const newRegion = {
      name: '',
      countries: [],
      continents: [],
      servers: [{ address: '', port: 80, weight: 100, host: '' }],
      health_check: {
        type: 'http',
        interval: '30s',
        timeout: '5s',
        path: '/health',
        host: '',
        failure_threshold: 3,
        success_threshold: 2,
      }
    }
    setEditingRegion(newRegion)
  }

  const saveRegion = (region) => {
    if (editingRegion.name && config.regions.some(r => r.name === editingRegion.name && r !== region)) {
      // Updating existing
      setConfig({
        ...config,
        regions: config.regions.map(r => r.name === region.name ? editingRegion : r)
      })
    } else {
      // Adding new
      setConfig({
        ...config,
        regions: [...config.regions.filter(r => r.name !== editingRegion.name), editingRegion]
      })
    }
    setEditingRegion(null)
  }

  const removeRegion = (name) => {
    setConfig({
      ...config,
      regions: config.regions.filter(r => r.name !== name)
    })
  }

  if (editingRegion) {
    return <RegionEditor region={editingRegion} setRegion={setEditingRegion} onSave={saveRegion} onCancel={() => setEditingRegion(null)} />
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Regions Configuration</h2>
      <p className="text-gray-400 mb-6">Define backend server groups by location</p>

      <HelpBox title="Regions">
        <p>Regions group backend servers, typically by geographic location or datacenter.</p>
        <p className="mt-1">Each region needs servers and health check configuration.</p>
      </HelpBox>

      <button
        type="button"
        onClick={addRegion}
        className="w-full p-4 border-2 border-dashed border-gray-600 rounded-lg hover:border-primary-500
          hover:bg-primary-500/5 transition-colors mb-4"
      >
        <div className="flex items-center justify-center gap-2 text-gray-400 hover:text-primary-400">
          <Plus className="w-5 h-5" />
          <span>Add Region</span>
        </div>
      </button>

      {config.regions.length > 0 && (
        <div className="space-y-3">
          {config.regions.map((region) => (
            <div key={region.name} className="p-4 bg-gray-800 rounded-lg">
              <div className="flex items-center justify-between">
                <div>
                  <h4 className="font-medium text-gray-100">{region.name}</h4>
                  <p className="text-sm text-gray-400">
                    {region.servers.length} server(s) • {region.health_check.type.toUpperCase()} health check
                  </p>
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setEditingRegion({ ...region })}
                    className="px-3 py-1 text-sm bg-gray-700 hover:bg-gray-600 text-gray-200 rounded transition-colors"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => removeRegion(region.name)}
                    className="px-3 py-1 text-sm bg-danger-600/20 hover:bg-danger-600/40 text-danger-400 rounded transition-colors"
                  >
                    Remove
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const RegionEditor = ({ region, setRegion, onSave, onCancel }) => {
  const update = (key, value) => setRegion({ ...region, [key]: value })
  const updateHealthCheck = (key, value) => setRegion({ ...region, health_check: { ...region.health_check, [key]: value } })

  const addServer = () => {
    setRegion({
      ...region,
      servers: [...region.servers, { address: '', port: 80, weight: 100, host: '' }]
    })
  }

  const updateServer = (index, key, value) => {
    const servers = [...region.servers]
    servers[index] = { ...servers[index], [key]: value }
    setRegion({ ...region, servers })
  }

  const removeServer = (index) => {
    setRegion({
      ...region,
      servers: region.servers.filter((_, i) => i !== index)
    })
  }

  return (
    <div className="animate-fadeIn">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-100">
          {region.name ? `Edit Region: ${region.name}` : 'New Region'}
        </h2>
        <button onClick={onCancel} className="text-gray-400 hover:text-gray-200">
          <X className="w-6 h-6" />
        </button>
      </div>

      <Input
        label="Region Name"
        value={region.name}
        onChange={(v) => update('name', v)}
        placeholder="us-east-1"
        required
      />

      <h3 className="text-lg font-semibold text-gray-200 mt-6 mb-4">Servers</h3>

      {region.servers.map((server, index) => (
        <div key={index} className="p-4 bg-gray-800 rounded-lg mb-3">
          <div className="flex justify-between items-start mb-3">
            <span className="text-sm font-medium text-gray-300">Server {index + 1}</span>
            {region.servers.length > 1 && (
              <button onClick={() => removeServer(index)} className="text-gray-400 hover:text-danger-500">
                <Trash2 className="w-4 h-4" />
              </button>
            )}
          </div>
          <div className="grid gap-4 md:grid-cols-4">
            <Input
              label="Address"
              value={server.address}
              onChange={(v) => updateServer(index, 'address', v)}
              placeholder="10.0.1.10"
              required
            />
            <Input
              label="Port"
              type="number"
              value={server.port}
              onChange={(v) => updateServer(index, 'port', parseInt(v) || 80)}
              placeholder="80"
            />
            <Input
              label="Weight"
              type="number"
              value={server.weight}
              onChange={(v) => updateServer(index, 'weight', parseInt(v) || 100)}
              placeholder="100"
              help="1-1000, 0=disabled"
            />
            <Input
              label="Host (TLS SNI)"
              value={server.host}
              onChange={(v) => updateServer(index, 'host', v)}
              placeholder="Optional"
            />
          </div>
        </div>
      ))}

      <button
        type="button"
        onClick={addServer}
        className="w-full p-3 border-2 border-dashed border-gray-600 rounded-lg hover:border-primary-500
          text-gray-400 hover:text-primary-400 transition-colors mb-6"
      >
        <div className="flex items-center justify-center gap-2">
          <Plus className="w-4 h-4" />
          <span>Add Server</span>
        </div>
      </button>

      <h3 className="text-lg font-semibold text-gray-200 mt-6 mb-4">Health Check</h3>

      <div className="grid gap-4 md:grid-cols-2">
        <Select
          label="Type"
          value={region.health_check.type}
          onChange={(v) => updateHealthCheck('type', v)}
          options={[
            { value: 'http', label: 'HTTP - GET request, expects 2xx' },
            { value: 'https', label: 'HTTPS - Same as HTTP with TLS' },
            { value: 'tcp', label: 'TCP - Connection check only' },
          ]}
        />
        <Input
          label="Interval"
          value={region.health_check.interval}
          onChange={(v) => updateHealthCheck('interval', v)}
          placeholder="30s"
        />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Input
          label="Timeout"
          value={region.health_check.timeout}
          onChange={(v) => updateHealthCheck('timeout', v)}
          placeholder="5s"
        />
        {(region.health_check.type === 'http' || region.health_check.type === 'https') && (
          <Input
            label="Path"
            value={region.health_check.path}
            onChange={(v) => updateHealthCheck('path', v)}
            placeholder="/health"
          />
        )}
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Input
          label="Failure Threshold"
          type="number"
          value={region.health_check.failure_threshold}
          onChange={(v) => updateHealthCheck('failure_threshold', parseInt(v) || 3)}
          help="Consecutive failures to mark unhealthy"
        />
        <Input
          label="Success Threshold"
          type="number"
          value={region.health_check.success_threshold}
          onChange={(v) => updateHealthCheck('success_threshold', parseInt(v) || 2)}
          help="Consecutive successes to mark healthy"
        />
      </div>

      <div className="flex gap-3 mt-6">
        <button
          type="button"
          onClick={() => onSave(region)}
          disabled={!region.name || !region.servers.some(s => s.address)}
          className="flex-1 py-3 bg-primary-600 hover:bg-primary-500 disabled:bg-gray-600
            disabled:cursor-not-allowed text-white rounded-lg transition-colors font-medium"
        >
          Save Region
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="px-6 py-3 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-lg transition-colors"
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

// ============================================================================
// DOMAINS SECTION
// ============================================================================

const DomainsSection = ({ config, setConfig }) => {
  const [editingDomain, setEditingDomain] = useState(null)

  const addDomain = () => {
    setEditingDomain({
      name: '',
      routing_algorithm: 'round-robin',
      regions: [],
      ttl: 0,
    })
  }

  const saveDomain = (domain) => {
    setConfig({
      ...config,
      domains: [...config.domains.filter(d => d.name !== editingDomain.name), editingDomain]
    })
    setEditingDomain(null)
  }

  const removeDomain = (name) => {
    setConfig({
      ...config,
      domains: config.domains.filter(d => d.name !== name)
    })
  }

  if (editingDomain) {
    return <DomainEditor domain={editingDomain} setDomain={setEditingDomain} regions={config.regions} onSave={saveDomain} onCancel={() => setEditingDomain(null)} />
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Domains Configuration</h2>
      <p className="text-gray-400 mb-6">Define DNS domains that OpenGSLB will serve</p>

      <HelpBox title="Domains">
        <p>Each domain needs a routing algorithm and at least one region.</p>
        <p className="mt-1"><strong>Tip:</strong> Configure regions first before adding domains.</p>
      </HelpBox>

      {config.regions.length === 0 && (
        <div className="p-4 bg-warning-500/10 border border-warning-500/30 rounded-lg mb-4">
          <div className="flex items-start gap-2">
            <AlertCircle className="w-5 h-5 text-warning-500 flex-shrink-0 mt-0.5" />
            <p className="text-sm text-gray-300">Configure at least one region before adding domains.</p>
          </div>
        </div>
      )}

      <button
        type="button"
        onClick={addDomain}
        disabled={config.regions.length === 0}
        className="w-full p-4 border-2 border-dashed border-gray-600 rounded-lg hover:border-primary-500
          hover:bg-primary-500/5 transition-colors mb-4 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        <div className="flex items-center justify-center gap-2 text-gray-400 hover:text-primary-400">
          <Plus className="w-5 h-5" />
          <span>Add Domain</span>
        </div>
      </button>

      {config.domains.length > 0 && (
        <div className="space-y-3">
          {config.domains.map((domain) => (
            <div key={domain.name} className="p-4 bg-gray-800 rounded-lg">
              <div className="flex items-center justify-between">
                <div>
                  <h4 className="font-medium text-gray-100">{domain.name}</h4>
                  <p className="text-sm text-gray-400">
                    {domain.routing_algorithm} • {domain.regions.join(', ')}
                  </p>
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setEditingDomain({ ...domain })}
                    className="px-3 py-1 text-sm bg-gray-700 hover:bg-gray-600 text-gray-200 rounded transition-colors"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => removeDomain(domain.name)}
                    className="px-3 py-1 text-sm bg-danger-600/20 hover:bg-danger-600/40 text-danger-400 rounded transition-colors"
                  >
                    Remove
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const DomainEditor = ({ domain, setDomain, regions, onSave, onCancel }) => {
  const update = (key, value) => setDomain({ ...domain, [key]: value })

  const toggleRegion = (regionName) => {
    if (domain.regions.includes(regionName)) {
      update('regions', domain.regions.filter(r => r !== regionName))
    } else {
      update('regions', [...domain.regions, regionName])
    }
  }

  return (
    <div className="animate-fadeIn">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-100">
          {domain.name ? `Edit Domain: ${domain.name}` : 'New Domain'}
        </h2>
        <button onClick={onCancel} className="text-gray-400 hover:text-gray-200">
          <X className="w-6 h-6" />
        </button>
      </div>

      <Input
        label="Domain Name (FQDN)"
        value={domain.name}
        onChange={(v) => update('name', v)}
        placeholder="api.example.com"
        required
      />

      <Select
        label="Routing Algorithm"
        value={domain.routing_algorithm}
        onChange={(v) => update('routing_algorithm', v)}
        options={[
          { value: 'round-robin', label: 'Round Robin - Equal distribution' },
          { value: 'weighted', label: 'Weighted - Proportional to server weights' },
          { value: 'failover', label: 'Failover - Highest priority healthy server' },
          { value: 'geolocation', label: 'Geolocation - Based on client location' },
          { value: 'latency', label: 'Latency - Lowest measured latency' },
        ]}
      />

      <Input
        label="TTL (seconds)"
        type="number"
        value={domain.ttl}
        onChange={(v) => update('ttl', parseInt(v) || 0)}
        placeholder="0"
        help="0 = use default TTL"
      />

      <div className="mb-4">
        <label className="block text-sm font-medium text-gray-200 mb-2">
          Regions <span className="text-danger-500">*</span>
        </label>
        <div className="space-y-2">
          {regions.map((region) => (
            <button
              key={region.name}
              type="button"
              onClick={() => toggleRegion(region.name)}
              className={`w-full p-3 rounded-lg border text-left transition-colors ${
                domain.regions.includes(region.name)
                  ? 'border-primary-500 bg-primary-500/10'
                  : 'border-gray-600 hover:border-gray-500 bg-gray-800'
              }`}
            >
              <div className="flex items-center gap-3">
                <div className={`w-5 h-5 rounded-full border-2 flex items-center justify-center ${
                  domain.regions.includes(region.name) ? 'border-primary-500 bg-primary-500' : 'border-gray-500'
                }`}>
                  {domain.regions.includes(region.name) && <Check className="w-3 h-3 text-white" />}
                </div>
                <div>
                  <span className="font-medium text-gray-200">{region.name}</span>
                  <span className="ml-2 text-sm text-gray-400">{region.servers.length} server(s)</span>
                </div>
              </div>
            </button>
          ))}
        </div>
      </div>

      <div className="flex gap-3 mt-6">
        <button
          type="button"
          onClick={() => onSave(domain)}
          disabled={!domain.name || domain.regions.length === 0}
          className="flex-1 py-3 bg-primary-600 hover:bg-primary-500 disabled:bg-gray-600
            disabled:cursor-not-allowed text-white rounded-lg transition-colors font-medium"
        >
          Save Domain
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="px-6 py-3 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-lg transition-colors"
        >
          Cancel
        </button>
      </div>
    </div>
  )
}

// ============================================================================
// AGENT SECTIONS
// ============================================================================

const AgentIdentitySection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      agent: {
        ...config.agent,
        identity: { ...config.agent.identity, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Agent Identity</h2>
      <p className="text-gray-400 mb-6">Configure how this agent identifies itself</p>

      <HelpBox title="Agent Authentication">
        <p><strong>Service Token:</strong> Must match the token configured in Overwatch.</p>
        <p className="mt-1"><strong>Region:</strong> Must match a region configured in Overwatch.</p>
      </HelpBox>

      <Input
        label="Service Token"
        value={config.agent.identity.service_token}
        onChange={(v) => update('service_token', v)}
        placeholder="Your service token (min 16 chars)"
        required
        error={config.agent.identity.service_token && config.agent.identity.service_token.length < 16 ? 'Token must be at least 16 characters' : ''}
      />

      <Input
        label="Region"
        value={config.agent.identity.region}
        onChange={(v) => update('region', v)}
        placeholder="us-east-1"
        required
      />

      <Input
        label="Certificate Path"
        value={config.agent.identity.cert_path}
        onChange={(v) => update('cert_path', v)}
        placeholder="/var/lib/opengslb/agent.crt"
        help="Path to agent certificate for mTLS"
      />

      <Input
        label="Private Key Path"
        value={config.agent.identity.key_path}
        onChange={(v) => update('key_path', v)}
        placeholder="/var/lib/opengslb/agent.key"
        help="Path to agent private key"
      />
    </div>
  )
}

const AgentBackendsSection = ({ config, setConfig }) => {
  const [editingBackend, setEditingBackend] = useState(null)

  const addBackend = () => {
    setEditingBackend({
      service: '',
      address: '127.0.0.1',
      port: 8080,
      weight: 100,
      health_check: {
        type: 'http',
        interval: '30s',
        timeout: '5s',
        path: '/health',
        host: '',
        failure_threshold: 3,
        success_threshold: 2,
      }
    })
  }

  const saveBackend = () => {
    setConfig({
      ...config,
      agent: {
        ...config.agent,
        backends: [...config.agent.backends.filter(b => b.service !== editingBackend.service), editingBackend]
      }
    })
    setEditingBackend(null)
  }

  const removeBackend = (service) => {
    setConfig({
      ...config,
      agent: {
        ...config.agent,
        backends: config.agent.backends.filter(b => b.service !== service)
      }
    })
  }

  if (editingBackend) {
    return (
      <div className="animate-fadeIn">
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-2xl font-bold text-gray-100">
            {editingBackend.service ? `Edit Backend: ${editingBackend.service}` : 'New Backend'}
          </h2>
          <button onClick={() => setEditingBackend(null)} className="text-gray-400 hover:text-gray-200">
            <X className="w-6 h-6" />
          </button>
        </div>

        <Input
          label="Service Name"
          value={editingBackend.service}
          onChange={(v) => setEditingBackend({ ...editingBackend, service: v })}
          placeholder="webapp"
          required
        />

        <div className="grid gap-4 md:grid-cols-3">
          <Input
            label="Address"
            value={editingBackend.address}
            onChange={(v) => setEditingBackend({ ...editingBackend, address: v })}
            placeholder="127.0.0.1"
          />
          <Input
            label="Port"
            type="number"
            value={editingBackend.port}
            onChange={(v) => setEditingBackend({ ...editingBackend, port: parseInt(v) || 8080 })}
          />
          <Input
            label="Weight"
            type="number"
            value={editingBackend.weight}
            onChange={(v) => setEditingBackend({ ...editingBackend, weight: parseInt(v) || 100 })}
          />
        </div>

        <h3 className="text-lg font-semibold text-gray-200 mt-6 mb-4">Health Check</h3>

        <Select
          label="Type"
          value={editingBackend.health_check.type}
          onChange={(v) => setEditingBackend({ ...editingBackend, health_check: { ...editingBackend.health_check, type: v } })}
          options={[
            { value: 'http', label: 'HTTP' },
            { value: 'https', label: 'HTTPS' },
            { value: 'tcp', label: 'TCP' },
          ]}
        />

        <div className="grid gap-4 md:grid-cols-2">
          <Input
            label="Interval"
            value={editingBackend.health_check.interval}
            onChange={(v) => setEditingBackend({ ...editingBackend, health_check: { ...editingBackend.health_check, interval: v } })}
            placeholder="30s"
          />
          <Input
            label="Path"
            value={editingBackend.health_check.path}
            onChange={(v) => setEditingBackend({ ...editingBackend, health_check: { ...editingBackend.health_check, path: v } })}
            placeholder="/health"
          />
        </div>

        <div className="flex gap-3 mt-6">
          <button
            type="button"
            onClick={saveBackend}
            disabled={!editingBackend.service}
            className="flex-1 py-3 bg-primary-600 hover:bg-primary-500 disabled:bg-gray-600 text-white rounded-lg font-medium"
          >
            Save Backend
          </button>
          <button
            type="button"
            onClick={() => setEditingBackend(null)}
            className="px-6 py-3 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded-lg"
          >
            Cancel
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Agent Backends</h2>
      <p className="text-gray-400 mb-6">Local services this agent monitors</p>

      <HelpBox title="Backends">
        <p>Configure the local services that this agent should health check and report to Overwatch.</p>
      </HelpBox>

      <button
        type="button"
        onClick={addBackend}
        className="w-full p-4 border-2 border-dashed border-gray-600 rounded-lg hover:border-primary-500
          hover:bg-primary-500/5 transition-colors mb-4"
      >
        <div className="flex items-center justify-center gap-2 text-gray-400 hover:text-primary-400">
          <Plus className="w-5 h-5" />
          <span>Add Backend</span>
        </div>
      </button>

      {config.agent.backends.length > 0 && (
        <div className="space-y-3">
          {config.agent.backends.map((backend) => (
            <div key={backend.service} className="p-4 bg-gray-800 rounded-lg">
              <div className="flex items-center justify-between">
                <div>
                  <h4 className="font-medium text-gray-100">{backend.service}</h4>
                  <p className="text-sm text-gray-400">{backend.address}:{backend.port}</p>
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setEditingBackend({ ...backend })}
                    className="px-3 py-1 text-sm bg-gray-700 hover:bg-gray-600 text-gray-200 rounded"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => removeBackend(backend.service)}
                    className="px-3 py-1 text-sm bg-danger-600/20 hover:bg-danger-600/40 text-danger-400 rounded"
                  >
                    Remove
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

const AgentGossipSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      agent: {
        ...config.agent,
        gossip: { ...config.agent.gossip, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Gossip Protocol</h2>
      <p className="text-gray-400 mb-6">Configure communication with Overwatch</p>

      <HelpBox title="Gossip Configuration">
        <p>The encryption key must match the Overwatch configuration.</p>
        <p className="mt-1">Add at least one Overwatch node address.</p>
      </HelpBox>

      <div className="mb-4">
        <label className="block text-sm font-medium text-gray-200 mb-1">
          Encryption Key <span className="text-danger-500">*</span>
        </label>
        <input
          type="text"
          value={config.agent.gossip.encryption_key}
          onChange={(e) => update('encryption_key', e.target.value)}
          placeholder="Same key as Overwatch"
          className="w-full px-3 py-2 bg-gray-800 border border-gray-600 rounded-lg text-gray-100
            font-mono text-sm placeholder-gray-500"
        />
        <p className="text-xs text-gray-400 mt-1">Must match the key configured in Overwatch</p>
      </div>

      <ArrayInput
        label="Overwatch Nodes"
        items={config.agent.gossip.overwatch_nodes}
        onChange={(v) => update('overwatch_nodes', v)}
        placeholder="overwatch-1:7946"
        help="Format: hostname:port or ip:port"
        validate={validateHostPort}
        itemLabel="address"
      />
    </div>
  )
}

const AgentHeartbeatSection = ({ config, setConfig }) => {
  const update = (key, value) => {
    setConfig({
      ...config,
      agent: {
        ...config.agent,
        heartbeat: { ...config.agent.heartbeat, [key]: value }
      }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Heartbeat Configuration</h2>
      <p className="text-gray-400 mb-6">Configure heartbeat timing</p>

      <HelpBox title="Heartbeat">
        <p>Heartbeats let Overwatch know this agent is alive.</p>
        <p className="mt-1">Missed heartbeats = interval × threshold before deregistration.</p>
      </HelpBox>

      <div className="grid gap-4 md:grid-cols-2">
        <Input
          label="Interval"
          value={config.agent.heartbeat.interval}
          onChange={(v) => update('interval', v)}
          placeholder="10s"
          help="How often to send heartbeats"
        />
        <Input
          label="Missed Threshold"
          type="number"
          value={config.agent.heartbeat.missed_threshold}
          onChange={(v) => update('missed_threshold', parseInt(v) || 3)}
          help="Missed heartbeats before deregistration"
        />
      </div>
    </div>
  )
}

const AgentPredictiveSection = ({ config, setConfig }) => {
  const update = (path, value) => {
    const keys = path.split('.')
    let newPredictive = { ...config.agent.predictive }

    if (keys.length === 1) {
      newPredictive[keys[0]] = value
    } else if (keys.length === 2) {
      newPredictive[keys[0]] = { ...newPredictive[keys[0]], [keys[1]]: value }
    }

    setConfig({
      ...config,
      agent: { ...config.agent, predictive: newPredictive }
    })
  }

  return (
    <div className="animate-fadeIn">
      <h2 className="text-2xl font-bold text-gray-100 mb-2">Predictive Health</h2>
      <p className="text-gray-400 mb-6">Proactively drain traffic before servers become unhealthy</p>

      <HelpBox title="Predictive Health">
        <p>Monitor CPU, memory, and error rates to drain traffic before failures occur.</p>
        <p className="mt-1"><strong>Bleed Duration:</strong> Time to gradually reduce traffic to zero.</p>
      </HelpBox>

      <Toggle
        label="Enable Predictive Health"
        description="Proactively monitor system resources"
        checked={config.agent.predictive.enabled}
        onChange={(v) => update('enabled', v)}
      />

      {config.agent.predictive.enabled && (
        <>
          <Input
            label="Check Interval"
            value={config.agent.predictive.check_interval}
            onChange={(v) => update('check_interval', v)}
            placeholder="10s"
          />

          <h3 className="text-lg font-semibold text-gray-200 mt-6 mb-4">CPU Monitoring</h3>
          <div className="grid gap-4 md:grid-cols-2">
            <Input
              label="Threshold (%)"
              type="number"
              value={config.agent.predictive.cpu.threshold}
              onChange={(v) => update('cpu.threshold', parseFloat(v) || 90)}
            />
            <Input
              label="Bleed Duration"
              value={config.agent.predictive.cpu.bleed_duration}
              onChange={(v) => update('cpu.bleed_duration', v)}
              placeholder="30s"
            />
          </div>

          <h3 className="text-lg font-semibold text-gray-200 mt-6 mb-4">Memory Monitoring</h3>
          <div className="grid gap-4 md:grid-cols-2">
            <Input
              label="Threshold (%)"
              type="number"
              value={config.agent.predictive.memory.threshold}
              onChange={(v) => update('memory.threshold', parseFloat(v) || 85)}
            />
            <Input
              label="Bleed Duration"
              value={config.agent.predictive.memory.bleed_duration}
              onChange={(v) => update('memory.bleed_duration', v)}
              placeholder="30s"
            />
          </div>

          <h3 className="text-lg font-semibold text-gray-200 mt-6 mb-4">Error Rate Monitoring</h3>
          <div className="grid gap-4 md:grid-cols-3">
            <Input
              label="Threshold (errors/min)"
              type="number"
              value={config.agent.predictive.error_rate.threshold}
              onChange={(v) => update('error_rate.threshold', parseFloat(v) || 10)}
            />
            <Input
              label="Window"
              value={config.agent.predictive.error_rate.window}
              onChange={(v) => update('error_rate.window', v)}
              placeholder="60s"
            />
            <Input
              label="Bleed Duration"
              value={config.agent.predictive.error_rate.bleed_duration}
              onChange={(v) => update('error_rate.bleed_duration', v)}
              placeholder="30s"
            />
          </div>
        </>
      )}
    </div>
  )
}

// ============================================================================
// YAML PREVIEW
// ============================================================================

const YAMLPreview = ({ config }) => {
  const [copied, setCopied] = useState(false)

  const generateYAML = () => {
    const output = { mode: config.mode }

    // Logging
    output.logging = config.logging

    // Metrics
    output.metrics = config.metrics

    if (config.mode === 'overwatch') {
      // DNS
      output.dns = config.dns

      // Overwatch section
      output.overwatch = {
        identity: config.overwatch.identity,
        data_dir: config.overwatch.data_dir,
      }

      if (Object.keys(config.overwatch.agent_tokens).length > 0) {
        output.overwatch.agent_tokens = config.overwatch.agent_tokens
      }

      output.overwatch.gossip = config.overwatch.gossip
      output.overwatch.validation = config.overwatch.validation
      output.overwatch.stale = config.overwatch.stale
      output.overwatch.dnssec = config.overwatch.dnssec

      if (config.overwatch.geolocation.database_path) {
        output.overwatch.geolocation = config.overwatch.geolocation
      }

      // API
      output.api = config.api

      // Regions
      if (config.regions.length > 0) {
        output.regions = config.regions
      }

      // Domains
      if (config.domains.length > 0) {
        output.domains = config.domains
      }
    } else {
      // Agent mode
      output.agent = config.agent
    }

    return yaml.dump(output, { lineWidth: -1, quotingType: '"' })
  }

  const yamlContent = generateYAML()

  const copyToClipboard = () => {
    navigator.clipboard.writeText(yamlContent)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const downloadYAML = () => {
    const blob = new Blob([yamlContent], { type: 'text/yaml' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'opengslb-config.yaml'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between px-4 py-3 bg-gray-800 border-b border-gray-700">
        <h3 className="font-medium text-gray-200">Generated Configuration</h3>
        <div className="flex gap-2">
          <button
            onClick={copyToClipboard}
            className="p-2 text-gray-400 hover:text-gray-200 hover:bg-gray-700 rounded transition-colors"
            title="Copy to clipboard"
          >
            {copied ? <Check className="w-4 h-4 text-success-500" /> : <Copy className="w-4 h-4" />}
          </button>
          <button
            onClick={downloadYAML}
            className="p-2 text-gray-400 hover:text-gray-200 hover:bg-gray-700 rounded transition-colors"
            title="Download YAML"
          >
            <Download className="w-4 h-4" />
          </button>
        </div>
      </div>
      <div className="flex-1 overflow-auto p-4 bg-gray-900">
        <pre className="yaml-preview text-gray-300 whitespace-pre-wrap">{yamlContent}</pre>
      </div>
    </div>
  )
}

// ============================================================================
// MAIN APP
// ============================================================================

function App() {
  const [config, setConfig] = useState(defaultConfig)
  const [currentSection, setCurrentSection] = useState('mode')
  const [showPreview, setShowPreview] = useState(true)

  const sections = config.mode === 'agent' ? agentSections : overwatchSections

  const isSectionConfigured = (sectionId) => {
    switch (sectionId) {
      case 'mode': return true
      case 'logging': return true
      case 'metrics': return true
      case 'identity': return !!config.overwatch.identity.node_id
      case 'dns': return true
      case 'gossip': return !!config.overwatch.gossip.encryption_key
      case 'tokens': return true
      case 'validation': return true
      case 'stale': return true
      case 'dnssec': return true
      case 'geolocation': return true
      case 'api': return true
      case 'regions': return config.regions.length > 0
      case 'domains': return config.domains.length > 0
      case 'agent_identity': return !!config.agent.identity.service_token && config.agent.identity.service_token.length >= 16
      case 'agent_backends': return config.agent.backends.length > 0
      case 'agent_gossip': return !!config.agent.gossip.encryption_key && config.agent.gossip.overwatch_nodes.length > 0
      case 'agent_heartbeat': return true
      case 'agent_predictive': return true
      default: return false
    }
  }

  const renderSection = () => {
    switch (currentSection) {
      case 'mode': return <ModeSection config={config} setConfig={setConfig} />
      case 'logging': return <LoggingSection config={config} setConfig={setConfig} />
      case 'metrics': return <MetricsSection config={config} setConfig={setConfig} />
      case 'identity': return <IdentitySection config={config} setConfig={setConfig} />
      case 'dns': return <DNSSection config={config} setConfig={setConfig} />
      case 'gossip': return <GossipSection config={config} setConfig={setConfig} />
      case 'tokens': return <TokensSection config={config} setConfig={setConfig} />
      case 'validation': return <ValidationSection config={config} setConfig={setConfig} />
      case 'stale': return <StaleSection config={config} setConfig={setConfig} />
      case 'dnssec': return <DNSSECSection config={config} setConfig={setConfig} />
      case 'geolocation': return <GeolocationSection config={config} setConfig={setConfig} />
      case 'api': return <APISection config={config} setConfig={setConfig} />
      case 'regions': return <RegionsSection config={config} setConfig={setConfig} />
      case 'domains': return <DomainsSection config={config} setConfig={setConfig} />
      case 'agent_identity': return <AgentIdentitySection config={config} setConfig={setConfig} />
      case 'agent_backends': return <AgentBackendsSection config={config} setConfig={setConfig} />
      case 'agent_gossip': return <AgentGossipSection config={config} setConfig={setConfig} />
      case 'agent_heartbeat': return <AgentHeartbeatSection config={config} setConfig={setConfig} />
      case 'agent_predictive': return <AgentPredictiveSection config={config} setConfig={setConfig} />
      default: return <ModeSection config={config} setConfig={setConfig} />
    }
  }

  return (
    <div className="min-h-screen bg-gray-900 flex">
      {/* Sidebar */}
      <aside className="w-64 bg-gray-800 border-r border-gray-700 flex flex-col">
        <div className="p-4 border-b border-gray-700">
          <h1 className="text-xl font-bold text-primary-400">OpenGSLB</h1>
          <p className="text-sm text-gray-400">Configuration Wizard</p>
        </div>

        <nav className="flex-1 overflow-y-auto p-2">
          {sections.map((section) => {
            const Icon = section.icon
            const isActive = currentSection === section.id
            const isConfigured = isSectionConfigured(section.id)

            return (
              <button
                key={section.id}
                onClick={() => setCurrentSection(section.id)}
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-left transition-colors mb-1 ${
                  isActive
                    ? 'bg-primary-500/20 text-primary-400'
                    : 'text-gray-300 hover:bg-gray-700/50'
                }`}
              >
                <Icon className="w-5 h-5 flex-shrink-0" />
                <span className="flex-1 text-sm">{section.name}</span>
                {isConfigured ? (
                  <Circle className="w-3 h-3 fill-success-500 text-success-500" />
                ) : (
                  <Circle className="w-3 h-3 text-gray-500" />
                )}
              </button>
            )
          })}
        </nav>

        <div className="p-4 border-t border-gray-700">
          <button
            onClick={() => setShowPreview(!showPreview)}
            className="w-full flex items-center justify-center gap-2 px-4 py-2 bg-gray-700 hover:bg-gray-600
              text-gray-200 rounded-lg transition-colors text-sm"
          >
            {showPreview ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            {showPreview ? 'Hide Preview' : 'Show Preview'}
          </button>
        </div>
      </aside>

      {/* Main Content */}
      <main className={`flex-1 flex ${showPreview ? '' : ''}`}>
        <div className={`${showPreview ? 'w-1/2' : 'w-full'} p-8 overflow-y-auto`}>
          {renderSection()}
        </div>

        {/* YAML Preview */}
        {showPreview && (
          <div className="w-1/2 border-l border-gray-700">
            <YAMLPreview config={config} />
          </div>
        )}
      </main>
    </div>
  )
}

export default App
