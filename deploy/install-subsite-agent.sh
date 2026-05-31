#!/usr/bin/env bash
#
# Fast installer for Pixel Sub2API subsite-agent binaries.
# Usage:
#   curl -sSL https://raw.githubusercontent.com/poiuyt2980554602/api-installer/main/deploy/install-subsite-agent.sh | sudo bash
#

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

RELEASE_REPO="${RELEASE_REPO:-poiuyt2980554602/api-installer}"
PIXEL_VERSION="${PIXEL_VERSION:-1.0.20.7}"
RELEASE_TAG="${RELEASE_TAG:-v${PIXEL_VERSION}-forwarder-pixel}"

APP_NAME="sub2api-subsite-agent"
SERVICE_NAME="sub2api-subsite-agent"
SERVICE_USER="sub2api"
INSTALL_DIR="/opt/sub2api-subsite-agent"
DATA_DIR="/var/lib/sub2api-subsite-agent"
CONFIG_DIR="/etc/sub2api"
ENV_FILE="${CONFIG_DIR}/subsite-agent.env"
SERVICE_FILE="/etc/systemd/system/sub2api-subsite-agent.service"

FORCE_YES="${FORCE_YES:-false}"
PURGE_CONFIG="${PURGE_CONFIG:-false}"

OS=""
ARCH=""
TMP_DIR=""

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
  install      Install Pixel ${PIXEL_VERSION} subsite-agent (default)
  upgrade      Same as install
  uninstall    Remove subsite-agent service and installed files

Options:
  -y, --yes           Skip uninstall confirmation
  --purge             Also remove ${ENV_FILE}
  -v, --version VER   Install Pixel version, default ${PIXEL_VERSION}
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
        exit 1
    fi
}

download_and_extract() {
    local version_num="${PIXEL_VERSION#v}"
    local archive_name="sub2api-subsite-agent_${version_num}_${OS}_${ARCH}.tar.gz"
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

    useradd -r -s /bin/sh -d "$INSTALL_DIR" "$SERVICE_USER"
    print_success "Created service user ${SERVICE_USER}"
}

install_files() {
    mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR"

    if [ -f "${INSTALL_DIR}/${APP_NAME}" ]; then
        cp "${INSTALL_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}.backup.$(date +%Y%m%d%H%M%S)"
    fi

    install -m 0755 "${TMP_DIR}/${APP_NAME}" "${INSTALL_DIR}/${APP_NAME}"

    if [ ! -f "$ENV_FILE" ]; then
        if [ -f "${TMP_DIR}/deploy/subsite-agent.env.example" ]; then
            install -m 0640 "${TMP_DIR}/deploy/subsite-agent.env.example" "$ENV_FILE"
        else
            cat > "$ENV_FILE" <<EOF
GIN_MODE=release
SUBSITE_LISTEN_ADDR=0.0.0.0:8081
SUBSITE_ID=
SUBSITE_PUBLIC_URL=https://edge.example.com
SUBSITE_MASTER_URL=https://master.example.com
SUBSITE_MASTER_SECRET=
SUBSITE_USAGE_QUEUE_PATH=${DATA_DIR}/subsite-usage.db
SUBSITE_VERSION=${PIXEL_VERSION}
EOF
        fi
        print_warning "Created ${ENV_FILE}. Fill SUBSITE_ID, SUBSITE_PUBLIC_URL, SUBSITE_MASTER_URL, and SUBSITE_MASTER_SECRET before starting."
    else
        print_info "Keeping existing ${ENV_FILE}"
    fi

    chown -R "${SERVICE_USER}:${SERVICE_USER}" "$INSTALL_DIR" "$DATA_DIR"
    chown "${SERVICE_USER}:${SERVICE_USER}" "$ENV_FILE" 2>/dev/null || true
    print_success "Binary installed to ${INSTALL_DIR}/${APP_NAME}"
}

install_service() {
    if [ -f "${TMP_DIR}/deploy/sub2api-subsite-agent.service" ]; then
        install -m 0644 "${TMP_DIR}/deploy/sub2api-subsite-agent.service" "$SERVICE_FILE"
    else
        cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=Sub2API Subsite Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/${APP_NAME}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
EOF
    fi

    systemctl daemon-reload
    print_success "systemd service installed"
}

env_is_configured() {
    [ -f "$ENV_FILE" ] &&
        grep -q '^SUBSITE_ID=.\+' "$ENV_FILE" &&
        grep -q '^SUBSITE_MASTER_SECRET=.\+' "$ENV_FILE" &&
        ! grep -q '^SUBSITE_PUBLIC_URL=https://edge.example.com$' "$ENV_FILE" &&
        ! grep -q '^SUBSITE_MASTER_URL=https://master.example.com$' "$ENV_FILE"
}

start_or_explain() {
    systemctl enable "$SERVICE_NAME" >/dev/null 2>&1 || true

    if ! env_is_configured; then
        print_warning "Subsite agent is installed but not started because ${ENV_FILE} still needs real values."
        print_info "Edit it with: nano ${ENV_FILE}"
        print_info "Then start with: systemctl enable --now ${SERVICE_NAME}"
        return 0
    fi

    print_info "Starting ${SERVICE_NAME}..."
    if systemctl restart "$SERVICE_NAME"; then
        print_success "Service started"
    else
        print_error "Service failed to start. Check logs with: journalctl -u ${SERVICE_NAME} -n 100"
        exit 1
    fi
}

do_install() {
    check_root
    detect_platform
    check_dependencies
    download_and_extract
    create_user_if_needed
    install_files
    install_service
    start_or_explain
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
    rm -rf "$INSTALL_DIR" "$DATA_DIR"

    if [ "$PURGE_CONFIG" = "true" ]; then
        rm -f "$ENV_FILE"
    else
        print_warning "Configuration kept at ${ENV_FILE}"
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
                RELEASE_TAG="v${PIXEL_VERSION}-forwarder-pixel"
                shift 2
                ;;
            --version=*)
                PIXEL_VERSION="${1#*=}"
                PIXEL_VERSION="${PIXEL_VERSION#v}"
                RELEASE_TAG="v${PIXEL_VERSION}-forwarder-pixel"
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
