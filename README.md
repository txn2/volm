# volm
WIP: PVC management API

## Run

From source:
```
PVC_NAMESPACE=volm-test PVC_SELECTOR=pvci.txn2.com/service=pvci go run ./cmd/volm.go
```

## Endpoints

**Get list of PVCs**:
```
curl --location --request GET 'http://localhost:8070/vol/' | jq
```

**Get a PVC**:
```
curl --location --request GET 'http://localhost:8070/vol/volm-test-pvc-1' | jq
```

**Delete a PVC**:
```
curl --location --request DELETE 'http://localhost:8070/vol/volm-test-pvc-1' | jq
```

## Development

Create test environment with manifests from `./k8s/`.

## Build and Deploy

### Release
```bash
goreleaser --skip-publish --rm-dist --skip-validate
```

```bash
GITHUB_TOKEN=$GITHUB_TOKEN goreleaser --rm-dist
```