# Overwatch store written by OpenGSLB 1.1.10

`overwatch.db` is a bbolt store as version 1.1.10 (`develop` at `52f84c4`) left it on disk. It is the fixture
of `TestStoreMigration_1_1_10` (`cmd/opengslb/store_migration_test.go`), which starts the current overwatch
on a copy of it.

It was produced by running that version in-process with the configuration of the test (one static server,
`app.gslb.example.com:127.0.0.1:18080`) and these API calls:

| Call | Body |
|---|---|
| `POST /api/v1/domains` | `{"name":"legacy.gslb.example.com","ttl":20,"routing_policy":"weighted","enabled":true,"description":"created by 1.1.10"}` |
| `POST /api/v1/domains/legacy.gslb.example.com/backends` | `{"address":"127.0.0.3","port":8080,"weight":10,"priority":1,"region":"us-east","enabled":true}` |
| `POST /api/v1/domains/legacy.gslb.example.com/backends` | `{"address":"127.0.0.4","port":8080,"weight":20,"region":"eu-west"}` (no `enabled`, stored as `false`) |
| `POST /api/v1/domains/legacy.gslb.example.com/backends` | `{"address":"127.0.0.6","port":8080,"weight":5,"region":"us-east","enabled":true}` |
| `POST /api/v1/servers` | `{"name":"legacy.gslb.example.com","address":"127.0.0.6","port":8080,"weight":50,"region":"eu-west","enabled":true}` |
| `POST /api/v1/servers` | `{"name":"legacy.gslb.example.com","address":"127.0.0.5","port":9001,"weight":30,"region":"us-east","enabled":true}` |
| `POST /api/v1/regions` | `{"name":"ap-south","code":"ap-south","description":"created by 1.1.10"}` |

Keys in the store (bucket `opengslb`):

```
backends/app.gslb.example.com:127.0.0.1:18080          source static
backends/legacy.gslb.example.com:127.0.0.5:9001        source api
backends/legacy.gslb.example.com:127.0.0.6:8080        source api, weight 50, region eu-west
domain_backends/legacy.gslb.example.com/legacy.gslb.example.com:127.0.0.3:8080
domain_backends/legacy.gslb.example.com/legacy.gslb.example.com:127.0.0.4:8080   "enabled": false
domain_backends/legacy.gslb.example.com/legacy.gslb.example.com:127.0.0.6:8080   weight 5, region us-east
domains/legacy.gslb.example.com                        "routing_policy": "weighted", no routing_algorithm
regions/ap-south
```

Do not regenerate it with a newer version: the point of the file is the old layout.
