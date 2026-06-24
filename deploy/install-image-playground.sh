#!/usr/bin/env bash
#
# Fast installer for Pixel Image Playground.
# Usage:
#   curl -sSL https://raw.githubusercontent.com/poiuyt2980554602/api-installer/main/deploy/install-image-playground.sh | sudo bash
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

RELEASE_REPO="${RELEASE_REPO:-poiuyt2980554602/api-installer}"
PLAYGROUND_VERSION="${PLAYGROUND_VERSION:-0.6.10.1}"
RELEASE_TAG="${RELEASE_TAG:-v${PLAYGROUND_VERSION}-image-playground}"

APP_NAME="pixel-image-playground"
SERVICE_NAME="pixel-image-playground"
SERVICE_USER="pixelimg"
INSTALL_DIR="/opt/pixel-image-playground"
SERVICE_FILE="/etc/systemd/system/pixel-image-playground.service"

SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
SERVER_PORT="${SERVER_PORT:-8090}"
FORCE_YES="${FORCE_YES:-false}"
SERVER_CONFIGURED="${SERVER_CONFIGURED:-false}"

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

is_interactive() {
    [ -e /dev/tty ] && [ -r /dev/tty ] && [ -w /dev/tty ]
}

validate_port() {
    local port="$1"
    if [[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
        return 0
    fi
    return 1
}

configure_server() {
    if [ "$SERVER_CONFIGURED" = "true" ]; then
        print_info "Image playground listen address: ${SERVER_HOST}:${SERVER_PORT}"
        return 0
    fi

    if ! is_interactive; then
        print_info "Image playground listen address: ${SERVER_HOST}:${SERVER_PORT}"
        return 0
    fi

    echo ""
    echo -e "${BLUE}Image playground configuration${NC}"
    echo "0.0.0.0 listens on all network interfaces; 127.0.0.1 only listens locally."

    local input_host=""
    read -r -p "Listen address [${SERVER_HOST}]: " input_host < /dev/tty
    if [ -n "$input_host" ]; then
        SERVER_HOST="$input_host"
    fi

    local input_port=""
    while true; do
        read -r -p "Listen port [${SERVER_PORT}]: " input_port < /dev/tty
        if [ -z "$input_port" ]; then
            break
        fi
        if validate_port "$input_port"; then
            SERVER_PORT="$input_port"
            break
        fi
        print_error "Invalid port. Enter a number from 1 to 65535."
    done

    print_info "Image playground listen address: ${SERVER_HOST}:${SERVER_PORT}"
}

usage() {
    cat <<EOF
Usage: $0 [install|upgrade|uninstall] [options]

Commands:
  install      Install Pixel Image Playground ${PLAYGROUND_VERSION} (default)
  upgrade      Same as install
  uninstall    Remove service and installed files

Options:
  -y, --yes           Skip uninstall confirmation
  -v, --version VER   Install image playground version, default ${PLAYGROUND_VERSION}
  --host HOST         Listen address, default ${SERVER_HOST}
  --port PORT         Listen port, default ${SERVER_PORT}

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
    local version_num="${PLAYGROUND_VERSION#v}"
    local archive_name="pixel-image-playground_${version_num}_${OS}_${ARCH}.tar.gz"
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
            expected="$(awk -v name="$archive_name" '$2 == name { print $1; found = 1; exit } END { if (!found) exit 0 }' "${TMP_DIR}/checksums.txt")"
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

    useradd -r -s /usr/sbin/nologin -d "$INSTALL_DIR" "$SERVICE_USER"
    print_success "Created service user ${SERVICE_USER}"
}

install_files() {
    mkdir -p "$INSTALL_DIR"

    if [ -f "${INSTALL_DIR}/${APP_NAME}" ]; then
        cp "${INSTALL_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}.backup.$(date +%Y%m%d%H%M%S)"
    fi

    install -m 0755 "${TMP_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"
    if [ -f "${TMP_DIR}/README.md" ]; then
        install -m 0644 "${TMP_DIR}/README.md" "${INSTALL_DIR}/README.md"
    fi
    if [ -f "${TMP_DIR}/LICENSE" ]; then
        install -m 0644 "${TMP_DIR}/LICENSE" "${INSTALL_DIR}/LICENSE"
    fi

    chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR"
    print_success "Binary installed to ${INSTALL_DIR}/${APP_NAME}"
}

verify_binary() {
    local binary_path="${INSTALL_DIR}/${APP_NAME}"
    local version_output=""

    version_output="$("$binary_path" --version 2>&1 || true)"
    if printf '%s' "$version_output" | grep -q "Pixel Image Playground ${PLAYGROUND_VERSION}"; then
        print_success "Installed binary version verified: ${PLAYGROUND_VERSION}"
        return 0
    fi

    print_warning "Installed binary version could not be verified as ${PLAYGROUND_VERSION}."
    print_warning "Version output: ${version_output}"
}

install_service() {
    cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Pixel Image Playground (${PLAYGROUND_VERSION})
Documentation=https://github.com/CookSleep/gpt_image_playground
After=network.target

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

Environment=IMAGE_PLAYGROUND_HOST=${SERVER_HOST}
Environment=IMAGE_PLAYGROUND_PORT=${SERVER_PORT}

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
 Pixel Image Playground installation completed
==============================================

Version:        ${PLAYGROUND_VERSION}
Install dir:    ${INSTALL_DIR}
Listen address: ${SERVER_HOST}:${SERVER_PORT}

Open:
  http://${display_host}:${SERVER_PORT}

Use this URL as Pixel frontend VITE_IMAGE_PLAYGROUND_URL:
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
    configure_server
    download_and_extract
    create_user_if_needed
    install_files
    verify_binary
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
            -v|--version)
                if [ -z "${2:-}" ]; then
                    print_error "--version requires a value"
                    exit 1
                fi
                PLAYGROUND_VERSION="${2#v}"
                RELEASE_TAG="v${PLAYGROUND_VERSION}-image-playground"
                shift 2
                ;;
            --version=*)
                PLAYGROUND_VERSION="${1#*=}"
                PLAYGROUND_VERSION="${PLAYGROUND_VERSION#v}"
                RELEASE_TAG="v${PLAYGROUND_VERSION}-image-playground"
                shift
                ;;
            --host)
                if [ -z "${2:-}" ]; then
                    print_error "--host requires a value"
                    exit 1
                fi
                SERVER_HOST="$2"
                SERVER_CONFIGURED="true"
                shift 2
                ;;
            --host=*)
                SERVER_HOST="${1#*=}"
                SERVER_CONFIGURED="true"
                shift
                ;;
            --port)
                if [ -z "${2:-}" ]; then
                    print_error "--port requires a value"
                    exit 1
                fi
                if ! validate_port "$2"; then
                    print_error "Invalid port. Enter a number from 1 to 65535."
                    exit 1
                fi
                SERVER_PORT="$2"
                SERVER_CONFIGURED="true"
                shift 2
                ;;
            --port=*)
                SERVER_PORT="${1#*=}"
                if ! validate_port "$SERVER_PORT"; then
                    print_error "Invalid port. Enter a number from 1 to 65535."
                    exit 1
                fi
                SERVER_CONFIGURED="true"
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

    PLAYGROUND_VERSION="${PLAYGROUND_VERSION#v}"

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
