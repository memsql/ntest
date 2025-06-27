# Makefile for ntest integration testing

.PHONY: test-integration test-unit all

all: test

test-integration:
	@echo "Running ReplaceLogger integration test..."
	@go test -v -run TestReplaceLoggerIntegration > integration_output.txt 2>&1 
	@USER_LINE=$$(grep -n "test message from user code" integration_test.go | cut -d: -f1); \
	if grep -q "integration_test.go:$$USER_LINE" integration_output.txt; then \
		echo "✓ SUCCESS: User code line ($$USER_LINE) correctly reported"; \
	else \
		echo "✗ FAILURE: Found user code line ($$USER_LINE) not in output - should be there"; \
		exit 1; \
	fi
	@rm -f integration_output.txt

test-unit:
	go test
	go test -cover
	go test -bench .
	go test -race

test: test-unit test-integration

