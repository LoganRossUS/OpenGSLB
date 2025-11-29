#!/bin/bash
# Quick Reference: Story 2 Implementation Commands
# Save this file as: scripts/story2-setup.sh
# Make executable: chmod +x scripts/story2-setup.sh

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}OpenGSLB - Story 2: CI Pipeline Setup${NC}"
echo ""

# Step 1: Create directory structure
echo -e "${GREEN}Step 1: Creating directory structure...${NC}"
mkdir -p .github/workflows
mkdir -p pkg/version
echo "✓ Directories created"
echo ""

# Step 2: Verify go.mod exists
echo -e "${GREEN}Step 2: Checking Go module...${NC}"
if [ -f "go.mod" ]; then
    echo "✓ go.mod exists"
else
    echo "⚠ go.mod not found - you'll need to create it manually"
fi
echo ""

# Step 3: Initialize/update Go modules
echo -e "${GREEN}Step 3: Running go mod tidy...${NC}"
go mod tidy
echo "✓ Go modules updated"
echo ""

# Step 4: Run local tests
echo -e "${GREEN}Step 4: Running tests locally...${NC}"
go test -race ./...
echo "✓ Tests completed"
echo ""

# Step 5: Run build
echo -e "${GREEN}Step 5: Running build...${NC}"
go build ./...
echo "✓ Build completed"
echo ""

# Step 6: Run linter (if installed)
echo -e "${GREEN}Step 6: Running linter (if available)...${NC}"
if command -v golangci-lint &> /dev/null; then
    golangci-lint run
    echo "✓ Linting completed"
else
    echo "⚠ golangci-lint not installed locally (CI will run it)"
fi
echo ""

echo -e "${BLUE}All local checks passed!${NC}"
echo ""
echo "Next steps:"
echo "1. Review the files created in .github/workflows/ and pkg/version/"
echo "2. Commit changes: git add ."
echo "3. Create feature branch: git checkout -b feature/ci-pipeline"
echo "4. Push: git push origin feature/ci-pipeline"
echo "5. Create PR on GitHub"
echo ""
echo "For detailed instructions, see the 'Story 2 Implementation Guide' artifact"