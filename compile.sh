#!/bin/bash
# Enhanced compilation script for MQTT Shell
# Supports multiple OS, architectures, GUI modes and output directories

# Default values
DEFAULT_OS="linux"
DEFAULT_ARCH="amd64"
DEFAULT_MODE="hybrid"  # hybrid (mqtt-shell), gui-only, cli-only
DEFAULT_OUTPUT_DIR="./dist"
DEFAULT_USE_FYNE_CROSS=false

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print colored output
print_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Display help information
show_help() {
    echo "MQTT Shell Enhanced Compilation Script"
    echo ""
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "OPTIONS:"
    echo "  -o, --os OS           Target operating system (default: linux)"
    echo "  -a, --arch ARCH       Target architecture (default: amd64)"
    echo "  -m, --mode MODE       Compilation mode (default: hybrid)"
    echo "  -d, --output-dir DIR  Output directory (default: ./dist)"
    echo "  -f, --fyne-cross      Use fyne-cross for GUI applications"
    echo "  -p, --parameters \"PARAMS\"  Extra parameters to pass to the build command"
    echo "  -c, --clean           Clean output directory before build"
    echo "  -l, --list            List all available options"
    echo "  -h, --help            Show this help message"
    echo "  --android             Shortcut: Build GUI for Android (all archs, fyne-cross)"
    echo "  --ios                 Shortcut: Build GUI for iOS (all archs, fyne-cross)"
    echo ""
    echo "OPERATING SYSTEMS:"
    echo "  linux, windows, darwin, android, ios"
    echo ""
    echo "ARCHITECTURES:"
    echo "  amd64, 386, arm, arm64, multiple (for mobile)"
    echo ""
    echo "MODES:"
    echo "  hybrid    - Command line app with --gui flag (mqtt-shell) [DEFAULT]"
    echo "  gui-only  - GUI-only application (mqtt-shell-gui)"
    echo "  cli-only  - Command line only (mqtt-shell-no-gui)"
    echo ""
    echo "EXAMPLES:"
    echo "  $0                                    # Build hybrid app for Linux amd64"
    echo "  $0 -o windows -a amd64 -m gui-only    # Build GUI-only for Windows 64-bit"
    echo "  $0 -o linux -a arm64 -m cli-only      # Build CLI-only for Linux ARM64"
    echo "  $0 --android                          # Build GUI for Android (all archs, fyne-cross)"
    echo "  $0 --ios                              # Build GUI for iOS (all archs, fyne-cross)"
    echo "  $0 -l                                 # List all available options"
    echo ""
    exit 0
}

# List available options
list_options() {
    echo "Available compilation options:"
    echo ""
    echo "Operating Systems:"
    echo "  linux     - Linux (all architectures)"
    echo "  windows   - Windows (amd64, 386)"
    echo "  darwin    - macOS (amd64, arm64)"
    echo "  android   - Android (requires fyne-cross, multiple arch)"
    echo "  ios       - iOS (requires fyne-cross, multiple arch)"
    echo ""
    echo "Architectures:"
    echo "  amd64     - x86-64 (64-bit)"
    echo "  386       - x86-32 (32-bit)"
    echo "  arm       - ARM 32-bit"
    echo "  arm64     - ARM 64-bit"
    echo "  multiple  - All supported architectures (mobile only)"
    echo ""
    echo "Compilation Modes:"
    echo "  hybrid    - mqtt-shell (CLI + GUI via --gui flag)"
    echo "  gui-only  - mqtt-shell-gui (GUI application only)"
    echo "  cli-only  - mqtt-shell-no-gui (CLI application only)"
    echo ""
    echo "Shortcuts:"
    echo "  --android   Shortcut for: -o android -a multiple -m gui-only -f"
    echo "  --ios       Shortcut for: -o ios -a multiple -m gui-only -f"
    echo ""
    exit 0
}

# Validate OS and architecture combination
validate_combination() {
    local os=$1
    local arch=$2
    local mode=$3
    
    case $os in
        linux)
            case $arch in
                amd64|386|arm|arm64) return 0 ;;
                *) print_error "Unsupported architecture '$arch' for Linux"; return 1 ;;
            esac
            ;;
        windows)
            case $arch in
                amd64|386) return 0 ;;
                *) print_error "Unsupported architecture '$arch' for Windows"; return 1 ;;
            esac
            ;;
        darwin)
            case $arch in
                amd64|arm64) return 0 ;;
                *) print_error "Unsupported architecture '$arch' for macOS"; return 1 ;;
            esac
            ;;
        android|ios)
            if [ "$arch" != "multiple" ]; then
                print_error "Mobile platforms only support 'multiple' architecture"
                return 1
            fi
            if [ "$mode" == "cli-only" ]; then
                print_error "CLI-only mode not supported for mobile platforms"
                return 1
            fi
            return 0
            ;;
        *)
            print_error "Unsupported operating system '$os'"
            return 1
            ;;
    esac
}

