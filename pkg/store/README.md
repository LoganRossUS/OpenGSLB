# KV Store

The `pkg/store` package provides an embedded key-value store with support for both local (standalone) and replicated (cluster) modes.

## Interface

The `Store` interface provides standard operations:
- `Get(ctx, key)`
- `Set(ctx, key, value)`
- `Delete(ctx, key)`
- `List(ctx, prefix)`
- `Watch(ctx, prefix)`

## Implementations

### BboltStore
Uses `bbolt` for persistent local storage. Suitable for standalone agents.
- Data is stored in `kv.db`.
- Supports full persistence.

### RaftStore
Wraps the Raft cluster node to provide a replicated store.
- Writes (`Set`, `Delete`) are proposed as Raft commands.
- Reads (`Get`, `List`) serve from the local FSM state (weak consistency).
- Data is replicated across the cluster.

## Usage

```go
cfg := store.Config{
    Type: store.StoreTypeBbolt,
    DataDir: "/var/lib/opengslb",
}
s, err := store.NewStore(cfg)
```

## Key Namespaces (Convention)
- `health/`: Health state
- `agents/`: Agent metadata
- `overrides/`: Runtime overrides
- `cluster/`: Cluster state
