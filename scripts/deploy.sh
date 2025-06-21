#!/bin/bash

# Entropic Deployment Script
# This script handles deployment of Entropic to various environments

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DEFAULT_ENV="development"
DEFAULT_VERSION="latest"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Help function
show_help() {
    cat << EOF
Entropic Deployment Script

Usage: $0 [OPTIONS] COMMAND

Commands:
    dev         Start development environment
    test        Run tests in containers
    build       Build Docker images
    deploy      Deploy to production
    stop        Stop all services
    clean       Clean up containers and volumes
    logs        Show service logs
    status      Show service status
    backup      Backup data volumes
    restore     Restore from backup

Options:
    -e, --env ENV       Environment (development|test|production) [default: development]
    -v, --version VER   Version tag for images [default: latest]
    -f, --force         Force operation without confirmation
    -h, --help          Show this help message

Examples:
    $0 dev                          # Start development environment
    $0 -e production deploy         # Deploy to production
    $0 build -v v1.2.3             # Build images with version tag
    $0 test                         # Run all tests
    $0 logs entropic                # Show entropic service logs

EOF
}

# Parse command line arguments
parse_args() {
    ENV="$DEFAULT_ENV"
    VERSION="$DEFAULT_VERSION"
    FORCE=false
    COMMAND=""

    while [[ $# -gt 0 ]]; do
        case $1 in
            -e|--env)
                ENV="$2"
                shift 2
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -f|--force)
                FORCE=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            dev|test|build|deploy|stop|clean|logs|status|backup|restore)
                COMMAND="$1"
                shift
                ;;
            *)
                if [[ -z "$COMMAND" ]]; then
                    log_error "Unknown command: $1"
                    show_help
                    exit 1
                else
                    # Pass remaining arguments to the command
                    break
                fi
                ;;
        esac
    done

    if [[ -z "$COMMAND" ]]; then
        log_error "No command specified"
        show_help
        exit 1
    fi
}

# Validate environment
validate_env() {
    case "$ENV" in
        development|dev)
            ENV="development"
            COMPOSE_FILE="docker-compose.yml"
            ;;
        test)
            COMPOSE_FILE="docker-compose.test.yml"
            ;;
        production|prod)
            ENV="production"
            COMPOSE_FILE="docker-compose.prod.yml"
            ;;
        *)
            log_error "Invalid environment: $ENV"
            exit 1
            ;;
    esac
}

# Check prerequisites
check_prerequisites() {
    local missing=()

    if ! command -v docker &> /dev/null; then
        missing+=("docker")
    fi

    if ! command -v docker-compose &> /dev/null; then
        missing+=("docker-compose")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        log_info "Please install the missing tools and try again"
        exit 1
    fi

    # Check Docker daemon
    if ! docker info &> /dev/null; then
        log_error "Docker daemon is not running"
        exit 1
    fi
}

# Build Docker images
build_images() {
    log_info "Building Docker images for version $VERSION..."

    cd "$PROJECT_DIR"

    # Set build arguments
    export VERSION
    export COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    export BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    # Build main application image
    docker build \
        --build-arg VERSION="$VERSION" \
        --build-arg COMMIT="$COMMIT" \
        --build-arg BUILD_TIME="$BUILD_TIME" \
        -t "entropic/server:$VERSION" \
        -t "entropic/server:latest" \
        .

    if [[ "$ENV" == "development" ]]; then
        # Build development image
        docker build -f Dockerfile.dev -t "entropic/server:dev" .
        
        # Build test image
        docker build -f Dockerfile.test -t "entropic/server:test" .
    fi

    log_success "Docker images built successfully"
}

# Start development environment
start_dev() {
    log_info "Starting development environment..."

    cd "$PROJECT_DIR"

    # Create necessary directories
    mkdir -p logs data/postgres data/typesense data/redis

    # Copy configuration if it doesn't exist
    if [[ ! -f "entropic.yaml" ]]; then
        cp "entropic.example.yaml" "entropic.yaml"
        log_warning "Created entropic.yaml from example. Please review and modify as needed."
    fi

    # Start services
    docker-compose up -d postgres typesense redis

    # Wait for services to be ready
    log_info "Waiting for services to be ready..."
    sleep 10

    # Run migrations
    log_info "Running database migrations..."
    make migrate || {
        log_warning "Migration failed. Services may not be ready yet."
    }

    # Start application
    docker-compose up -d entropic

    log_success "Development environment started"
    log_info "Application available at: http://localhost:8080"
    log_info "Health check: http://localhost:8080/health"
}

# Run tests
run_tests() {
    log_info "Running tests in containers..."

    cd "$PROJECT_DIR"

    # Start test services
    docker-compose -f docker-compose.test.yml up -d postgres-test typesense-test redis-test

    # Wait for services
    log_info "Waiting for test services to be ready..."
    sleep 15

    # Run unit tests
    log_info "Running unit tests..."
    docker-compose -f docker-compose.test.yml run --rm test-runner go test ./... -v -cover

    # Run integration tests
    log_info "Running integration tests..."
    docker-compose -f docker-compose.test.yml run --rm integration-test

    # Cleanup
    docker-compose -f docker-compose.test.yml down -v

    log_success "Tests completed"
}