# Get source directory based on mode
get_source_dir() {
    case $1 in
        hybrid) echo "./cmd/mqtt-shell" ;;
        gui-only) echo "./cmd/mqtt-shell-gui" ;;
        cli-only) echo "./cmd/mqtt-shell-no-gui" ;;
        *) print_error "Unknown mode '$1'"; exit 1 ;;
    esac
}

# Generate output filename
generate_filename() {
    local os=$1
    local arch=$2
    local mode=$3
    
    local base_name="mqtt-shell"
    local suffix=""
    
    case $mode in
        gui-only) suffix="-gui" ;;
        cli-only) suffix="-cli" ;;
    esac
    
    if [ "$os" == "windows" ]; then
        echo "${base_name}${suffix}-${os}-${arch}.exe"
    elif [ "$os" == "android" ] || [ "$os" == "ios" ]; then
        echo "${base_name}${suffix}-${os}"
    else
        echo "${base_name}${suffix}-${os}-${arch}"
    fi
}

# Build using standard Go build
build_with_go() {
    local os=$1
    local arch=$2
    local mode=$3
    local output_dir=$4
    local extra_parameters=$5
    
    local source_dir=$(get_source_dir $mode)
    local filename=$(generate_filename $os $arch $mode)
    local output_path="${output_dir}/${filename}"
    
    print_info "Building $mode for $os/$arch using Go build..."
    print_info "Source: $source_dir"
    print_info "Output: $output_path"
    
    # Set build environment
    export GOOS=$os
    export GOARCH=$arch
    
    # Build the application
    if go build $extra_parameters -o "$output_path" -ldflags '-w -s' "$source_dir"; then
        print_success "Built: $output_path"
        return 0
    else
        print_error "Failed to build $mode for $os/$arch"
        return 1
    fi
}

