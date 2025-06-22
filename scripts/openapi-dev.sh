#!/bin/bash

# OpenAPI Development Helper Script
# This script helps with OpenAPI development tasks

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Change to project root
cd "$PROJECT_ROOT"

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to install required tools
install_tools() {
    print_status "Checking and installing required tools..."
    
    # Install Go tools
    if ! command_exists swag; then
        print_status "Installing swag..."
        go install github.com/swaggo/swag/cmd/swag@v1.16.2
    fi
    
    if ! command_exists oapi-codegen; then
        print_status "Installing oapi-codegen..."
        go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.3.0
    fi
    
    # Install Node tools
    if ! command_exists swagger-cli; then
        print_status "Installing swagger-cli..."
        npm install -g @apidevtools/swagger-cli
    fi
    
    if ! command_exists openapi-to-postmanv2; then
        print_status "Installing openapi-to-postmanv2..."
        npm install -g openapi-to-postmanv2
    fi
    
    print_status "All tools installed successfully!"
}

# Function to validate OpenAPI spec
validate_spec() {
    print_status "Validating OpenAPI specification..."
    
    if [ ! -f "api/openapi.yaml" ]; then
        print_error "OpenAPI spec not found at api/openapi.yaml"
        exit 1
    fi
    
    swagger-cli validate api/openapi.yaml
    
    if [ $? -eq 0 ]; then
        print_status "OpenAPI spec is valid!"
    else
        print_error "OpenAPI spec validation failed!"
        exit 1
    fi
}

# Function to generate code from OpenAPI spec
generate_code() {
    print_status "Generating code from OpenAPI spec..."
    
    # Create directories if they don't exist
    mkdir -p internal/generated
    mkdir -p pkg/client
    
    # Generate server code
    print_status "Generating server interfaces..."
    oapi-codegen -generate types,server,spec -package generated -o internal/generated/openapi_types.gen.go api/openapi.yaml
    
    # Generate client code
    print_status "Generating client code..."
    oapi-codegen -generate types,client -package client -o pkg/client/openapi_client.gen.go api/openapi.yaml
    
    print_status "Code generation complete!"
}

# Function to generate documentation from code annotations
generate_docs_from_code() {
    print_status "Generating OpenAPI documentation from code annotations..."
    
    swag init -g cmd/server/main.go -o docs/swagger --parseDependency --parseInternal
    
    print_status "Documentation generated in docs/swagger/"
}

# Function to generate Postman collection
generate_postman() {
    print_status "Generating Postman collection..."
    
    openapi-to-postmanv2 -s api/openapi.yaml -o api/postman-collection.json -p
    
    print_status "Postman collection generated at api/postman-collection.json"
}

# Function to serve OpenAPI documentation
serve_docs() {
    print_status "Starting Swagger UI at http://localhost:8081"
    print_status "Press Ctrl+C to stop..."
    
    docker run --rm -p 8081:8080 \
        -e SWAGGER_JSON=/api/openapi.yaml \
        -v "$PROJECT_ROOT/api:/api" \
        swaggerapi/swagger-ui
}

# Function to watch for changes and regenerate
watch_mode() {
    print_status "Watching for OpenAPI spec changes..."
    print_status "Press Ctrl+C to stop..."
    
    # Check if fswatch is installed
    if ! command_exists fswatch; then
        print_error "fswatch not found. Install it with: brew install fswatch"
        exit 1
    fi
    
    # Watch for changes
    fswatch -o api/openapi.yaml | while read num ; do
        print_status "OpenAPI spec changed, regenerating..."
        validate_spec
        generate_code
        print_status "Regeneration complete!"
        echo ""
    done
}

# Function to compare specs for breaking changes
compare_specs() {
    local base_spec=$1
    local new_spec=$2
    
    if [ -z "$base_spec" ] || [ -z "$new_spec" ]; then
        print_error "Usage: $0 compare <base-spec> <new-spec>"
        exit 1
    fi
    
    print_status "Comparing OpenAPI specs for breaking changes..."
    
    # Check if oasdiff is installed
    if ! command_exists oasdiff; then
        print_status "Installing oasdiff..."
        go install github.com/tufin/oasdiff/cmd/oasdiff@latest
    fi
    
    oasdiff -base "$base_spec" -revision "$new_spec" -format text
}

# Function to generate SDK for different languages
generate_sdk() {
    local language=$1
    
    if [ -z "$language" ]; then
        print_error "Usage: $0 sdk <language>"
        print_status "Supported languages: go, typescript, python, java, ruby"
        exit 1
    fi
    
    print_status "Generating $language SDK..."
    
    # Create SDK directory
    mkdir -p "sdk/$language"
    
    # Use OpenAPI Generator
    docker run --rm \
        -v "$PROJECT_ROOT:/local" \
        openapitools/openapi-generator-cli generate \
        -i /local/api/openapi.yaml \
        -g "$language" \
        -o "/local/sdk/$language"
    
    print_status "$language SDK generated in sdk/$language/"
}

# Function to show help
show_help() {
    echo "OpenAPI Development Helper"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  install     Install required tools"
    echo "  validate    Validate OpenAPI specification"
    echo "  generate    Generate code from OpenAPI spec"
    echo "  docs        Generate docs from code annotations"
    echo "  postman     Generate Postman collection"
    echo "  serve       Serve OpenAPI documentation locally"
    echo "  watch       Watch for changes and regenerate"
    echo "  compare     Compare two specs for breaking changes"
    echo "  sdk         Generate SDK for a specific language"
    echo "  all         Run validate and generate"
    echo "  help        Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 validate"
    echo "  $0 generate"
    echo "  $0 serve"
    echo "  $0 compare api/openapi-v1.yaml api/openapi.yaml"
    echo "  $0 sdk typescript"
}

# Main script logic
case "$1" in
    install)
        install_tools
        ;;
    validate)
        validate_spec
        ;;
    generate)
        validate_spec
        generate_code
        ;;
    docs)
        generate_docs_from_code
        ;;
    postman)
        generate_postman
        ;;
    serve)
        serve_docs
        ;;
    watch)
        watch_mode
        ;;
    compare)
        compare_specs "$2" "$3"
        ;;
    sdk)
        generate_sdk "$2"
        ;;
    all)
        validate_spec
        generate_code
        generate_postman
        ;;
    help|"")
        show_help
        ;;
    *)
        print_error "Unknown command: $1"
        show_help
        exit 1
        ;;
esac