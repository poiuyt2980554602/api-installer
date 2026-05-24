#!/usr/bin/env bash
#
# Pixel 1.0.12 installer for Sub2API-compatible deployment
# Usage:
#   curl -sSL <your-raw-install-sh-url> | sudo bash
#   curl -sSL <your-raw-install-sh-url> | sudo env PIXEL_VERSION=1.0.12 bash
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

APP_NAME="sub2api"
SERVICE_NAME="sub2api"
SERVICE_USER="sub2api"
INSTALL_DIR="/opt/sub2api"
CONFIG_DIR="/etc/sub2api"
SERVICE_FILE="/etc/systemd/system/sub2api.service"

PIXEL_REPO="${PIXEL_REPO:-Pixel-API/Pixel}"
PIXEL_VERSION="${PIXEL_VERSION:-1.0.12}"
PIXEL_TARBALL_URL="${PIXEL_TARBALL_URL:-https://github.com/Pixel-API/Pixel/archive/refs/tags/${PIXEL_VERSION}.tar.gz}"

GO_MIN_VERSION="${GO_MIN_VERSION:-1.26.2}"
GO_INSTALL_VERSION="${GO_INSTALL_VERSION:-1.26.3}"
NODE_INSTALL_VERSION="${NODE_INSTALL_VERSION:-24.16.0}"
PNPM_INSTALL_VERSION="${PNPM_INSTALL_VERSION:-9.0.0}"

SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_PORT="${SERVER_PORT:-8080}"
FORCE_YES="${FORCE_YES:-false}"
PURGE_CONFIG="${PURGE_CONFIG:-false}"

OS=""
ARCH=""
NODE_ARCH=""
PKG_MANAGER=""
TMP_DIR=""
PUBLIC_IP=""

cleanup() {
    if [ -n "${TMP_DIR:-}" ] && [ -d "$TMP_DIR" ]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT

print_info() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

print_success() {
    echo -e "${GREEN}[OK]${NC} $*" >&2
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

usage() {
    cat <<EOF
Usage: $0 [install|upgrade|uninstall] [options]

Commands:
  install      Download, build, and install Pixel ${PIXEL_VERSION} (default)
  upgrade      Same as install
  uninstall    Remove installed service and files

Options:
  -y, --yes           Skip uninstall confirmation
  --purge             Also remove ${CONFIG_DIR}
  -v, --version VER   Override Pixel version (default: ${PIXEL_VERSION})

Environment overrides:
  PIXEL_VERSION=${PIXEL_VERSION}
  PIXEL_TARBALL_URL=${PIXEL_TARBALL_URL}
  SERVER_HOST=${SERVER_HOST}
  SERVER_PORT=${SERVER_PORT}
EOF
}

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        print_error "Please run this installer as root."
        exit 1
    fi
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

version_ge() {
    [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}

detect_platform() {
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    case "$OS" in
        linux) ;;
        *)
            print_error "Unsupported OS: $OS. This installer only supports Linux."
            exit 1
            ;;
    esac

    case "$(uname -m)" in
        x86_64|amd64)
            ARCH="amd64"
            NODE_ARCH="x64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            NODE_ARCH="arm64"
            ;;
        *)
            print_error "Unsupported architecture: $(uname -m)"
            exit 1
            ;;
    esac

    print_info "Detected platform: ${OS}/${ARCH}"
}

detect_package_manager() {
    for candidate in apt-get dnf yum zypper pacman apk; do
        if command_exists "$candidate"; then
            PKG_MANAGER="$candidate"
            return 0
        fi
    done
    return 1
}

install_packages() {
    if [ "$#" -eq 0 ]; then
        return 0
    fi

    if ! detect_package_manager; then
        print_error "No supported package manager found. Please install missing dependencies manually: $*"
        exit 1
    fi

    print_info "Installing build dependencies with ${PKG_MANAGER}..."

    case "$PKG_MANAGER" in
        apt-get)
            apt-get update
            DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
            ;;
        dnf)
            dnf install -y "$@"
            ;;
        yum)
            yum install -y "$@"
            ;;
        zypper)
            zypper --non-interactive install "$@"
            ;;
        pacman)
            pacman -Sy --noconfirm "$@"
            ;;
        apk)
            apk add --no-cache "$@"
            ;;
    esac
}

