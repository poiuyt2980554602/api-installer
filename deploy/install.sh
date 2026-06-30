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
PIXEL_VERSION="${PIXEL_VERSION:-1.1.50.5}"
RELEASE_TAG="${RELEASE_TAG:-v${PIXEL_VERSION}-pixel}"
IMAGE_PLAYGROUND_URL="${IMAGE_PLAYGROUND_URL:-https://tp.kelisiai.pro}"
EXTRA_CORS_ORIGINS="${EXTRA_CORS_ORIGINS:-http://wails.localhost}"

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
        print_info "Server listen address: ${SERVER_HOST}:${SERVER_PORT}"
        return 0
    fi

    if ! is_interactive; then
        print_info "Server listen address: ${SERVER_HOST}:${SERVER_PORT}"
        return 0
    fi

    echo ""
    echo -e "${BLUE}Server configuration${NC}"
    echo "0.0.0.0 listens on all network interfaces; 127.0.0.1 only listens locally."

    local input_host=""
    read -r -p "Server listen address [${SERVER_HOST}]: " input_host < /dev/tty
    if [ -n "$input_host" ]; then
        SERVER_HOST="$input_host"
    fi

    local input_port=""
    while true; do
        read -r -p "Server port [${SERVER_PORT}]: " input_port < /dev/tty
        if [ -z "$input_port" ]; then
            break
        fi
        if validate_port "$input_port"; then
            SERVER_PORT="$input_port"
            break
        fi
        print_error "Invalid port. Enter a number from 1 to 65535."
    done

    print_info "Server listen address: ${SERVER_HOST}:${SERVER_PORT}"
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
  --host HOST         Server listen address, default ${SERVER_HOST}
  --port PORT         Server port, default ${SERVER_PORT}

Environment overrides:
  RELEASE_REPO=${RELEASE_REPO}
  RELEASE_TAG=${RELEASE_TAG}
  IMAGE_PLAYGROUND_URL=${IMAGE_PLAYGROUND_URL}
  EXTRA_CORS_ORIGINS=${EXTRA_CORS_ORIGINS}
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

