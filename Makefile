export GOPATH := $(shell go env GOPATH)

# We require CGO_ENABLED=1 for getting group information to work properly; the
# pure go version doesn't work on all systems such as those using LDAP for
# groups
export CGO_ENABLED = 1

default: install

build:
	go build -tags netgo

install:
	@rm -f ${GOPATH}/bin/backup-plans
	@go install -tags netgo
	@echo installed to ${GOPATH}/bin/backup-plans

test:
	@go test -tags netgo --count 1 -p 1 ./...

race:
	go test -tags netgo -race --count 1 -p 1 ./...

bench:
	go test -tags netgo --count 1 -p 1 -run Bench -bench=. ./...

# curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.4.0
lint:
	@golangci-lint run --timeout 2m

frontend-lint:
	@cd ./frontend/src && go run vimagination.zapto.org/jspacker/cmd/jspacker@latest -i "/$$(grep "<script" index.html | sed -e 's/.*src="\([^"]*\)".*/\1/')" -n > /dev/null

frontend-test:
	@cd ./frontend/src && \
		test_files="$$(find . -maxdepth 1 -type f \( -name "*.test.mjs" -o -name "*.test.js" \) | sort)"; \
		if [ -z "$$test_files" ]; then \
			echo "No frontend tests found"; \
			exit 0; \
		fi; \
		tmp_tests=""; \
		trap 'rm -f $$tmp_tests' EXIT; \
		for test_file in $$test_files; do \
			tmp_test=".tmp-$$(basename "$$test_file")"; \
			cp "$$test_file" "$$tmp_test"; \
			imports="$$(grep -Eo "['\"]\./[^'\"]+\.js['\"]" "$$test_file" | tr -d "'\"" | sort -u || true)"; \
			for module in $$imports; do \
				ts_module="$${module%.js}.ts"; \
				if [ ! -f "$$module" ] && [ -f "$$ts_module" ]; then \
					sed -i "s#'$$module'#'$$ts_module'#g; s#\"$$module\"#\"$$ts_module\"#g" "$$tmp_test"; \
				fi; \
			done; \
			tmp_tests="$$tmp_tests $$tmp_test"; \
		done; \
		node --experimental-strip-types --test $$tmp_tests

frontend-check: frontend-lint frontend-test

clean:
	@rm -f ./backup-plans
	@rm -f ./dist.zip

.PHONY: test race bench lint frontend-lint frontend-test frontend-check build install clean