# Deploy to production
deploy_production() {
    if [[ "$FORCE" != "true" ]]; then
        log_warning "Deploying to production environment"
        read -p "Are you sure you want to continue? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Deployment cancelled"
            exit 0
        fi
    fi

    log_info "Deploying to production..."

    cd "$PROJECT_DIR"

    # Validate production configuration
    if [[ ! -f "entropic.prod.yaml" ]]; then
        log_error "Production configuration file 'entropic.prod.yaml' not found"
        exit 1
    fi

    # Check for secrets
    if [[ ! -d "secrets" ]]; then
        log_error "Secrets directory not found. Please create secrets/ with required files."
        exit 1
    fi

    # Create data directories
    sudo mkdir -p /data/postgres /data/typesense /data/redis /logs/entropic /logs/nginx

    # Deploy with Docker Compose
    docker-compose -f docker-compose.prod.yml up -d

    log_success "Production deployment completed"
}

# Stop services
stop_services() {
    log_info "Stopping services..."

    cd "$PROJECT_DIR"

    case "$ENV" in
        development)
            docker-compose down
            ;;
        test)
            docker-compose -f docker-compose.test.yml down -v
            ;;
        production)
            docker-compose -f docker-compose.prod.yml down
            ;;
    esac

    log_success "Services stopped"
}

# Clean up
cleanup() {
    if [[ "$FORCE" != "true" ]]; then
        log_warning "This will remove all containers, volumes, and data"
        read -p "Are you sure you want to continue? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Cleanup cancelled"
            exit 0
        fi
    fi

    log_info "Cleaning up..."

    cd "$PROJECT_DIR"

    # Stop and remove containers
    docker-compose down -v --remove-orphans
    docker-compose -f docker-compose.test.yml down -v --remove-orphans 2>/dev/null || true
    docker-compose -f docker-compose.prod.yml down -v --remove-orphans 2>/dev/null || true

    # Remove images
    docker rmi entropic/server:latest entropic/server:dev entropic/server:test 2>/dev/null || true

    # Prune unused resources
    docker system prune -f

    log_success "Cleanup completed"
}

# Show logs
show_logs() {
    local service="${1:-}"
    
    cd "$PROJECT_DIR"

    if [[ -n "$service" ]]; then
        docker-compose -f "$COMPOSE_FILE" logs -f "$service"
    else
        docker-compose -f "$COMPOSE_FILE" logs -f
    fi
}

# Show status
show_status() {
    log_info "Service status:"

    cd "$PROJECT_DIR"

    docker-compose -f "$COMPOSE_FILE" ps
}

# Backup data
backup_data() {
    local backup_dir="./backups/$(date +%Y%m%d_%H%M%S)"
    
    log_info "Creating backup in $backup_dir..."

    mkdir -p "$backup_dir"

    # Backup database
    docker-compose -f "$COMPOSE_FILE" exec -T postgres pg_dump -U postgres entropic > "$backup_dir/database.sql"

    # Backup Typesense data
    docker-compose -f "$COMPOSE_FILE" exec -T typesense tar czf - /data > "$backup_dir/typesense.tar.gz"

    log_success "Backup created in $backup_dir"
}

# Restore from backup
restore_data() {
    local backup_dir="${1:-}"
    
    if [[ -z "$backup_dir" || ! -d "$backup_dir" ]]; then
        log_error "Please specify a valid backup directory"
        exit 1
    fi

    log_warning "This will restore data from $backup_dir"
    if [[ "$FORCE" != "true" ]]; then
        read -p "Are you sure you want to continue? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            log_info "Restore cancelled"
            exit 0
        fi
    fi

    log_info "Restoring from backup..."

    cd "$PROJECT_DIR"

    # Restore database
    if [[ -f "$backup_dir/database.sql" ]]; then
        docker-compose -f "$COMPOSE_FILE" exec -T postgres psql -U postgres -d entropic < "$backup_dir/database.sql"
    fi

    # Restore Typesense data
    if [[ -f "$backup_dir/typesense.tar.gz" ]]; then
        docker-compose -f "$COMPOSE_FILE" exec -T typesense tar xzf - -C / < "$backup_dir/typesense.tar.gz"
    fi

    log_success "Restore completed"
}

# Main function
main() {
    parse_args "$@"
    validate_env
    check_prerequisites

    case "$COMMAND" in
        dev)
            start_dev
            ;;
        test)
            run_tests
            ;;
        build)
            build_images
            ;;
        deploy)
            if [[ "$ENV" == "production" ]]; then
                deploy_production
            else
                start_dev
            fi
            ;;
        stop)
            stop_services
            ;;
        clean)
            cleanup
            ;;
        logs)
            show_logs "$@"
            ;;
        status)
            show_status
            ;;
        backup)
            backup_data
            ;;
        restore)
            restore_data "$@"
            ;;
        *)
            log_error "Unknown command: $COMMAND"
            exit 1
            ;;
    esac
}

# Run main function
main "$@"