ensure_base_dependencies() {
    local missing=()
    local packages=()

    for cmd in curl tar; do
        if ! command_exists "$cmd"; then
            missing+=("$cmd")
        fi
    done

    if ! command_exists xz && ! command_exists unxz; then
        missing+=("xz")
    fi

    if [ "${#missing[@]}" -eq 0 ]; then
        return 0
    fi

    print_warning "Missing base dependencies: ${missing[*]}"

    case "${PKG_MANAGER:-}" in
        apt-get) packages=(curl tar xz-utils ca-certificates) ;;
        dnf|yum) packages=(curl tar xz ca-certificates) ;;
        zypper) packages=(curl tar xz ca-certificates) ;;
        pacman) packages=(curl tar xz ca-certificates) ;;
        apk) packages=(curl tar xz ca-certificates) ;;
        *)
            detect_package_manager || true
            ;;
    esac

    install_packages "${packages[@]}"
}

ensure_build_toolchain_packages() {
    case "${PKG_MANAGER:-}" in
        apt-get)
            install_packages build-essential git python3 xz-utils ca-certificates
            ;;
        dnf|yum)
            install_packages gcc gcc-c++ make git python3 xz ca-certificates
            ;;
        zypper)
            install_packages gcc gcc-c++ make git python3 xz ca-certificates
            ;;
        pacman)
            install_packages base-devel git python python-pip xz ca-certificates
            ;;
        apk)
            install_packages build-base git python3 xz ca-certificates
            ;;
        *)
            print_warning "Skipping package-manager based toolchain install."
            ;;
    esac
}

current_go_version() {
    if ! command_exists go; then
        return 1
    fi
    go version | awk '{print $3}' | sed 's/^go//'
}

ensure_go() {
    local current=""
    current="$(current_go_version || true)"

    if [ -n "$current" ] && version_ge "$current" "$GO_MIN_VERSION"; then
        print_info "Using existing Go ${current}"
    else
        print_info "Installing Go ${GO_INSTALL_VERSION}..."
        local archive="go${GO_INSTALL_VERSION}.linux-${ARCH}.tar.gz"
        local url="https://go.dev/dl/${archive}"

        TMP_DIR="$(mktemp -d)"
        curl -fsSL "$url" -o "$TMP_DIR/$archive"
        rm -rf /usr/local/go
        tar -C /usr/local -xzf "$TMP_DIR/$archive"
        ln -sf /usr/local/go/bin/go /usr/local/bin/go
        ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
        print_success "Installed Go ${GO_INSTALL_VERSION}"
    fi

    export PATH="/usr/local/go/bin:${PATH}"
}

current_node_major() {
    if ! command_exists node; then
        return 1
    fi
    node -p "process.versions.node.split('.')[0]"
}

ensure_node_and_pnpm() {
    local node_major=""
    node_major="$(current_node_major || true)"

    if [ -n "$node_major" ] && [ "$node_major" -ge 20 ]; then
        print_info "Using existing Node.js $(node -v)"
    else
        print_info "Installing Node.js v${NODE_INSTALL_VERSION}..."
        local archive="node-v${NODE_INSTALL_VERSION}-linux-${NODE_ARCH}.tar.xz"
        local url="https://nodejs.org/dist/v${NODE_INSTALL_VERSION}/${archive}"

        TMP_DIR="$(mktemp -d)"
        curl -fsSL "$url" -o "$TMP_DIR/$archive"
        rm -rf /usr/local/lib/nodejs
        mkdir -p /usr/local/lib/nodejs
        tar -C /usr/local/lib/nodejs -xf "$TMP_DIR/$archive"

        local node_root="/usr/local/lib/nodejs/node-v${NODE_INSTALL_VERSION}-linux-${NODE_ARCH}"
        ln -sf "${node_root}/bin/node" /usr/local/bin/node
        ln -sf "${node_root}/bin/npm" /usr/local/bin/npm
        ln -sf "${node_root}/bin/npx" /usr/local/bin/npx
        ln -sf "${node_root}/bin/corepack" /usr/local/bin/corepack
        print_success "Installed Node.js v${NODE_INSTALL_VERSION}"
    fi

    corepack enable
    corepack prepare "pnpm@${PNPM_INSTALL_VERSION}" --activate
    print_info "Using pnpm $(pnpm --version)"
}

