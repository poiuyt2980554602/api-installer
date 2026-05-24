#!/usr/bin/env bash
#
# Fast installer for Pixel-built Sub2API binaries.
# Usage:
#   curl -sSL https://raw.githubusercontent.com/poiuyt2980554602/api-installer/main/deploy/install.sh | sudo bash
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

RELEASE_REPO="${RELEASE_REPO:-poiuyt2980554602/api-installer}"
PIXEL_VERSION="${PIXEL_VERSION:-1.0.12}"
RELEASE_TAG="${RELEASE_TAG:-v${PIXEL_VERSION}-pixel}"

APP_NAME="sub2api"
SERVICE_NAME="sub2api"
SERVICE_USER="sub2api"
INSTALL_DIR="/opt/sub2api"
CONFIG_DIR="/etc/sub2api"
SERVICE_FILE="/etc/systemd/system/sub2api.service"

SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_PORT="${SERVER_PORT:-8080}"
FORCE_YES="${FORCE_YES:-false}"
PURGE_CONFIG="${PURGE_CONFIG:-false}"

OS=""
ARCH=""
TMP_DIR=""
PUBLIC_IP=""

cleanup() {
    if [ -n "${TMP_DIR:-}" ] && [ -d "$TMP_DIR" ]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT

print_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

print_success() {
    echo -e "${GREEN}[OK]${NC} $*"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

usage() {
    cat <<EOF
Usage: $0 [install|upgrade|uninstall] [options]

Commands:
  install      Install Pixel ${PIXEL_VERSION} binary release (default)
  upgrade      Same as install
  uninstall    Remove service and installed files

Options:
  -y, --yes           Skip uninstall confirmation
  --purge             Also remove ${CONFIG_DIR}
  -v, --version VER   Install Pixel version, default ${PIXEL_VERSION}

Environment overrides:
  RELEASE_REPO=${RELEASE_REPO}
  RELEASE_TAG=${RELEASE_TAG}
  SERVER_HOST=${SERVER_HOST}
  SERVER_PORT=${SERVER_PORT}
EOF
}

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        print_error "Please run as root, for example with sudo."
        exit 1
    fi
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
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
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            print_error "Unsupported architecture: $(uname -m)"
            exit 1
            ;;
    esac

    print_info "Detected platform: ${OS}_${ARCH}"
}

check_dependencies() {
    local missing=()

    for cmd in curl tar; do
        if ! command_exists "$cmd"; then
            missing+=("$cmd")
        fi
    done

    if [ "${#missing[@]}" -gt 0 ]; then
        print_error "Missing dependencies: ${missing[*]}"
        print_info "Please install them first, then rerun this installer."
        exit 1
    fi
}

download_and_extract() {
    local version_num="${PIXEL_VERSION#v}"
    local archive_name="sub2api_${version_num}_${OS}_${ARCH}.tar.gz"
    local base_url="https://github.com/${RELEASE_REPO}/releases/download/${RELEASE_TAG}"
    local download_url="${base_url}/${archive_name}"
    local checksum_url="${base_url}/checksums.txt"

    TMP_DIR="$(mktemp -d)"

    print_info "Downloading ${archive_name}..."
    if ! curl -fL "$download_url" -o "${TMP_DIR}/${archive_name}"; then
        print_error "Download failed: ${download_url}"
        print_error "The release asset may not be built yet. Check https://github.com/${RELEASE_REPO}/releases/tag/${RELEASE_TAG}"
        exit 1
    fi

    if command_exists sha256sum; then
        print_info "Verifying checksum..."
        if curl -fsL "$checksum_url" -o "${TMP_DIR}/checksums.txt"; then
            local expected
            local actual
            expected="$(grep " ${archive_name}$" "${TMP_DIR}/checksums.txt" | awk '{print $1}')"
            actual="$(sha256sum "${TMP_DIR}/${archive_name}" | awk '{print $1}')"

            if [ -z "$expected" ]; then
                print_warning "No checksum entry found for ${archive_name}; skipping checksum verification."
            elif [ "$expected" != "$actual" ]; then
                print_error "Checksum verification failed."
                print_error "Expected: $expected"
                print_error "Actual:   $actual"
                exit 1
            else
                print_success "Checksum verified"
            fi
        else
            print_warning "checksums.txt not found; skipping checksum verification."
        fi
    fi

    print_info "Extracting..."
    tar -xzf "${TMP_DIR}/${archive_name}" -C "$TMP_DIR"
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
    mkdir -p "$INSTALL_DIR" "${INSTALL_DIR}/data" "$CONFIG_DIR"

    if [ -f "${INSTALL_DIR}/${APP_NAME}" ]; then
        cp "${INSTALL_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}.backup.$(date +%Y%m%d%H%M%S)"
    fi

    install -m 0755 "${TMP_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"

    if [ -d "${TMP_DIR}/deploy" ]; then
        mkdir -p "${INSTALL_DIR}/deploy"
        cp -R "${TMP_DIR}/deploy/." "${INSTALL_DIR}/deploy/"
        if [ -f "${TMP_DIR}/deploy/config.example.yaml" ]; then
            install -m 0644 "${TMP_DIR}/deploy/config.example.yaml" "${CONFIG_DIR}/config.example.yaml"
        fi
    fi

    chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR" "$CONFIG_DIR"
    print_success "Binary installed to ${INSTALL_DIR}/${APP_NAME}"
}

install_service() {
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Sub2API (Pixel ${PIXEL_VERSION})
Documentation=https://github.com/Pixel-API/Pixel
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
    print_success "systemd service installed"
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
EOF
}

do_install() {
    check_root
    detect_platform
    check_dependencies
    download_and_extract
    create_user_if_needed
    install_files
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
                RELEASE_TAG="v${PIXEL_VERSION}-pixel"
                shift 2
                ;;
            --version=*)
                PIXEL_VERSION="${1#*=}"
                PIXEL_VERSION="${PIXEL_VERSION#v}"
                RELEASE_TAG="v${PIXEL_VERSION}-pixel"
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
