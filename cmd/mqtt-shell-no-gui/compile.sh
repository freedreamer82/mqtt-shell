#!/bin/bash
# Enhanced compilation script for MQTT Shell
# Usage: ./compile.sh [architecture]
# If no architecture is specified, all architectures will be compiled

# Display help information
show_help() {
    echo "MQTT Shell Compilation Script"
    echo ""
    echo "Usage: $0 [architecture]"
    echo ""
    echo "If no architecture is specified, all architectures will be compiled."
    echo ""
    echo "Available architectures:"
    echo "  arm64        ARM 64-bit (Linux)"
    echo "  arm          ARM 32-bit (Linux)"
    echo "  amd64        x86-64 (Linux)"
    echo "  386          x86-32 (Linux)"
    echo "  darwin-arm64 MacOS ARM 64-bit"
    echo "  all          Compile for all architectures (default)"
    echo ""
    exit 0
}

# Check for help option
if [[ "$1" == "--help" || "$1" == "-h" ]]; then
    show_help
fi

# Clean previous builds
rm -rf mqtt-shell*

# Set build flags for all architectures
BUILD_FLAGS="-ldflags '-w -s'"

# Function to build for a specific architecture
build_arch() {
    case $1 in
        arm64)
            echo "Building for ARM 64-bit (Linux)..."
            env GOOS=linux GOARCH=arm64 go build -o mqtt-shell-arm64 -ldflags '-w -s'
            ;;
        arm)
            echo "Building for ARM 32-bit (Linux)..."
            env GOOS=linux GOARCH=arm go build -o mqtt-shell-arm32 -ldflags '-w -s'
            ;;
        amd64)
            echo "Building for x86-64 (Linux)..."
            env GOOS=linux GOARCH=amd64 go build -o mqtt-shell-x86-64 -ldflags '-w -s'
            ;;
        386)
            echo "Building for x86-32 (Linux)..."
            env GOOS=linux GOARCH=386 go build -o mqtt-shell-x86-32 -ldflags '-w -s'
            ;;
        darwin-arm64)
            echo "Building for MacOS ARM 64-bit..."
            env GOOS=darwin GOARCH=arm64 go build -o mqtt-shell-macos-arm64 -ldflags '-w -s'
            ;;
        all)
            echo "Building for all architectures..."
            env GOOS=linux GOARCH=arm64 go build -o mqtt-shell-arm64 -ldflags '-w -s'
            env GOOS=linux GOARCH=arm go build -o mqtt-shell-arm32 -ldflags '-w -s'
            env GOOS=linux GOARCH=amd64 go build -o mqtt-shell-x86-64 -ldflags '-w -s'
            env GOOS=linux GOARCH=386 go build -o mqtt-shell-x86-32 -ldflags '-w -s'
            env GOOS=darwin GOARCH=arm64 go build -o mqtt-shell-macos-arm64 -ldflags '-w -s'
            ;;
        *)
            echo "Error: Unknown architecture '$1'"
            echo "Run '$0 --help' for usage information"
            exit 1
            ;;
    esac
}

# Execute the build based on arguments
if [ $# -eq 0 ]; then
    # No architecture specified, build all
    build_arch all
else
    # Build for the specified architecture
    build_arch "$1"
fi

echo "Build complete!"
