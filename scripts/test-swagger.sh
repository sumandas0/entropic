#!/bin/bash
echo "Testing Swagger documentation generation and serving..."
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'
echo "Generating Swagger documentation..."
if swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal; then
    echo -e "${GREEN}✓ Swagger documentation generated successfully${NC}"
else
    echo -e "${RED}✗ Failed to generate Swagger documentation${NC}"
    exit 1
fi
if [ -f "docs/swagger.json" ] && [ -f "docs/swagger.yaml" ] && [ -f "docs/docs.go" ]; then
    echo -e "${GREEN}✓ All documentation files created${NC}"
else
    echo -e "${RED}✗ Documentation files missing${NC}"
    exit 1
fi
echo ""
echo "Generated files:"
ls -lh docs/
echo ""
echo "Swagger documentation is ready!"
echo "To view the documentation:"
echo "1. Start the server: go run cmd/server/main.go"
echo "2. Open browser to: http://localhost:8080/swagger/index.html"
echo ""
echo "Or use the standalone Swagger UI:"
echo "make openapi-serve"
