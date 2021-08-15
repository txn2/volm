# volm
WIP: PVC management API


## Development

Create test environment with manifests from `./k8s/`.

```
PVC_NAMESPACE=volm-test PVC_SELECTOR=pvci.txn2.com/service=pvci go run ./cmd/volm.go
```

## Build and Deploy

### Release
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```