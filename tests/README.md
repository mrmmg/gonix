# Tests

Following standard Go convention, tests live next to the code they test as `*_test.go` files
within each package under `internal/` (e.g. `internal/nginx/renderer_test.go`), rather than in a
separate top-level tree. This directory is kept as a placeholder for any future
integration/end-to-end tests that don't belong to a single package.

Run the full suite with:

```bash
go test ./...
```
