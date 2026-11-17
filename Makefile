export GOTOOLCHAIN := go1.27.4
export GOWORK := off
export GOFLAGS := -tags=feature_lsp,feature_wasm_ext
export CGO_ENABLED := 0

.PHONY: check lint test build matrix inventory fmt
check:
	go run ./scripts check
lint:
	go run ./scripts lint
test:
	go test ./...
build:
	go build -trimpath -o bin/ ./cmd/opera
matrix:
	go run ./scripts matrix
inventory:
	go run ./scripts inventory
fmt:
	gofmt -w cmd internal scripts

package:
	go run ./scripts package
package-smoke:
	go run ./scripts package-smoke
