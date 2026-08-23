.PHONY: build test vet verify clean package
build:
	mkdir -p bin
	go build -trimpath -o bin/certlife ./cmd/certlife

test:
	go test ./...

vet:
	go vet ./...

verify:
	bash scripts/verify.sh

package: build
	mkdir -p dist
	tar -czf dist/certlife-$$(go env GOOS)-$$(go env GOARCH).tar.gz bin/certlife config.yaml README.md

clean:
	rm -rf bin dist certlife.db deployments/*.json