# Build using fyne-cross
build_with_fyne_cross() {
    local os=$1
    local arch=$2
    local mode=$3
    local output_dir=$4
    local extra_parameters=$5
    
    local source_dir=$(get_source_dir $mode)
    local app_id="com.mqttshell"
    
    # Check if fyne-cross is available
    local fyne_cross_path
    if command -v fyne-cross &> /dev/null; then
        fyne_cross_path="fyne-cross"
    elif [ -f "$HOME/go/bin/fyne-cross" ]; then
        fyne_cross_path="$HOME/go/bin/fyne-cross"
        print_info "Found fyne-cross at $HOME/go/bin/fyne-cross"
    elif [ -f "$(go env GOPATH)/bin/fyne-cross" ]; then
        fyne_cross_path="$(go env GOPATH)/bin/fyne-cross"
        print_info "Found fyne-cross at $(go env GOPATH)/bin/fyne-cross"
    else
        print_error "fyne-cross is not installed or not in PATH."
        print_error "Please install it with:"
        print_error "  go install fyne.io/fyne/v2/cmd/fyne@latest"
        print_error "  go install github.com/fyne-io/fyne-cross@latest"
        print_error ""
        print_error "Then add Go bin directory to PATH:"
        print_error "  export PATH=\$PATH:\$(go env GOPATH)/bin"
        print_error "  # or add to ~/.bashrc: export PATH=\$PATH:\$HOME/go/bin"
        return 1
    fi
    
    print_info "Building $mode for $os/$arch using fyne-cross..."
    print_info "Source: $source_dir"
    
    # Build command arguments
    local fyne_args=()
    fyne_args+=("-app-id=$app_id")
    fyne_args+=("-debug")
    fyne_args+=("-no-cache")
    
    # Add icon if it exists
    if [ -f "assets/mqtt-shell-ico.png" ]; then
        fyne_args+=("-icon=assets/mqtt-shell-ico.png")
    fi
    
    # Add architecture
    if [ "$os" != "ios" ]; then
        if [ "$arch" == "multiple" ]; then
            fyne_args+=("-arch=multiple")
        else
            fyne_args+=("-arch=$arch")
        fi
    fi
    
    # Execute fyne-cross
    if "$fyne_cross_path" "$os" "${fyne_args[@]}" $extra_parameters "$source_dir"; then
        print_success "Built with fyne-cross for $os"
        
        # Move files to output directory if different
        if [ "$output_dir" != "./dist" ] && [ -d "fyne-cross/dist" ]; then
            mkdir -p "$output_dir"
            cp -r fyne-cross/dist/* "$output_dir/"
            print_info "Moved files to $output_dir"
        fi
        return 0
    else
        print_error "Failed to build with fyne-cross"
        return 1
    fi
}

# Main build function
build_application() {
    local os=$1
    local arch=$2
    local mode=$3
    local output_dir=$4
    local use_fyne_cross=$5
    local extra_parameters=$6

    # Validate combination
    if ! validate_combination "$os" "$arch" "$mode"; then
        return 1
    fi
    
    # Create output directory
    mkdir -p "$output_dir"
    
    # Choose build method
    if [ "$use_fyne_cross" == "true" ] || [ "$os" == "android" ] || [ "$os" == "ios" ]; then
        build_with_fyne_cross "$os" "$arch" "$mode" "$output_dir" "$extra_parameters"
    else
        build_with_go "$os" "$arch" "$mode" "$output_dir" "$extra_parameters"
    fi
}

# Parse command line arguments
parse_args() {
    local os=$DEFAULT_OS
    local arch=$DEFAULT_ARCH
    local mode=$DEFAULT_MODE
    local output_dir=$DEFAULT_OUTPUT_DIR
    local use_fyne_cross=$DEFAULT_USE_FYNE_CROSS
    local clean=false
    local extra_parameters=""
    
    while [[ $# -gt 0 ]]; do
        case $1 in
            -o|--os)
                os="$2"
                shift 2
                ;;
            -a|--arch)
                arch="$2"
                shift 2
                ;;
            -m|--mode)
                mode="$2"
                shift 2
                ;;
            -d|--output-dir)
                output_dir="$2"
                shift 2
                ;;
            -f|--fyne-cross)
                use_fyne_cross=true
                shift
                ;;
            --android)
                os="android"
                arch="multiple"
                mode="gui-only"
                use_fyne_cross=true
                shift
                ;;
            --ios)
                os="ios"
                arch="multiple"
                mode="gui-only"
                use_fyne_cross=true
                shift
                ;;
            -p|--parameters)
                extra_parameters="$2"
                shift 2
                ;;
            -c|--clean)
                clean=true
                shift
                ;;
            -l|--list)
                list_options
                ;;
            -h|--help)
                show_help
                ;;
            *)
                print_error "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
    
    # Clean output directory if requested
    if [ "$clean" == "true" ]; then
        print_info "Cleaning output directory: $output_dir"
        rm -rf "$output_dir"
    fi
    
    # Validate mode
    case $mode in
        hybrid|gui-only|cli-only) ;;
        *) print_error "Invalid mode '$mode'. Use hybrid, gui-only, or cli-only"; exit 1 ;;
    esac
    
    # Start build
    print_info "Starting build with parameters:"
    print_info "  OS: $os"
    print_info "  Architecture: $arch"
    print_info "  Mode: $mode"
    print_info "  Output Directory: $output_dir"
    print_info "  Use fyne-cross: $use_fyne_cross"
    echo ""
    
    # Check dependencies before building
    check_gui_dependencies "$os" "$mode"
    
    build_application "$os" "$arch" "$mode" "$output_dir" "$use_fyne_cross" "$extra_parameters"
}

# Check system dependencies for GUI builds
check_gui_dependencies() {
    local os=$1
    local mode=$2
    
    # Only check for Linux GUI builds with standard Go build
    if [ "$os" == "linux" ] && [ "$mode" != "cli-only" ]; then
        print_info "Checking GUI dependencies for Linux..."
        
        # List of required packages
        local required_libs=("libgl1-mesa-dev" "libxrandr-dev" "libxcursor-dev" "libxinerama-dev" "libxi-dev" "libxxf86vm-dev")
        local missing_libs=()
        
        # Check if we can detect package manager
        if command -v dpkg &> /dev/null; then
            # Debian/Ubuntu
            for lib in "${required_libs[@]}"; do
                if ! dpkg -l | grep -q "^ii  $lib "; then
                    missing_libs+=("$lib")
                fi
            done
        elif command -v rpm &> /dev/null; then
            # RedHat/CentOS/Fedora - different package names
            local rpm_libs=("mesa-libGL-devel" "libXrandr-devel" "libXcursor-devel" "libXinerama-devel" "libXi-devel" "libXxf86vm-devel")
            for lib in "${rpm_libs[@]}"; do
                if ! rpm -q "$lib" &> /dev/null; then
                    missing_libs+=("$lib")
                fi
            done
        else
            print_warning "Cannot detect package manager. If build fails, install GUI development libraries."
            return 0
        fi
        
        if [ ${#missing_libs[@]} -ne 0 ]; then
            print_error "Missing required libraries for GUI build:"
            for lib in "${missing_libs[@]}"; do
                echo "  - $lib"
            done
            echo ""
            if command -v dpkg &> /dev/null; then
                print_error "Install with: sudo apt-get install ${missing_libs[*]}"
            elif command -v rpm &> /dev/null; then
                print_error "Install with: sudo dnf install ${missing_libs[*]} (or sudo yum install ...)"
            fi
            echo ""
            print_error "Cannot proceed without required dependencies. Exiting."
            exit 1
        else
            print_success "All GUI dependencies found"
        fi
    fi
}

# Main execution
main() {
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        print_error "Go is not installed or not in PATH"
        exit 1
    fi
    
    # Check if we're in a Go project
    if [ ! -f "go.mod" ]; then
        print_error "No go.mod found. Please run this script from the project root directory"
        exit 1
    fi
    
    # Parse arguments and execute
    if [ $# -eq 0 ]; then
        print_info "No arguments provided, using defaults"
        check_gui_dependencies "$DEFAULT_OS" "$DEFAULT_MODE"
        build_application "$DEFAULT_OS" "$DEFAULT_ARCH" "$DEFAULT_MODE" "$DEFAULT_OUTPUT_DIR" "$DEFAULT_USE_FYNE_CROSS"
    else
        parse_args "$@"
    fi
    
    print_success "Compilation script completed!"
}

# Execute main
main "$@"