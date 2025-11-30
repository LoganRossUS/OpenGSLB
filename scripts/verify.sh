#!/bin/bash
# Pre-PR verification script
# Run this before opening a pull request

set -e  # Exit on first error

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== OpenGSLB Pre-PR Verification ===${NC}"
echo ""

# Determine what to check - default to all, or pass a package path
TARGET="${1:-./...}"
echo -e "Target: ${TARGET}"
echo ""

# Step 1: Format with gofmt
echo -e "${YELLOW}[1/6] Running gofmt...${NC}"
GOFMT_OUT=$(gofmt -l -w .)
if [ -n "$GOFMT_OUT" ]; then
    echo -e "${YELLOW}Formatted files:${NC}"
    echo "$GOFMT_OUT"
else
    echo -e "${GREEN}No gofmt changes needed${NC}"
fi
echo ""

# Step 2: Format with go fmt (catches module-level issues)
echo -e "${YELLOW}[2/6] Running go fmt...${NC}"
FMTOUT=$(go fmt ${TARGET})
if [ -n "$FMTOUT" ]; then
    echo -e "${YELLOW}Formatted files:${NC}"
    echo "$FMTOUT"
else
    echo -e "${GREEN}No go fmt changes needed${NC}"
fi
echo ""

# Step 3: Vet
echo -e "${YELLOW}[3/6] Running go vet...${NC}"
go vet ${TARGET}
echo -e "${GREEN}Vet passed${NC}"
echo ""

# Step 4: Build
echo -e "${YELLOW}[4/6] Running go build...${NC}"
go build ${TARGET}
echo -e "${GREEN}Build passed${NC}"
echo ""

# Step 5: Tests with race detection and coverage
echo -e "${YELLOW}[5/6] Running tests with race detection...${NC}"
go test -race -cover ${TARGET}
echo -e "${GREEN}Tests passed${NC}"
echo ""

# Step 6: Lint (optional - skip if not installed)
echo -e "${YELLOW}[6/6] Running golangci-lint...${NC}"
LINT_PATH="${HOME}/go/bin/golangci-lint"
if command -v golangci-lint &> /dev/null; then
    golangci-lint run ${TARGET}
    echo -e "${GREEN}Lint passed${NC}"
elif [ -x "$LINT_PATH" ]; then
    $LINT_PATH run ${TARGET}
    echo -e "${GREEN}Lint passed${NC}"
else
    echo -e "${YELLOW}golangci-lint not found, skipping (CI will run it)${NC}"
fi
echo ""

echo -e "${GREEN}=== All checks passed! Ready to open PR ===${NC}"