binary_contains() {
    local file="$1"
    local pattern="$2"

    if command_exists grep; then
        grep -a -q "$pattern" "$file" 2>/dev/null
        return $?
    fi

    if command_exists strings; then
        strings "$file" | grep -q "$pattern" 2>/dev/null
        return $?
    fi

    return 2
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

patch_image_playground_cors_file() {
    local config_file="$1"
    local origin="$2"
    local create_if_missing="${3:-false}"

    if [ ! -f "$config_file" ]; then
        if [ "$create_if_missing" != "true" ]; then
            return 0
        fi
        mkdir -p "$(dirname "$config_file")"
        cat > "$config_file" <<EOF
cors:
    allowed_origins:
        - ${origin}
    allow_credentials: true
EOF
        chown "${SERVICE_USER}:${SERVICE_USER}" "$config_file" 2>/dev/null || true
        print_success "Created ${config_file} with image playground CORS origin: ${origin}"
        return 0
    fi

    if grep -Fq -- "- ${origin}" "$config_file"; then
        print_info "Image playground CORS origin already configured: ${origin}"
        return 0
    fi

    cp "$config_file" "${config_file}.backup.$(date +%Y%m%d%H%M%S)"

    if command_exists python3; then
        ORIGIN="$origin" python3 - "$config_file" <<'PY'
import os
import re
import sys
from pathlib import Path

config_path = Path(sys.argv[1])
origin = os.environ["ORIGIN"].strip().rstrip("/")
text = config_path.read_text(encoding="utf-8")
lines = text.splitlines(True)

def line_indent(line: str) -> str:
    return line[: len(line) - len(line.lstrip(" "))]

def is_top_level_key(line: str) -> bool:
    stripped = line.strip()
    return bool(stripped and not line.startswith((" ", "\t", "#")) and ":" in stripped)

if any(re.match(rf"^\s*-\s*{re.escape(origin)}\s*(#.*)?$", line) for line in lines):
    sys.exit(0)

cors_idx = next((i for i, line in enumerate(lines) if re.match(r"^cors\s*:\s*(#.*)?$", line)), None)
if cors_idx is None:
    if lines and not lines[-1].endswith("\n"):
        lines[-1] += "\n"
    if lines and lines[-1].strip():
        lines.append("\n")
    lines.extend([
        "cors:\n",
        "    allowed_origins:\n",
        f"        - {origin}\n",
        "    allow_credentials: true\n",
    ])
    config_path.write_text("".join(lines), encoding="utf-8")
    sys.exit(0)

block_end = len(lines)
for i in range(cors_idx + 1, len(lines)):
    if is_top_level_key(lines[i]):
        block_end = i
        break

allowed_idx = None
for i in range(cors_idx + 1, block_end):
    if re.match(r"^\s*allowed_origins\s*:", lines[i]):
        allowed_idx = i
        break

if allowed_idx is None:
    lines.insert(cors_idx + 1, f"    allow_credentials: true\n")
    lines.insert(cors_idx + 1, f"        - {origin}\n")
    lines.insert(cors_idx + 1, "    allowed_origins:\n")
    config_path.write_text("".join(lines), encoding="utf-8")
    sys.exit(0)

allowed_line = lines[allowed_idx]
allowed_indent = line_indent(allowed_line)
list_indent = allowed_indent + "    "

if re.search(r"allowed_origins\s*:\s*\[\s*\]", allowed_line):
    lines[allowed_idx] = f"{allowed_indent}allowed_origins:\n"
    lines.insert(allowed_idx + 1, f"{list_indent}- {origin}\n")
    config_path.write_text("".join(lines), encoding="utf-8")
    sys.exit(0)

insert_idx = allowed_idx + 1
for i in range(allowed_idx + 1, block_end):
    stripped = lines[i].strip()
    if not stripped or stripped.startswith("#"):
        insert_idx = i + 1
        continue
    current_indent = len(lines[i]) - len(lines[i].lstrip(" "))
    allowed_indent_len = len(allowed_indent)
    if current_indent <= allowed_indent_len and not stripped.startswith("-"):
        break
    if stripped.startswith("-"):
        list_indent = line_indent(lines[i])
        insert_idx = i + 1
        continue
    insert_idx = i + 1

lines.insert(insert_idx, f"{list_indent}- {origin}\n")
config_path.write_text("".join(lines), encoding="utf-8")
PY
    else
        cat >> "$config_file" <<EOF

cors:
    allowed_origins:
        - ${origin}
    allow_credentials: true
EOF
        print_warning "python3 not found; appended CORS block to ${config_file}"
    fi

    chown "${SERVICE_USER}:${SERVICE_USER}" "$config_file" 2>/dev/null || true
    print_success "Configured image playground CORS origin in ${config_file}: ${origin}"
}

ensure_image_playground_cors() {
    local origin="${IMAGE_PLAYGROUND_URL%/}"
    local etc_config="${CONFIG_DIR}/config.yaml"
    local install_config="${INSTALL_DIR}/config.yaml"
    local origins=()
    local extra_origin=""

    if [ -z "$origin" ]; then
        print_info "Image playground CORS origin is empty; skipping CORS update"
        return 0
    fi

    mkdir -p "$CONFIG_DIR"

    origins+=("$origin")
    if [ -n "$EXTRA_CORS_ORIGINS" ]; then
        IFS=',' read -r -a extra_origins <<< "$EXTRA_CORS_ORIGINS"
        for extra_origin in "${extra_origins[@]}"; do
            extra_origin="$(printf '%s' "$extra_origin" | xargs)"
            if [ -n "$extra_origin" ]; then
                origins+=("${extra_origin%/}")
            fi
        done
    fi

    for origin in "${origins[@]}"; do
        patch_image_playground_cors_file "$etc_config" "$origin" "true"
    done

    if [ -f "$install_config" ]; then
        print_warning "${install_config} exists and can take precedence over ${etc_config}; updating it too."
        for origin in "${origins[@]}"; do
            patch_image_playground_cors_file "$install_config" "$origin" "false"
        done
    fi
}

verify_binary() {
    local binary_path="${INSTALL_DIR}/${APP_NAME}"
    local version_output=""

    version_output="$("$binary_path" --version 2>&1 || true)"
    if printf '%s' "$version_output" | grep -q "Sub2API ${PIXEL_VERSION}"; then
        print_success "Installed binary version verified: ${PIXEL_VERSION}"
        return 0
    fi

    print_warning "Installed binary version could not be verified as ${PIXEL_VERSION}."
    print_warning "Version output: ${version_output}"
    print_warning "Expected release: https://github.com/${RELEASE_REPO}/releases/tag/${RELEASE_TAG}"
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
    configure_server
    download_and_extract
    create_user_if_needed
    install_files
    ensure_image_playground_cors
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