download_source() {
    TMP_DIR="$(mktemp -d)"
    local tarball="${TMP_DIR}/pixel.tar.gz"
    local src_dir="${TMP_DIR}/src"

    print_info "Downloading Pixel source: ${PIXEL_TARBALL_URL}"
    curl -fsSL "${PIXEL_TARBALL_URL}" -o "$tarball" >&2

    mkdir -p "$src_dir"
    tar -xzf "$tarball" -C "$src_dir" --strip-components=1
    echo "$src_dir"
}

build_pixel() {
    local src_dir="$1"
    local build_dir="${src_dir}/build"
    local frontend_dir="${src_dir}/frontend"
    local backend_dir="${src_dir}/backend"

    mkdir -p "$build_dir"

    print_info "Installing frontend dependencies..."
    pnpm --dir "$frontend_dir" install --frozen-lockfile >&2

    print_info "Building embedded frontend..."
    pnpm --dir "$frontend_dir" run build >&2

    if [ ! -f "${backend_dir}/internal/web/dist/index.html" ]; then
        print_error "Frontend build completed but embedded dist/index.html was not generated."
        exit 1
    fi

    print_info "Building backend binary..."
    (
        cd "$backend_dir"
        CGO_ENABLED=0 go build \
            -tags=embed \
            -trimpath \
            -ldflags="-s -w -X main.Version=${PIXEL_VERSION} -X main.BuildType=source" \
            -o "${build_dir}/${APP_NAME}" \
            ./cmd/server >&2
    )

    if [ ! -f "${build_dir}/${APP_NAME}" ]; then
        print_error "Backend build failed: binary not found."
        exit 1
    fi

    echo "${build_dir}/${APP_NAME}"
}

create_user_if_needed() {
    if id "$SERVICE_USER" >/dev/null 2>&1; then
        print_info "User ${SERVICE_USER} already exists"
        return 0
    fi

    useradd -r -s /bin/sh -d "$INSTALL_DIR" "$SERVICE_USER"
    print_success "Created service user ${SERVICE_USER}"
}

install_files() {
    local src_dir="$1"
    local binary_path="$2"

    mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/data" "$CONFIG_DIR"

    if [ -f "${INSTALL_DIR}/${APP_NAME}" ]; then
        cp "${INSTALL_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}.backup.$(date +%Y%m%d%H%M%S)"
    fi

    install -m 0755 "$binary_path" "${INSTALL_DIR}/${APP_NAME}"

    if [ -f "${src_dir}/deploy/config.example.yaml" ]; then
        install -m 0644 "${src_dir}/deploy/config.example.yaml" "${CONFIG_DIR}/config.example.yaml"
    fi

    if [ -d "${src_dir}/deploy" ]; then
        mkdir -p "${INSTALL_DIR}/deploy"
        cp -R "${src_dir}/deploy/." "${INSTALL_DIR}/deploy/"
    fi

    chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR" "$CONFIG_DIR"
    print_success "Installed files to ${INSTALL_DIR}"
}

install_service() {
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Sub2API (Pixel ${PIXEL_VERSION})
Documentation=https://github.com/${PIXEL_REPO}
After=network.target postgresql.service redis.service
Wants=postgresql.service redis.service

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${APP_NAME}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}

NoNewPrivileges=true
ProtectHome=true
PrivateTmp=true
ReadWritePaths=${INSTALL_DIR}

Environment=GIN_MODE=release
Environment=SERVER_HOST=${SERVER_HOST}
Environment=SERVER_PORT=${SERVER_PORT}

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true
    print_success "Installed systemd service ${SERVICE_FILE}"
}

get_public_ip() {
    PUBLIC_IP="$(curl -fsSL --connect-timeout 5 --max-time 10 https://api.ipify.org 2>/dev/null || true)"
    if [ -z "$PUBLIC_IP" ]; then
        PUBLIC_IP="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    fi
}

start_service() {
    print_info "Starting ${SERVICE_NAME}..."
    if systemctl restart "$SERVICE_NAME"; then
        print_success "Service started"
    else
        print_error "Service failed to start. Check logs with: journalctl -u ${SERVICE_NAME} -n 100"
        exit 1
    fi
}

print_completion() {
    get_public_ip
    local display_host="${PUBLIC_IP:-YOUR_SERVER_IP}"

    if [ "$SERVER_HOST" = "127.0.0.1" ]; then
        display_host="127.0.0.1"
    fi

    cat <<EOF

==============================================
 Installation completed
==============================================

Version:        Pixel ${PIXEL_VERSION}
Install dir:    ${INSTALL_DIR}
Config dir:     ${CONFIG_DIR}
Listen address: ${SERVER_HOST}:${SERVER_PORT}

Open the setup page:
  http://${display_host}:${SERVER_PORT}

Useful commands:
  systemctl status ${SERVICE_NAME}
  journalctl -u ${SERVICE_NAME} -f
  systemctl restart ${SERVICE_NAME}

If this is the first install, complete the web setup wizard to write config.yaml.
EOF
}

do_install() {
    check_root
    detect_platform
    detect_package_manager || true
    ensure_base_dependencies
    ensure_build_toolchain_packages
    ensure_go
    ensure_node_and_pnpm

    local src_dir
    local binary_path

    src_dir="$(download_source)"
    binary_path="$(build_pixel "$src_dir")"

    create_user_if_needed
    install_files "$src_dir" "$binary_path"
    install_service
    start_service
    print_completion
}

do_uninstall() {
    check_root

    if [ "$FORCE_YES" != "true" ]; then
        read -r -p "This will remove ${APP_NAME}. Continue? [y/N]: " reply
        case "$reply" in
            y|Y|yes|YES) ;;
            *)
                print_info "Uninstall cancelled"
                exit 0
                ;;
        esac
    fi

    systemctl stop "$SERVICE_NAME" 2>/dev/null || true
    systemctl disable "$SERVICE_NAME" 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload

    rm -rf "$INSTALL_DIR"

    if id "$SERVICE_USER" >/dev/null 2>&1; then
        userdel "$SERVICE_USER" 2>/dev/null || true
    fi

    if [ "$PURGE_CONFIG" = "true" ]; then
        rm -rf "$CONFIG_DIR"
    else
        print_warning "Configuration kept at ${CONFIG_DIR}"
    fi

    print_success "Uninstall finished"
}

main() {
    local command="install"

    while [ "$#" -gt 0 ]; do
        case "$1" in
            install|upgrade|uninstall)
                command="$1"
                shift
                ;;
            -y|--yes)
                FORCE_YES="true"
                shift
                ;;
            --purge)
                PURGE_CONFIG="true"
                shift
                ;;
            -v|--version)
                if [ -z "${2:-}" ]; then
                    print_error "--version requires a value"
                    exit 1
                fi
                PIXEL_VERSION="${2#v}"
                PIXEL_TARBALL_URL="https://github.com/Pixel-API/Pixel/archive/refs/tags/${PIXEL_VERSION}.tar.gz"
                shift 2
                ;;
            --version=*)
                PIXEL_VERSION="${1#*=}"
                PIXEL_VERSION="${PIXEL_VERSION#v}"
                PIXEL_TARBALL_URL="https://github.com/Pixel-API/Pixel/archive/refs/tags/${PIXEL_VERSION}.tar.gz"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                print_error "Unknown argument: $1"
                usage
                exit 1
                ;;
        esac
    done

    PIXEL_VERSION="${PIXEL_VERSION#v}"
    PIXEL_TARBALL_URL="${PIXEL_TARBALL_URL:-https://github.com/Pixel-API/Pixel/archive/refs/tags/${PIXEL_VERSION}.tar.gz}"

    case "$command" in
        install|upgrade)
            do_install
            ;;
        uninstall)
            do_uninstall
            ;;
        *)
            print_error "Unknown command: $command"
            exit 1
            ;;
    esac
}

main "$@"
